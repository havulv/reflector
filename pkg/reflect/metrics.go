package reflect

import "github.com/prometheus/client_golang/prometheus"

// Namespace is the namespace for metrics produced by the reflector.
const Namespace = "reflector"

// SubsystemReflections is the subsystem for reflections
const SubsystemReflections = "reflections"

var (
	reflectorReflections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: SubsystemReflections,
			Name:      "reflected_total",
			Help:      "The number of total reflections since the start of the reflector",
		},
		[]string{"reflection_action", "secret", "success", "namespace"},
	)

	reflectorReflectionLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: SubsystemReflections,
			Name:      "reflection_latency",
			Help:      "The latency from when a reflection is detected, to when it is completely reflected",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"secret"},
	)

	reflectorSecretLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: SubsystemReflections,
			Name:      "reflect_latency",
			Help:      "The latency for the reflection of a single secret",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"secret", "namespace"},
	)
)

func init() {
	prometheus.MustRegister(reflectorReflections)
	prometheus.MustRegister(reflectorReflectionLatency)
	prometheus.MustRegister(reflectorSecretLatency)
	// Add Go module build info.
	prometheus.MustRegister(prometheus.NewBuildInfoCollector())
}
