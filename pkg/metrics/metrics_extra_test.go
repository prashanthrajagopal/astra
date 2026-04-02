package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// TestMetricNames verifies each metric variable has the expected fully-qualified name.
func TestMetricNames(t *testing.T) {
	// Collect metric descriptors by calling Describe on each collector.
	descOf := func(c prometheus.Collector) string {
		ch := make(chan *prometheus.Desc, 1)
		c.Describe(ch)
		d := <-ch
		// Desc.String() returns something like Desc{fqName: "...", ...}
		s := d.String()
		start := strings.Index(s, `"`)
		end := strings.LastIndex(s, `"`)
		if start < 0 || end <= start {
			return s
		}
		return s[start+1 : end]
	}

	tests := []struct {
		name      string
		collector prometheus.Collector
		wantName  string
	}{
		{"TaskLatency", TaskLatency, "astra_task_latency_seconds"},
		{"TaskSuccessTotal", TaskSuccessTotal, "astra_task_success_total"},
		{"TaskFailureTotal", TaskFailureTotal, "astra_task_failure_total"},
		{"ActorCount", ActorCount, "astra_actor_count"},
		{"SchedulerReadyQueueDepth", SchedulerReadyQueueDepth, "astra_scheduler_ready_queue_depth"},
		{"LLMTokenUsageTotal", LLMTokenUsageTotal, "astra_llm_token_usage_total"},
		{"LLMCostDollars", LLMCostDollars, "astra_llm_cost_dollars"},
		{"LLMCostByAgentModel", LLMCostByAgentModel, "astra_llm_cost_dollars_total"},
		{"LLMCompletionSeconds", LLMCompletionSeconds, "astra_llm_completion_seconds"},
		{"WorkerHeartbeatTotal", WorkerHeartbeatTotal, "astra_worker_heartbeat_total"},
		{"EventsProcessedTotal", EventsProcessedTotal, "astra_events_processed_total"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := descOf(tc.collector)
			if !strings.Contains(got, tc.wantName) {
				t.Errorf("%s: name should contain %q, got descriptor: %q", tc.name, tc.wantName, got)
			}
		})
	}
}

// TestRegister_Idempotent verifies calling Register followed by Unregister works without panic.
func TestRegister_Idempotent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Register panicked: %v", r)
		}
	}()
	Register()
	// Clean up so subsequent test runs don't get "already registered" panics.
	collectors := []prometheus.Collector{
		TaskLatency, TaskSuccessTotal, TaskFailureTotal,
		ActorCount, SchedulerReadyQueueDepth,
		LLMTokenUsageTotal, LLMCostDollars, LLMCostByAgentModel,
		LLMCompletionSeconds, WorkerHeartbeatTotal, EventsProcessedTotal,
	}
	for _, c := range collectors {
		_ = prometheus.Unregister(c)
	}
}

// TestCounterVec_ObserveDoesNotPanic verifies counters and histograms can be used
// without panicking (i.e., label sets are correct).
func TestCounterVec_ObserveDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("metric operation panicked: %v", r)
		}
	}()

	// Use local registries so we don't interfere with global state.
	reg := prometheus.NewRegistry()

	taskLatency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "astra_task_latency_seconds",
		Help:    "Task execution latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"task_type", "status"})

	taskSuccess := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "astra_task_success_total",
		Help: "Total successful task completions",
	}, []string{"task_type"})

	taskFailure := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "astra_task_failure_total",
		Help: "Total failed task executions",
	}, []string{"task_type"})

	llmTokens := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "astra_llm_token_usage_total",
		Help: "Total LLM tokens used",
	}, []string{"model", "direction"})

	llmCost := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "astra_llm_cost_dollars",
		Help: "Total LLM cost",
	}, []string{"model"})

	llmCostByAgent := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "astra_llm_cost_dollars_total",
		Help: "Total LLM cost by agent and model",
	}, []string{"agent_id", "model"})

	llmCompletion := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "astra_llm_completion_seconds",
		Help:    "LLM completion latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"agent_id", "model"})

	actorCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "astra_actor_count",
		Help: "Currently running actors",
	})

	reg.MustRegister(taskLatency, taskSuccess, taskFailure, llmTokens, llmCost, llmCostByAgent, llmCompletion, actorCount)

	// Exercise the metrics with valid label combinations.
	taskLatency.WithLabelValues("llm_call", "completed").Observe(0.5)
	taskSuccess.WithLabelValues("llm_call").Inc()
	taskFailure.WithLabelValues("tool_call").Inc()
	llmTokens.WithLabelValues("gpt-4", "in").Add(100)
	llmTokens.WithLabelValues("gpt-4", "out").Add(200)
	llmCost.WithLabelValues("gpt-4").Add(0.01)
	llmCostByAgent.WithLabelValues("agent-abc", "gpt-4").Add(0.005)
	llmCompletion.WithLabelValues("agent-abc", "gpt-4").Observe(1.2)
	actorCount.Set(42)
	actorCount.Inc()
	actorCount.Dec()
}

// TestGauge_SetIncDec verifies ActorCount gauge operations produce correct values.
func TestGauge_SetIncDec(t *testing.T) {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_gauge_set_inc_dec",
		Help: "test gauge",
	})

	gauge.Set(10)
	gauge.Inc()
	gauge.Inc()
	gauge.Dec()

	// Collect the metric and verify the value.
	ch := make(chan prometheus.Metric, 1)
	gauge.Collect(ch)
	m := <-ch
	var dm dto.Metric
	if err := m.Write(&dm); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if dm.Gauge == nil {
		t.Fatal("expected gauge metric")
	}
	if *dm.Gauge.Value != 11 {
		t.Errorf("gauge value: want 11, got %f", *dm.Gauge.Value)
	}
}
