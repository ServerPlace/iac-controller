package async

import (
	"fmt"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: map[string]Handler{}}
}

func (r *Registry) Register(kind string, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[kind] = h
}

func (r *Registry) Get(kind string) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[kind]
	return h, ok
}

func (r *Registry) MustGet(kind string) Handler {
	h, ok := r.Get(kind)
	if !ok {
		panic(fmt.Sprintf("async: no handler registered for kind=%q", kind))
	}
	return h
}
