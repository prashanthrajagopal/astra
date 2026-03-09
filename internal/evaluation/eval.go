package evaluation

import "context"

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

var _ Evaluator = (*DefaultEvaluator)(nil)
