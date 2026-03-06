package compliance

import (
	"fmt"
	"sync"
)

var (
	registry = make(map[string]Factory)
	mu       sync.RWMutex
)

func Register(id string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[id]; exists {
		panic(fmt.Sprintf("rule '%s' already registered", id))
	}
	registry[id] = f
}

func Instantiate(id string, config map[string]interface{}) (Rule, error) {
	mu.RLock()
	defer mu.RUnlock()
	factory, exists := registry[id]
	if !exists {
		return nil, fmt.Errorf("rule '%s' not found", id)
	}
	return factory(config)
}
