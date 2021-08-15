package reflect

import "github.com/prometheus/client_golang/prometheus"

const Namespace = "reflector"
const SubsystemReflections = "reflections"

var (
	reflectorReflections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: SubsystemReflections,
			Name:      "reflected_total",
			Help:      "The number of total reflections since the start of the reflector",
		},
		[]string{"type", "secret"},
	)
)

func init() {
	prometheus.MustRegister(reflectorReflections)
	// Add Go module build info.
	prometheus.MustRegister(prometheus.NewBuildInfoCollector())
}
