package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstanceOperations(t *testing.T) {
	// Make sure we get a nil back in case the instance name is empty
	failedInstance := NewInstance("", Prometheus)
	assert.Nil(t, failedInstance)

	// Mocking the instance id
	instanceID := "1234"

	// Mocking the metric
	r := &Metric{
		Name:         CPU.String(),
		Usage:        170.4,
		UnitAmount:   4.0,
		ResourceType: CPU,
		Unit:         VCPU,
		Emissions:    NewResourceEmission(1024.57, GCO2eqkWh),
	}

	// Create a new instance
	instance := NewInstance(instanceID, Prometheus)
	instance.Region = "europe-west4"
	instance.Kind = "n2-standard-8"

	// Make sure the region is assigned correctly
	assert.Equal(t, instanceID, instance.Name)
	assert.Equal(t, Prometheus, instance.Provider)
	assert.Equal(t, "europe-west4", instance.Region)
	assert.Equal(t, "n2-standard-8", instance.Kind)

	// Add the metrics
	instance.Metrics.Upsert(r)

	// Add a label
	instance.Labels.Add("name", "test")

	// Make sure the label exists
	_, exists := instance.Labels["name"]
	assert.True(t, exists)

	// Check the resource was added
	existingResource, exists := instance.Metrics[r.Name]

	// Make sure the resource exists
	assert.True(t, exists)

	// Make sure the resource is the same
	assert.Equal(t, *r, existingResource)
}
