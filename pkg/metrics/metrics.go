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
)

func Register() {
	prometheus.MustRegister(
		TaskLatency,
		TaskSuccessTotal,
		TaskFailureTotal,
		ActorCount,
		SchedulerReadyQueueDepth,
	)
}
