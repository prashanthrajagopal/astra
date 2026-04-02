package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"astra/internal/evaluation"
)

func newEvalMux() http.Handler {
	eval := evaluation.NewDefault()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /evaluate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TaskID   string `json:"task_id"`
			Result   string `json:"result"`
			Criteria string `json:"criteria"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}
		if req.TaskID == "" {
			http.Error(w, `{"error":"task_id required"}`, http.StatusBadRequest)
			return
		}
		output := []byte(req.Result)
		res, err := eval.EvaluateWithCriteria(r.Context(), req.TaskID, output, req.Criteria)
		if err != nil {
			http.Error(w, `{"error":"evaluation failed"}`, http.StatusInternalServerError)
			return
		}
		passed := res.Result == evaluation.Pass
		resp := map[string]interface{}{
			"passed":   passed,
			"feedback": res.Notes,
			"score":    res.Score,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	return mux
}

func TestEvalHealth(t *testing.T) {
	mux := newEvalMux()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", rec.Body.String())
	}
}

func TestEvalHandler_InvalidJSON(t *testing.T) {
	mux := newEvalMux()
	req := httptest.NewRequest(http.MethodPost, "/evaluate", strings.NewReader("{bad"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestEvalHandler_MissingTaskID(t *testing.T) {
	mux := newEvalMux()
	body, _ := json.Marshal(map[string]interface{}{
		"result":   "some output",
		"criteria": "pass if non-empty",
	})
	req := httptest.NewRequest(http.MethodPost, "/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "task_id") {
		t.Errorf("expected 'task_id' in response, got: %s", rec.Body.String())
	}
}

func TestEvalHandler_ValidRequest(t *testing.T) {
	mux := newEvalMux()
	body, _ := json.Marshal(map[string]interface{}{
		"task_id":  "task-abc-123",
		"result":   `{"output":"success","value":42}`,
		"criteria": "task succeeded",
	})
	req := httptest.NewRequest(http.MethodPost, "/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["passed"]; !ok {
		t.Error("expected 'passed' field in response")
	}
	if _, ok := resp["score"]; !ok {
		t.Error("expected 'score' field in response")
	}
}

func TestEvalHandler_EmptyResult(t *testing.T) {
	mux := newEvalMux()
	body, _ := json.Marshal(map[string]interface{}{
		"task_id":  "task-empty",
		"result":   "",
		"criteria": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	// Empty result is valid input; evaluator decides pass/fail
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for empty result, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestEvalHandler_ContentType(t *testing.T) {
	mux := newEvalMux()
	body, _ := json.Marshal(map[string]interface{}{
		"task_id": "task-ct",
		"result":  "ok",
	})
	req := httptest.NewRequest(http.MethodPost, "/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %q", ct)
	}
}

func TestEvalHandler_ScoreIsNumeric(t *testing.T) {
	mux := newEvalMux()
	body, _ := json.Marshal(map[string]interface{}{
		"task_id":  "task-score",
		"result":   "great output",
		"criteria": "must be non-empty",
	})
	req := httptest.NewRequest(http.MethodPost, "/evaluate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if score, ok := resp["score"].(float64); !ok {
		t.Errorf("expected numeric score, got %T: %v", resp["score"], resp["score"])
	} else if score < 0 || score > 1 {
		t.Errorf("score %v out of [0,1] range", score)
	}
}
