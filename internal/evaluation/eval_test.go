package evaluation

import (
	"context"
	"testing"
)

func TestDefaultEvaluator_Evaluate(t *testing.T) {
	e := NewDefault()
	ctx := context.Background()

	res, err := e.Evaluate(ctx, "task-1", []byte("hello"))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if res.Result != Pass {
		t.Errorf("expected Pass, got %s", res.Result)
	}
	if res.Score != 1.0 {
		t.Errorf("expected score 1.0, got %f", res.Score)
	}

	res, err = e.Evaluate(ctx, "task-2", []byte{})
	if err != nil {
		t.Fatalf("Evaluate empty: %v", err)
	}
	if res.Result != Fail {
		t.Errorf("expected Fail for empty, got %s", res.Result)
	}
}

func TestDefaultEvaluator_EvaluateWithCriteria(t *testing.T) {
	e := NewDefault()
	ctx := context.Background()

	// No criteria: uses default
	res, err := e.EvaluateWithCriteria(ctx, "t1", []byte("ok"), "")
	if err != nil {
		t.Fatalf("EvaluateWithCriteria: %v", err)
	}
	if res.Result != Pass {
		t.Errorf("expected Pass, got %s", res.Result)
	}

	// Substring match
	res, err = e.EvaluateWithCriteria(ctx, "t2", []byte("hello world"), "world")
	if err != nil {
		t.Fatalf("EvaluateWithCriteria: %v", err)
	}
	if res.Result != Pass {
		t.Errorf("expected Pass for substring, got %s", res.Result)
	}

	// Substring no match
	res, err = e.EvaluateWithCriteria(ctx, "t3", []byte("hello"), "xyz")
	if err != nil {
		t.Fatalf("EvaluateWithCriteria: %v", err)
	}
	if res.Result != Fail {
		t.Errorf("expected Fail for no substring, got %s", res.Result)
	}

	// Regex match
	res, err = e.EvaluateWithCriteria(ctx, "t4", []byte("error: 404"), `\d+`)
	if err != nil {
		t.Fatalf("EvaluateWithCriteria: %v", err)
	}
	if res.Result != Pass {
		t.Errorf("expected Pass for regex, got %s", res.Result)
	}

	// Regex no match
	res, err = e.EvaluateWithCriteria(ctx, "t5", []byte("hello"), `\d+`)
	if err != nil {
		t.Fatalf("EvaluateWithCriteria: %v", err)
	}
	if res.Result != Fail {
		t.Errorf("expected Fail for regex no match, got %s", res.Result)
	}
}
