package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	TaskLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "astra_task_latency_seconds",
		Help:    "Task execution latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"task_type", "status"})

	TaskSuccessTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "astra_task_success_total",
		Help: "Total successful task completions",
	}, []string{"task_type"})

	TaskFailureTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "astra_task_failure_total",
		Help: "Total failed task executions",
	}, []string{"task_type"})

	ActorCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "astra_actor_count",
		Help: "Currently running actors",
	})

	SchedulerReadyQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "astra_scheduler_ready_queue_depth",
		Help: "Tasks waiting to be scheduled",
	})

	// LLM token usage (direction: in | out).
	LLMTokenUsageTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "astra_llm_token_usage_total",
		Help: "Total LLM tokens used by model and direction (in/out)",
	}, []string{"model", "direction"})

	// LLM cost in dollars (incremented when cost is available).
	LLMCostDollars = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "astra_llm_cost_dollars",
		Help: "Total LLM cost in dollars by model",
	}, []string{"model"})
)

func Register() {
	prometheus.MustRegister(
		TaskLatency,
		TaskSuccessTotal,
		TaskFailureTotal,
		ActorCount,
		SchedulerReadyQueueDepth,
		LLMTokenUsageTotal,
		LLMCostDollars,
	)
}
