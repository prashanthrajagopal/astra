import { Page, APIRequestContext, expect } from '@playwright/test';

export const DASHBOARD_URL = '/superadmin/dashboard/';
export const LOGIN_URL = '/login';

export interface TestUser {
  email: string;
  password: string;
  token: string;
}

/**
 * Login via API and store token in localStorage.
 * Navigates to /health (minimal same-origin page) to establish localStorage access,
 * sets the token, then navigates to the dashboard.
 */
export async function loginAsAdmin(page: Page): Promise<string> {
  const email = process.env.ASTRA_TEST_EMAIL || 'admin@astra.local';
  const password = process.env.ASTRA_TEST_PASSWORD || 'changeme-admin';

  // Get a real JWT token from the identity service (retry once on transient failure)
  let token = '';
  let user = { email, name: 'Admin', is_super_admin: true };
  for (let attempt = 0; attempt < 3; attempt++) {
    const response = await page.request.post('/login', {
      data: { email, password },
    });
    if (response.ok()) {
      const body = await response.json();
      token = body.token || '';
      user = body.user || user;
      break;
    }
    // Wait before retry on transient failure
    await new Promise(r => setTimeout(r, 1000));
  }
  if (!token) {
    throw new Error('Login failed after 3 attempts');
  }

  // Navigate to /health (plain text, no JS) to establish same-origin localStorage
  await page.goto('/health', { waitUntil: 'load' });

  // Set token in localStorage — persists across same-origin navigations
  await page.evaluate(({ t, u }) => {
    localStorage.setItem('astra_token', t);
    localStorage.setItem('astra_user', JSON.stringify(u));
  }, { t: token, u: user });

  return token;
}

/**
 * Login and navigate to dashboard
 */
export async function loginAndGotoDashboard(page: Page): Promise<void> {
  await loginAsAdmin(page);
  await page.goto(DASHBOARD_URL, { waitUntil: 'load', timeout: 15_000 });
  // Verify we're on the dashboard, not redirected to login
  await page.waitForSelector('body.dashboard-redesign, .topnav, .stats-grid', { timeout: 10_000 });
}

/**
 * Get auth headers for API requests
 */
export async function getAuthHeaders(request: APIRequestContext): Promise<Record<string, string>> {
  const email = process.env.ASTRA_TEST_EMAIL || 'admin@astra.local';
  const password = process.env.ASTRA_TEST_PASSWORD || 'changeme-admin';

  for (let attempt = 0; attempt < 3; attempt++) {
    const response = await request.post('/login', {
      data: { email, password },
    });
    if (response.ok()) {
      const body = await response.json();
      return {
        'Authorization': `Bearer ${body.token}`,
        'X-Is-Super-Admin': 'true',
        'X-User-Id': body.user?.id || 'test-user',
      };
    }
    await new Promise(r => setTimeout(r, 1000));
  }
  return { 'X-Is-Super-Admin': 'true', 'X-User-Id': 'test-user' };
}

/**
 * Wait for dashboard data to load (stats populated)
 */
export async function waitForDashboardData(page: Page): Promise<void> {
  // Wait for the stats grid to be visible — data may be 0 but elements should exist
  await page.waitForSelector('.stats-grid .stat-card', { timeout: 10_000 });
  // Give the async fetch a moment to complete
  await page.waitForTimeout(1500);
}

/**
 * Switch dashboard tab
 */
export async function switchTab(page: Page, tabName: string): Promise<void> {
  await page.click(`.nav-tab[data-tab="${tabName}"]`);
  await page.waitForTimeout(300);
}

/**
 * Create a test agent via API
 */
export async function createTestAgent(request: APIRequestContext, headers: Record<string, string>, name?: string): Promise<string> {
  const agentName = name || `pw-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const response = await request.post('/agents', {
    headers,
    data: { name: agentName },
  });
  if (response.ok()) {
    const body = await response.json();
    return body.id || body.agent_id || '';
  }
  return '';
}

/**
 * Delete a test agent via API
 */
export async function deleteTestAgent(request: APIRequestContext, headers: Record<string, string>, agentId: string): Promise<void> {
  await request.delete(`/agents/${agentId}`, { headers });
}
