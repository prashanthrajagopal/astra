package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const goalText = `Build a complete Next.js 14 e-commerce website with the following features:

1. PROJECT SETUP: Initialize a Next.js 14 project with TypeScript, Tailwind CSS, and App Router.
   Run: npx create-next-app@latest . --typescript --tailwind --app --src-dir --no-eslint --import-alias "@/*" --use-npm

2. DATA MODEL: Create TypeScript types for Product (id, name, description, price, image, category, rating, inStock)
   and CartItem (product + quantity). Create a mock data file with 12 products across 3 categories
   (Electronics, Clothing, Home & Garden).

3. CART STATE: Implement a React Context provider for shopping cart state management with:
   addToCart, removeFromCart, updateQuantity, clearCart, cartTotal, cartCount.

4. LAYOUT: Create a shared app layout with:
   - Header with logo, navigation links (Home, Products, Cart with item count badge), and a search bar
   - Footer with copyright and links
   - Responsive design using Tailwind CSS

5. HOME PAGE: Landing page with hero section, featured products grid (6 items), and category cards linking to filtered product list.

6. PRODUCTS PAGE: Product listing page at /products with:
   - Category filter sidebar
   - Sort by price/name/rating
   - Product cards in a responsive grid

7. PRODUCT DETAIL PAGE: Dynamic route /products/[id] showing:
   - Large product image
   - Name, description, price, rating, stock status
   - Add to cart button with quantity selector

8. CART PAGE: Shopping cart at /cart with:
   - List of cart items with quantity controls and remove button
   - Cart summary with subtotal, tax, and total
   - Proceed to checkout button

9. CHECKOUT PAGE: Checkout form at /checkout with:
   - Shipping information form (name, email, address, city, zip)
   - Order summary sidebar
   - Place order button (mock, stores to localStorage)

10. ORDER CONFIRMATION: Success page at /order-confirmation showing order details.

11. STYLING: Apply consistent Tailwind CSS styling throughout. Modern, clean design with:
    - Color scheme: indigo primary, gray neutrals
    - Rounded corners, subtle shadows
    - Hover effects on interactive elements
    - Mobile-first responsive design`

func main() {
	gateway := flag.String("gateway", "http://localhost:8080", "API gateway address")
	identity := flag.String("identity", "http://localhost:8085", "Identity service address")
	goalSvc := flag.String("goal-service", "http://localhost:8088", "Goal service address")
	accessControl := flag.String("access-control", "http://localhost:8086", "Access control address")
	workspace := flag.String("workspace", "", "Workspace directory (default: ./workspace/ecommerce-store)")
	autoApprove := flag.Bool("auto-approve", false, "Automatically approve pending approval requests")
	pollInterval := flag.Duration("poll", 10*time.Second, "Poll interval for task status")
	timeout := flag.Duration("timeout", 30*time.Minute, "Maximum time to wait for completion")
	flag.Parse()

	if *workspace == "" {
		wd, _ := os.Getwd()
		*workspace = filepath.Join(wd, "workspace", "ecommerce-store")
	}

	fmt.Println("=== Astra E-Commerce Test ===")
	fmt.Printf("Gateway:     %s\n", *gateway)
	fmt.Printf("Workspace:   %s\n", *workspace)
	fmt.Printf("Auto-approve: %v\n", *autoApprove)
	fmt.Println()

	token := acquireToken(*identity)
	fmt.Printf("JWT acquired: %s...\n\n", token[:min(len(token), 20)])

	agentID := createAgent(*gateway, token)
	fmt.Printf("Agent created: %s\n\n", agentID)

	goalID, graphID, taskCount := submitGoal(*goalSvc, agentID, *workspace)
	fmt.Printf("Goal submitted: %s\n", goalID)
	fmt.Printf("Graph: %s (%d tasks)\n\n", graphID, taskCount)

	fmt.Println("Monitoring task progress...")
	fmt.Println(strings.Repeat("-", 60))

	deadline := time.Now().Add(*timeout)
	completed := false

	for time.Now().Before(deadline) {
		if *autoApprove {
			approveAll(*accessControl)
		}

		status, done := checkGoalStatus(*goalSvc, goalID)
		fmt.Printf("[%s] Goal %s: %s\n", time.Now().Format("15:04:05"), goalID[:8], status)

		if done {
			completed = true
			break
		}

		time.Sleep(*pollInterval)
	}

	fmt.Println(strings.Repeat("-", 60))
	finalStatus, _ := checkGoalStatus(*goalSvc, goalID)
	if completed && finalStatus == "completed" {
		fmt.Println("Goal completed successfully!")
		fmt.Printf("\nYour e-commerce site is at: %s\n", *workspace)
		fmt.Println("\nTo run it:")
		fmt.Printf("  cd %s\n", *workspace)
		fmt.Println("  npm install  # if not already done")
		fmt.Println("  npm run dev")
		fmt.Println("  # Open http://localhost:3000")
	} else if completed && finalStatus == "failed" {
		fmt.Println("Goal FAILED. Some tasks did not complete successfully.")
		fmt.Printf("Check the dashboard for details: %s/superadmin/dashboard/\n", *gateway)
		fmt.Printf("Workspace (partial output): %s\n", *workspace)
		os.Exit(1)
	} else {
		finalizeGoal(*goalSvc, goalID)
		fmt.Println("Goal did not complete within timeout. Check the dashboard for details.")
		fmt.Printf("Dashboard: %s/superadmin/dashboard/\n", *gateway)
		os.Exit(1)
	}
}

func acquireToken(identityAddr string) string {
	body := `{"subject":"ecommerce-test","scopes":["admin"],"ttl_seconds":3600}`
	resp, err := http.Post(identityAddr+"/tokens", "application/json", strings.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to acquire token: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	tok := result["token"]
	if tok == "" {
		fmt.Fprintf(os.Stderr, "Empty token from identity service\n")
		os.Exit(1)
	}
	return tok
}

func createAgent(gateway, token string) string {
	body := `{"actor_type":"ecommerce-builder","config":"{\"description\":\"Autonomous e-commerce site builder\"}"}`
	req, _ := http.NewRequest("POST", gateway+"/agents", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "Create agent failed (%d): %s\n", resp.StatusCode, string(data))
		os.Exit(1)
	}
	var result map[string]string
	json.Unmarshal(data, &result)
	id := result["actor_id"]
	if id == "" {
		fmt.Fprintf(os.Stderr, "No actor_id in response: %s\n", string(data))
		os.Exit(1)
	}
	return id
}

func submitGoal(goalSvc, agentID, workspace string) (goalID, graphID string, taskCount int) {
	payload := map[string]any{
		"agent_id":  agentID,
		"goal_text": goalText,
		"priority":  50,
		"workspace": workspace,
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(goalSvc+"/goals", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to submit goal: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "Submit goal failed (%d): %s\n", resp.StatusCode, string(data))
		os.Exit(1)
	}
	var result map[string]any
	json.Unmarshal(data, &result)
	goalID, _ = result["goal_id"].(string)
	graphID, _ = result["graph_id"].(string)
	if tc, ok := result["task_count"].(float64); ok {
		taskCount = int(tc)
	}
	return
}

func checkGoalStatus(goalSvc, goalID string) (string, bool) {
	resp, err := http.Get(goalSvc + "/goals/" + goalID)
	if err != nil {
		return "error: " + err.Error(), false
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	status, _ := result["status"].(string)

	switch status {
	case "completed":
		return status, true
	case "failed":
		return status, true
	default:
		return status, false
	}
}

func finalizeGoal(goalSvc, goalID string) {
	resp, err := http.Post(goalSvc+"/goals/"+goalID+"/finalize", "application/json", nil)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func approveAll(accessControlAddr string) {
	resp, err := http.Get(accessControlAddr + "/approvals/pending")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var pending []map[string]any
	json.NewDecoder(resp.Body).Decode(&pending)

	for _, item := range pending {
		id, ok := item["id"].(string)
		if !ok {
			continue
		}
		req, _ := http.NewRequest("POST", accessControlAddr+"/approvals/"+id+"/approve", nil)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}
		r.Body.Close()
		fmt.Printf("[auto-approve] Approved %s\n", id)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
