package mcp

import (
	"fmt"
	"sync"

	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
)

const configSourceInjected = "injected"

type syncRuntime struct {
	mu        sync.Mutex
	store     hivesync.SyncStore
	reload    bool
	syncer    SyncRunner
	signature string
	status    hivesync.SyncConfigStatus
	loader    func() (*hivesync.Config, hivesync.SyncConfigStatus, error)
	factory   func(*hivesync.Config, hivesync.SyncStore) SyncRunner
}

func newSyncRuntime(store hivesync.SyncStore, syncer SyncRunner, cfg *hivesync.Config) *syncRuntime {
	status := hivesync.SyncConfigStatus{Configured: false, Source: hivesync.ConfigSourceNone}
	if cfg != nil {
		status = hivesync.SyncConfigStatus{Configured: true, Source: configSourceInjected, AutoSync: cfg.AutoSync}
	}
	return &syncRuntime{
		store:     store,
		reload:    store != nil,
		syncer:    syncer,
		signature: syncConfigSignature(cfg),
		status:    status,
		loader:    hivesync.LoadWithStatus,
		factory: func(cfg *hivesync.Config, store hivesync.SyncStore) SyncRunner {
			return hivesync.New(cfg, store)
		},
	}
}

func (r *syncRuntime) current() (SyncRunner, hivesync.SyncConfigStatus, error) {
	if r == nil {
		return nil, hivesync.SyncConfigStatus{Configured: false, Source: hivesync.ConfigSourceNone}, nil
	}

	if !r.reload {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.syncer, r.status, nil
	}

	cfg, status, err := r.loader()
	if err != nil {
		r.mu.Lock()
		r.status = status
		r.syncer = nil
		r.signature = ""
		r.mu.Unlock()
		return nil, status, err
	}
	if cfg == nil || !status.Configured {
		r.mu.Lock()
		r.status = status
		r.syncer = nil
		r.signature = ""
		r.mu.Unlock()
		return nil, status, nil
	}

	signature := syncConfigSignature(cfg)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.syncer == nil || r.signature != signature {
		r.syncer = r.factory(cfg, r.store)
		r.signature = signature
	}
	r.status = status
	return r.syncer, status, nil
}

func syncConfigSignature(cfg *hivesync.Config) string {
	if cfg == nil {
		return ""
	}
	return fmt.Sprintf("%s\x00%s\x00%s\x00%t", cfg.APIURL, cfg.Email, cfg.Password, cfg.AutoSync)
}
