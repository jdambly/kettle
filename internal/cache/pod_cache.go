package cache

import (
	"sync"
)

// PodCache represents a single pod's cache entry
type PodCache struct {
	Name        string
	Namespace   string
	IP          string
	Annotations map[string]string
}

// PodList manages a list of PodCache entries in a thread-safe way
type PodList struct {
	mu    sync.RWMutex
	items map[string]PodCache
}

var (
	sharedPodList     *PodList
	sharedPodListOnce sync.Once
)

// GetSharedPodList returns the singleton instance of PodList
func GetSharedPodList() *PodList {
	sharedPodListOnce.Do(func() {
		sharedPodList = NewPodList()
	})
	return sharedPodList
}

// NewPodList creates and initializes a new PodList
func NewPodList() *PodList {
	return &PodList{
		items: make(map[string]PodCache),
	}
}

// Add adds or updates a PodCache entry
func (p *PodList) Add(cache PodCache) {
	key := p.getKey(cache.Namespace, cache.Name)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items[key] = cache
}

// Get retrieves a PodCache entry by namespace and name
func (p *PodList) Get(namespace, name string) (PodCache, bool) {
	key := p.getKey(namespace, name)
	p.mu.RLock()
	defer p.mu.RUnlock()
	cache, exists := p.items[key]
	return cache, exists
}

// Delete removes a PodCache entry
func (p *PodList) Delete(namespace, name string) {
	key := p.getKey(namespace, name)
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.items, key)
}

// getKey generates a unique key for a namespace and name
func (p *PodList) getKey(namespace, name string) string {
	return namespace + "/" + name
}
