package sessioninit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/models"
)

type joiningContext struct {
	context.Context
	joined chan struct{}
	once   sync.Once
}

func (c *joiningContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.joined) })
	return c.Context.Done()
}

func waitForFollower(t *testing.T, joined <-chan struct{}, unexpectedLeader <-chan struct{}) {
	t.Helper()
	select {
	case <-joined:
	case <-unexpectedLeader:
		t.Fatal("follower became a second leader")
	}
}

func TestGroupDo_SharesOneDetachedResultAndCleansUp(t *testing.T) {
	group := NewGroup()
	key := Key{Project: "canonical-project", ID: "exact-id"}
	started, release := make(chan struct{}), make(chan struct{})
	var calls int
	var callsMu sync.Mutex
	initialize := func(context.Context) (*models.Session, error) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		close(started)
		<-release
		ended := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
		return &models.Session{ID: key.ID, Project: key.Project, Summary: "first", EndedAt: &ended}, nil
	}

	results, errs := make(chan *models.Session, 2), make(chan error, 2)
	go func() {
		session, err := group.Do(context.Background(), key, initialize)
		results <- session
		errs <- err
	}()
	<-started
	joined, unexpectedLeader := make(chan struct{}), make(chan struct{})
	go func() {
		session, err := group.Do(&joiningContext{Context: context.Background(), joined: joined}, key, func(context.Context) (*models.Session, error) {
			unexpectedLeader <- struct{}{}
			return nil, nil
		})
		results <- session
		errs <- err
	}()
	waitForFollower(t, joined, unexpectedLeader)
	close(release)

	first, second := <-results, <-results
	if err := <-errs; err != nil {
		t.Fatalf("first result error = %v", err)
	}
	if err := <-errs; err != nil {
		t.Fatalf("second result error = %v", err)
	}
	callsMu.Lock()
	gotCalls := calls
	callsMu.Unlock()
	if gotCalls != 1 {
		t.Fatalf("initializer calls = %d, want 1", gotCalls)
	}
	if first == second || first.EndedAt == second.EndedAt || first.ID != key.ID || second.Summary != "first" {
		t.Fatalf("results must be equal detached snapshots: first=%+v second=%+v", first, second)
	}
	first.Summary, *first.EndedAt = "mutated", time.Time{}
	if second.Summary != "first" || second.EndedAt.IsZero() {
		t.Fatalf("mutating one result changed another: %+v", second)
	}

	if _, err := group.Do(context.Background(), key, func(context.Context) (*models.Session, error) {
		callsMu.Lock()
		calls++
		callsMu.Unlock()
		return &models.Session{ID: key.ID, Project: key.Project, Summary: "retry"}, nil
	}); err != nil {
		t.Fatalf("retry after cleanup: %v", err)
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	if calls != 2 {
		t.Fatalf("calls after cleanup = %d, want 2", calls)
	}
}

func TestGroupDo_IsolatesKeysSharesErrorsAndLetsFollowersCancel(t *testing.T) {
	group := NewGroup()
	key := Key{Project: "canonical-project", ID: "exact-id"}
	alias := Key{Project: "canonical-project", ID: "exact-id"}
	other := Key{Project: "other-project", ID: "exact-id"}
	started, release := make(chan struct{}), make(chan struct{})
	shared, leaderErr := errors.New("store unavailable"), make(chan error, 1)
	go func() {
		_, err := group.Do(context.Background(), key, func(context.Context) (*models.Session, error) {
			close(started)
			<-release
			return nil, shared
		})
		leaderErr <- err
	}()
	<-started

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := group.Do(cancelled, key, func(context.Context) (*models.Session, error) {
		t.Fatal("cancelled follower became leader")
		return nil, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled follower error = %v, want context cancellation", err)
	}
	otherSession, err := group.Do(context.Background(), other, func(context.Context) (*models.Session, error) {
		return &models.Session{ID: other.ID, Project: other.Project}, nil
	})
	if err != nil || otherSession.Project != other.Project {
		t.Fatalf("independent key result = %+v, %v", otherSession, err)
	}

	errorResult := make(chan error, 1)
	joined, unexpectedLeader := make(chan struct{}), make(chan struct{})
	go func() {
		_, err := group.Do(&joiningContext{Context: context.Background(), joined: joined}, alias, func(context.Context) (*models.Session, error) {
			unexpectedLeader <- struct{}{}
			return nil, nil
		})
		errorResult <- err
	}()
	waitForFollower(t, joined, unexpectedLeader)
	close(release)
	if err := <-leaderErr; !errors.Is(err, shared) {
		t.Fatalf("leader error = %v, want shared initializer error", err)
	}
	if err := <-errorResult; !errors.Is(err, shared) {
		t.Fatalf("joined error = %v, want shared initializer error", err)
	}
	if _, err := group.Do(context.Background(), key, func(context.Context) (*models.Session, error) { return &models.Session{}, nil }); err != nil {
		t.Fatalf("retry after error: %v", err)
	}
}

func TestGroupDo_PanicWakesWaitersAndAllowsRetry(t *testing.T) {
	group := NewGroup()
	key := Key{Project: "canonical-project", ID: "exact-id"}
	started, release := make(chan struct{}), make(chan struct{})
	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		defer func() { _ = recover() }()
		_, _ = group.Do(context.Background(), key, func(context.Context) (*models.Session, error) {
			close(started)
			<-release
			panic("unexpected initializer panic")
		})
	}()
	<-started
	waiter := make(chan error, 1)
	joined, unexpectedLeader := make(chan struct{}), make(chan struct{})
	go func() {
		_, err := group.Do(&joiningContext{Context: context.Background(), joined: joined}, key, func(context.Context) (*models.Session, error) {
			unexpectedLeader <- struct{}{}
			return nil, nil
		})
		waiter <- err
	}()
	waitForFollower(t, joined, unexpectedLeader)
	close(release)
	if err := <-waiter; err == nil || err.Error() != "session initialization failed" {
		t.Fatalf("waiter panic error = %v", err)
	}
	<-leaderDone
	if _, err := group.Do(context.Background(), key, func(context.Context) (*models.Session, error) { return &models.Session{}, nil }); err != nil {
		t.Fatalf("retry after panic: %v", err)
	}
}
