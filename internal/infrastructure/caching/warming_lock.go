// Package caching provides application-wide caching and related utilities.
package caching

import (
	"log"
	"sync"
)

// WarmingLock provides a mechanism to prevent "thundering herd" problems
// by ensuring only one background cache warming task runs for a given key at a time.
type WarmingLock struct {
	mu    sync.Mutex
	locks map[string]struct{}
}

// NewWarmingLock creates a new instance of a WarmingLock.
func NewWarmingLock() *WarmingLock {
	return &WarmingLock{
		locks: make(map[string]struct{}),
	}
}

// TryLock attempts to acquire a lock for a given key.
// It returns true if the lock was acquired, and false if the lock is already held.
// This operation is non-blocking.
func (l *WarmingLock) TryLock(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, exists := l.locks[key]; exists {
		// Lock is already held by another process.
		return false
	}

	// Acquire the lock.
	l.locks[key] = struct{}{}
	return true
}

// Unlock releases a lock for a given key.
// This should be called with `defer` in the goroutine that acquired the lock.
func (l *WarmingLock) Unlock(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.locks, key)
}

// --- Global Singleton Instance ---

var (
	globalWarmingLock *WarmingLock
	lockOnce          sync.Once
)

// GetGlobalWarmingLock initializes and/or returns the global singleton warming lock manager.
func GetGlobalWarmingLock() *WarmingLock {
	lockOnce.Do(func() {
		globalWarmingLock = NewWarmingLock()
		log.Println("Global warming lock manager initialized")
	})
	return globalWarmingLock
}
