package evaluation

import (
	"context"
	"regexp"
	"strings"
)

type Result string

const (
	Pass    Result = "pass"
	Fail    Result = "fail"
	Partial Result = "partial"
)

type EvalResult struct {
	TaskID string
	Result Result
	Score  float64
	Notes  string
}

type Evaluator interface {
	Evaluate(ctx context.Context, taskID string, output []byte) (EvalResult, error)
}

type DefaultEvaluator struct{}

func NewDefault() *DefaultEvaluator {
	return &DefaultEvaluator{}
}

func (e *DefaultEvaluator) Evaluate(ctx context.Context, taskID string, output []byte) (EvalResult, error) {
	if len(output) == 0 {
		return EvalResult{TaskID: taskID, Result: Fail, Notes: "empty output"}, nil
	}
	return EvalResult{TaskID: taskID, Result: Pass, Score: 1.0}, nil
}

// EvaluateWithCriteria evaluates output using DefaultEvaluator logic, or regex match when criteria
// looks like a regex pattern (contains regex metacharacters). Returns EvalResult.
func (e *DefaultEvaluator) EvaluateWithCriteria(ctx context.Context, taskID string, output []byte, criteria string) (EvalResult, error) {
	if criteria == "" {
		return e.Evaluate(ctx, taskID, output)
	}
	// Check if criteria looks like a regex (contains metacharacters).
	if isRegexLike(criteria) {
		re, err := regexp.Compile(criteria)
		if err != nil {
			return EvalResult{TaskID: taskID, Result: Fail, Score: 0, Notes: "invalid regex: " + err.Error()}, nil
		}
		if re.Match(output) {
			return EvalResult{TaskID: taskID, Result: Pass, Score: 1.0, Notes: "regex matched"}, nil
		}
		return EvalResult{TaskID: taskID, Result: Fail, Score: 0, Notes: "regex did not match"}, nil
	}
	// Substring match.
	if strings.Contains(string(output), criteria) {
		return EvalResult{TaskID: taskID, Result: Pass, Score: 1.0, Notes: "substring matched"}, nil
	}
	return EvalResult{TaskID: taskID, Result: Fail, Score: 0, Notes: "substring not found"}, nil
}

func isRegexLike(s string) bool {
	for _, c := range s {
		switch c {
		case '*', '+', '?', '.', '[', ']', '(', ')', '|', '^', '$', '\\':
			return true
		}
	}
	return false
}

var _ Evaluator = (*DefaultEvaluator)(nil)
