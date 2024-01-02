package reflect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidMetrics(t *testing.T) {
	t.Run("reflection counter is correct", func(t *testing.T) {
		t.Parallel()
		vals := []string{"create", "secret", "false", "default"}
		reflectorReflections.WithLabelValues(vals...).Inc()
		m, err := reflectorReflections.GetMetricWithLabelValues(vals...)
		require.Nil(t, err)
		assert.Equal(t, "Desc{fqName: \"reflector_reflections_reflected_total\", help: \"The number of total reflections since the start of the reflector\", constLabels: {}, variableLabels: {reflection_action,secret,success,namespace}}", m.Desc().String())
	})

	t.Run("reflection secret latency is correct", func(t *testing.T) {
		t.Parallel()
		vals := []string{"sec", "default"}
		reflectorSecretLatency.WithLabelValues(vals...).Observe(3)
		m, err := reflectorSecretLatency.MetricVec.GetMetricWithLabelValues(vals...)
		require.Nil(t, err)
		assert.Equal(t, "Desc{fqName: \"reflector_reflections_reflect_latency\", help: \"The latency for the reflection of a single secret\", constLabels: {}, variableLabels: {secret,namespace}}", m.Desc().String())
	})

	t.Run("reflection latency is correct", func(t *testing.T) {
		t.Parallel()
		vals := []string{"sec"}
		reflectorReflectionLatency.WithLabelValues(vals...).Observe(10)
		m, err := reflectorReflectionLatency.MetricVec.GetMetricWithLabelValues(vals...)
		require.Nil(t, err)
		assert.Equal(t, "Desc{fqName: \"reflector_reflections_reflection_latency\", help: \"The latency from when a reflection is detected, to when it is completely reflected\", constLabels: {}, variableLabels: {secret}}", m.Desc().String())
	})
}
