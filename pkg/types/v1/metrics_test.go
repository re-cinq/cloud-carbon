package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricsParser(t *testing.T) {
	// Create a valid resource
	cpuResource := &Metric{
		Name:         "cpu",
		Usage:        20.54,
		UnitAmount:   4.0,
		ResourceType: CPU,
		Unit:         VCPU,
		Emissions: ResourceEmissions{
			Value: 1056.76,
			Unit:  GCO2eqkWh,
		},
	}
	assert.NotNil(t, cpuResource)

	metrics := Metrics{}
	metrics.Upsert(cpuResource)

	existing := metrics[cpuResource.Name]
	assert.Equal(t, *cpuResource, existing)
}

func TestAddMetricEmptyMap(t *testing.T) {
	name := Memory.String()
	metric := NewMetric(name)

	instance := Instance{
		Name:     "test",
		Provider: "test",
	}

	instance.Metrics.Upsert(metric)

	expectedMetric, exists := instance.Metrics[metric.Name]
	assert.True(t, exists)
	assert.Equal(t, name, expectedMetric.Name)
}
