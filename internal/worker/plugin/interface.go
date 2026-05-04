package plugin

// JobPlugin defines the contract for job execution units.
// This interface allows for an extensible system where new job types
// can be added as self-contained plugins.
type JobPlugin interface {
	// Type returns the unique identifier for the job type this plugin handles (e.g., "email").
	Type() string

	// Execute performs the actual work of the job using the provided payload.
	// It returns an optional result and an error if processing fails.
	Execute(payload map[string]interface{}) (interface{}, error)
}
