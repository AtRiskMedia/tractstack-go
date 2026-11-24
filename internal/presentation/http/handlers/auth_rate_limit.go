package handlers

import (
	"sync"
	"time"
)

type authAttempt struct {
	count   int
	resetAt time.Time
}

var (
	authAttemptMu sync.Mutex
	authAttempts  = map[string]*authAttempt{}
)

const (
	authAttemptWindow = time.Minute
	authAttemptLimit  = 20
)

func allowAuthAttempt(tenantID, ip, identity string) bool {
	key := tenantID + "|" + ip + "|" + identity
	now := time.Now()
	authAttemptMu.Lock()
	defer authAttemptMu.Unlock()
	a, ok := authAttempts[key]
	if !ok || now.After(a.resetAt) {
		authAttempts[key] = &authAttempt{count: 1, resetAt: now.Add(authAttemptWindow)}
		return true
	}
	if a.count >= authAttemptLimit {
		return false
	}
	a.count++
	return true
}
