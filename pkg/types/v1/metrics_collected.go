package v1

import bus "github.com/re-cinq/go-bus"

type MetricsCollected struct {
	bus.Event
	Instance Service
}

// The topic this event is about
func (e MetricsCollected) Topic() bus.Topic {
	return MetricsCollectedTopic
}

// Returns the unique name of the instance or service
func (e MetricsCollected) Identifier() string {
	return e.Instance.name
}