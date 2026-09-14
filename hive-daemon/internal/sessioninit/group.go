// Package sessioninit coalesces standalone session initialization requests.
package sessioninit

import (
	"context"
	"errors"
	"sync"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

var errInitializationFailed = errors.New("session initialization failed")

// Key identifies one validated session initialization flight.
type Key struct {
	Project string
	ID      string
}

// Group shares only concurrent standalone initializations with the same key.
type Group struct {
	mu      sync.Mutex
	flights map[Key]*flight
}

type flight struct {
	done    chan struct{}
	session *models.Session
	err     error
}

// NewGroup returns a group with no in-flight initializations.
func NewGroup() *Group {
	return &Group{flights: make(map[Key]*flight)}
}

// Do runs initialize once for concurrent callers with key. A cancelled follower
// leaves without affecting the shared initialization.
func (g *Group) Do(ctx context.Context, key Key, initialize func(context.Context) (*models.Session, error)) (*models.Session, error) {
	g.mu.Lock()
	if current := g.flights[key]; current != nil {
		g.mu.Unlock()
		select {
		case <-current.done:
			return cloneSession(current.session), current.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	current := &flight{done: make(chan struct{})}
	g.flights[key] = current
	g.mu.Unlock()

	defer func() {
		if recovered := recover(); recovered != nil {
			g.complete(key, current, nil, errInitializationFailed)
			panic(recovered)
		}
	}()

	if initialize == nil {
		g.complete(key, current, nil, errInitializationFailed)
		return nil, errInitializationFailed
	}
	session, err := initialize(ctx)
	g.complete(key, current, session, err)
	return cloneSession(session), err
}

func (g *Group) complete(key Key, current *flight, session *models.Session, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	current.session = cloneSession(session)
	current.err = err
	if g.flights[key] == current {
		delete(g.flights, key)
	}
	close(current.done)
}

func cloneSession(session *models.Session) *models.Session {
	if session == nil {
		return nil
	}
	copy := *session
	if session.EndedAt != nil {
		endedAt := *session.EndedAt
		copy.EndedAt = &endedAt
	}
	if session.SyncedAt != nil {
		syncedAt := *session.SyncedAt
		copy.SyncedAt = &syncedAt
	}
	return &copy
}
