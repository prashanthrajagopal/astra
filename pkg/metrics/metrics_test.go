package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRegister(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Register panicked: %v", r)
		}
	}()
	Register()
	// Unregister so repeated runs or other tests don't hit "already registered"
	_ = prometheus.Unregister(TaskLatency)
	_ = prometheus.Unregister(TaskSuccessTotal)
	_ = prometheus.Unregister(TaskFailureTotal)
	_ = prometheus.Unregister(ActorCount)
	_ = prometheus.Unregister(SchedulerReadyQueueDepth)
	_ = prometheus.Unregister(LLMTokenUsageTotal)
	_ = prometheus.Unregister(LLMCostDollars)
}
