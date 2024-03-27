package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLabelsOperations(t *testing.T) {
	key := "name"
	value := "test"

	// Init an empty var
	labels := Labels{}

	// Add a label
	labels.Add(key, value)

	// Check value exists
	existingValue, exists := labels[key]
	assert.True(t, exists)
	assert.Equal(t, value, existingValue)

	// Delete an entry
	labels.Delete(key)

	// Make sure the entry was deleted
	_, exists = labels[key]
	assert.False(t, exists)
}

func TestAddLabelEmptyMap(t *testing.T) {
	key := "foo"
	value := "bar"

	instance := Instance{
		Name:     "test",
		Provider: "test",
	}

	instance.Labels.Add(key, value)

	expectedVal, exists := instance.Labels[key]
	assert.True(t, exists)
	assert.Equal(t, value, expectedVal)
}

func TestDeleteLabelEmptyMap(t *testing.T) {
	key := "foo"

	instance := Instance{
		Name:     "test",
		Provider: "test",
	}

	instance.Labels.Delete(key)

	_, exists := instance.Labels[key]
	assert.False(t, exists)
}
