package v1

// Represents the metrics for a specific service
// The key is the unique name of the resource
type Metrics map[string]Metric

// Helper method for adding a specific metric
func (m *Metrics) Upsert(metric *Metric) {
	if metric == nil {
		return
	}

	// if the map doesn't exist, initialize it
	if *m == nil {
		*m = make(Metrics)
	}

	(*m)[metric.Name] = *metric
}
