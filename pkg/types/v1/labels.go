package v1

// Labels definition
type Labels map[string]string

// Helper method for adding a label
func (l *Labels) Add(key, value string) {
	// Initialize the map if it doesn't exist
	// Pointers are used to update the value in
	// the address if the Labels type is embedded
	if *l == nil {
		*l = make(Labels)
	}

	(*l)[key] = value
}

// Helper method for deleting a specific label
func (l *Labels) Delete(key string) {
	// Make sure the map is initialized
	if *l == nil {
		return
	}

	// Delete it
	delete(*l, key)
}
