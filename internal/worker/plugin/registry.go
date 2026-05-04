package plugin

import (
	"fmt"
	"sync"
)

// Registry manages a collection of JobPlugins.
// It is thread-safe and prevents duplicate registrations.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]JobPlugin
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]JobPlugin),
	}
}

// Register adds a new plugin to the registry.
// Returns an error if a plugin for the same type is already registered.
func (r *Registry) Register(p JobPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	jobType := p.Type()
	if _, exists := r.plugins[jobType]; exists {
		return fmt.Errorf("plugin registry: type %q already registered", jobType)
	}

	r.plugins[jobType] = p
	return nil
}

// Get retrieves the plugin for the given job type.
// Returns an error if no plugin is found.
func (r *Registry) Get(jobType string) (JobPlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.plugins[jobType]
	if !ok {
		return nil, fmt.Errorf("plugin registry: no plugin found for type %q", jobType)
	}

	return p, nil
}

var (
	globalRegistry = NewRegistry()
)

// RegisterGlobal binds a plugin to the system-wide global registry.
// This is intended to be called from a plugin's init() function.
func RegisterGlobal(p JobPlugin) {
	if err := globalRegistry.Register(p); err != nil {
		fmt.Printf("plugin: global registration failed for %q: %v\n", p.Type(), err)
	}
}

// GetGlobalRegistry returns the shared plugin registry.
func GetGlobalRegistry() *Registry {
	return globalRegistry
}
