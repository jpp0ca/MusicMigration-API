package adapters

import (
	"fmt"
	"sync"

	"github.com/jpp0ca/MusicMigration-API/internal/ports"
)

// ProviderRegistry maps provider names to their MusicProvider implementations.
// It is safe for concurrent use.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]ports.MusicProvider
}

// NewProviderRegistry creates an empty registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]ports.MusicProvider),
	}
}

// Register adds a provider to the registry, keyed by its Name().
func (r *ProviderRegistry) Register(provider ports.MusicProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.Name()] = provider
}

// Get returns the provider for the given name, or an error if not found.
func (r *ProviderRegistry) Get(name string) (ports.MusicProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
	return provider, nil
}

// Available returns the names of all registered providers.
func (r *ProviderRegistry) Available() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
