package mcp

import (
	"context"
	"testing"

	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
)

type runtimeTestSyncer struct{}

func (runtimeTestSyncer) Sync(context.Context, string) (*hivesync.Result, error) {
	return &hivesync.Result{}, nil
}

func (runtimeTestSyncer) Drain(context.Context, string, hivesync.TriggerPolicy) (*hivesync.Result, hivesync.DrainOutcome, error) {
	return &hivesync.Result{}, hivesync.DrainOutcome{BatchesDone: 1, State: hivesync.DrainFullySynced}, nil
}

func TestSyncRuntimeReloadsWhenConfigFingerprintChanges(t *testing.T) {
	configs := []*hivesync.Config{
		{APIURL: "https://one.example.com", Email: "one@example.com", Password: "one", AutoSync: true},
		{APIURL: "https://two.example.com", Email: "two@example.com", Password: "two", AutoSync: false},
	}
	loadCount := 0
	factoryCount := 0
	runtime := &syncRuntime{
		reload: true,
		loader: func() (*hivesync.Config, hivesync.SyncConfigStatus, error) {
			idx := loadCount
			if idx >= len(configs) {
				idx = len(configs) - 1
			}
			loadCount++
			cfg := configs[idx]
			return cfg, hivesync.SyncConfigStatus{Configured: true, Source: hivesync.ConfigSourceFile, AutoSync: cfg.AutoSync}, nil
		},
		factory: func(*hivesync.Config, hivesync.SyncStore) SyncRunner {
			factoryCount++
			return runtimeTestSyncer{}
		},
	}

	_, firstStatus, err := runtime.current()
	if err != nil {
		t.Fatalf("first current() error = %v", err)
	}
	_, secondStatus, err := runtime.current()
	if err != nil {
		t.Fatalf("second current() error = %v", err)
	}

	if firstStatus.Source != hivesync.ConfigSourceFile || !firstStatus.AutoSync {
		t.Fatalf("first status = %+v, want file source with auto_sync=true", firstStatus)
	}
	if secondStatus.Source != hivesync.ConfigSourceFile || secondStatus.AutoSync {
		t.Fatalf("second status = %+v, want file source with auto_sync=false", secondStatus)
	}
	if factoryCount != 2 {
		t.Fatalf("factory calls = %d, want 2 after fingerprint change", factoryCount)
	}
}
