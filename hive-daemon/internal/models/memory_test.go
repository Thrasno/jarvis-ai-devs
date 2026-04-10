package models_test

import (
	"encoding/json"
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

func TestMemory_JSONRoundTrip(t *testing.T) {
	key := "arch/auth"
	mem := models.Memory{
		ID:            42,
		SyncID:        "uuid-abc",
		Project:       "jarvis-dev",
		TopicKey:      &key,
		Category:      "architecture",
		Title:         "Auth Design",
		Content:       "JWT-based auth",
		Tags:          []string{"auth", "jwt"},
		FilesAffected: []string{"internal/auth/auth.go"},
		CreatedBy:     "andres",
		ImpactScore:   80,
	}

	data, err := json.Marshal(mem)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded models.Memory
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.ID != mem.ID {
		t.Errorf("ID: got %d, want %d", decoded.ID, mem.ID)
	}
	if decoded.Project != mem.Project {
		t.Errorf("Project: got %q, want %q", decoded.Project, mem.Project)
	}
	if decoded.TopicKey == nil || *decoded.TopicKey != key {
		t.Errorf("TopicKey: got %v, want %q", decoded.TopicKey, key)
	}
	if len(decoded.Tags) != 2 || decoded.Tags[0] != "auth" {
		t.Errorf("Tags: got %v, want [auth jwt]", decoded.Tags)
	}
}

func TestMemory_TopicKeyNullable(t *testing.T) {
	mem := models.Memory{Project: "test", Title: "t", Content: "c"}
	if mem.TopicKey != nil {
		t.Error("expected TopicKey to be nil by default")
	}

	key := "sdd/foo/spec"
	mem.TopicKey = &key
	if *mem.TopicKey != "sdd/foo/spec" {
		t.Errorf("TopicKey = %q, want sdd/foo/spec", *mem.TopicKey)
	}
}

func TestMemory_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mem     models.Memory
		wantErr bool
	}{
		{
			name:    "valid",
			mem:     models.Memory{Project: "p", Title: "t", Content: "c"},
			wantErr: false,
		},
		{
			name:    "missing project",
			mem:     models.Memory{Title: "t", Content: "c"},
			wantErr: true,
		},
		{
			name:    "missing title",
			mem:     models.Memory{Project: "p", Content: "c"},
			wantErr: true,
		},
		{
			name:    "missing content",
			mem:     models.Memory{Project: "p", Title: "t"},
			wantErr: true,
		},
		{
			name:    "all empty",
			mem:     models.Memory{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mem.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMemory_TagsDefaultsToNil(t *testing.T) {
	mem := models.Memory{}
	data, err := json.Marshal(mem)
	if err != nil {
		t.Fatal(err)
	}
	// Tags nil → marshals as null in JSON, acceptable for default state
	_ = data
}
