package model

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

// validateStruct validates a struct using the Gin validator.
// This allows us to test binding tags without spinning up a full HTTP server.
func validateStruct(obj interface{}) error {
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		panic("validator engine is not *validator.Validate")
	}
	return v.Struct(obj)
}

// TestUserLevel_IsValid tests validation of user level values.
func TestUserLevel_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		level UserLevel
		want  bool
	}{
		{
			name:  "valid viewer level",
			level: LevelViewer,
			want:  true,
		},
		{
			name:  "valid member level",
			level: LevelMember,
			want:  true,
		},
		{
			name:  "valid admin level",
			level: LevelAdmin,
			want:  true,
		},
		{
			name:  "invalid empty level",
			level: "",
			want:  false,
		},
		{
			name:  "invalid custom level",
			level: "superuser",
			want:  false,
		},
		{
			name:  "invalid case mismatch",
			level: "ADMIN",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.level.IsValid()
			assert.Equal(t, tt.want, got, "IsValid() result mismatch")
		})
	}
}

// TestMemoryCategory_IsValid tests validation of memory category values.
func TestMemoryCategory_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category MemoryCategory
		want     bool
	}{
		{
			name:     "valid bugfix category",
			category: CatBugfix,
			want:     true,
		},
		{
			name:     "valid decision category",
			category: CatDecision,
			want:     true,
		},
		{
			name:     "valid architecture category",
			category: CatArchitecture,
			want:     true,
		},
		{
			name:     "valid discovery category",
			category: CatDiscovery,
			want:     true,
		},
		{
			name:     "valid pattern category",
			category: CatPattern,
			want:     true,
		},
		{
			name:     "valid config category",
			category: CatConfig,
			want:     true,
		},
		{
			name:     "valid preference category",
			category: CatPreference,
			want:     true,
		},
		{
			name:     "valid session_summary category",
			category: CatSessionSummary,
			want:     true,
		},
		{
			name:     "invalid empty category",
			category: "",
			want:     false,
		},
		{
			name:     "invalid custom category",
			category: "custom_type",
			want:     false,
		},
		{
			name:     "invalid case mismatch",
			category: "BUGFIX",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.category.IsValid()
			assert.Equal(t, tt.want, got, "IsValid() result mismatch")
		})
	}
}

// TestCreateMemoryRequest_Validation tests field validation for CreateMemoryRequest.
// This validates the Gin binding tags work correctly.
func TestCreateMemoryRequest_Validation(t *testing.T) {
	t.Parallel()

	validSyncID := "550e8400-e29b-41d4-a716-446655440001"

	tests := []struct {
		name      string
		request   CreateMemoryRequest
		wantValid bool
		wantError string
	}{
		{
			name: "valid request with all required fields",
			request: CreateMemoryRequest{
				SyncID:   validSyncID,
				Project:  "test-project",
				Category: CatDecision,
				Title:    "Test Memory",
				Content:  "Test content",
			},
			wantValid: true,
		},
		{
			name: "valid request with optional fields",
			request: CreateMemoryRequest{
				SyncID:        validSyncID,
				Project:       "test-project",
				Category:      CatBugfix,
				Title:         "Test Memory",
				Content:       "Test content",
				Tags:          []string{"tag1", "tag2"},
				FilesAffected: []string{"file1.go"},
			},
			wantValid: true,
		},
		{
			name: "invalid missing sync_id",
			request: CreateMemoryRequest{
				Project:  "test-project",
				Category: CatDecision,
				Title:    "Test Memory",
				Content:  "Test content",
			},
			wantValid: false,
			wantError: "SyncID",
		},
		{
			name: "invalid sync_id format",
			request: CreateMemoryRequest{
				SyncID:   "not-a-uuid",
				Project:  "test-project",
				Category: CatDecision,
				Title:    "Test Memory",
				Content:  "Test content",
			},
			wantValid: false,
			wantError: "SyncID",
		},
		{
			name: "invalid missing project",
			request: CreateMemoryRequest{
				SyncID:   validSyncID,
				Category: CatDecision,
				Title:    "Test Memory",
				Content:  "Test content",
			},
			wantValid: false,
			wantError: "Project",
		},
		{
			name: "invalid missing category",
			request: CreateMemoryRequest{
				SyncID:  validSyncID,
				Project: "test-project",
				Title:   "Test Memory",
				Content: "Test content",
			},
			wantValid: false,
			wantError: "Category",
		},
		{
			name: "invalid missing title",
			request: CreateMemoryRequest{
				SyncID:   validSyncID,
				Project:  "test-project",
				Category: CatDecision,
				Content:  "Test content",
			},
			wantValid: false,
			wantError: "Title",
		},
		{
			name: "invalid missing content",
			request: CreateMemoryRequest{
				SyncID:   validSyncID,
				Project:  "test-project",
				Category: CatDecision,
				Title:    "Test Memory",
			},
			wantValid: false,
			wantError: "Content",
		},
		{
			name: "invalid title too long",
			request: CreateMemoryRequest{
				SyncID:   validSyncID,
				Project:  "test-project",
				Category: CatDecision,
				Title:    string(make([]byte, 501)), // 501 chars, max is 500
				Content:  "Test content",
			},
			wantValid: false,
			wantError: "Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateStruct(tt.request)

			if tt.wantValid {
				assert.NoError(t, err, "expected valid request")
			} else {
				assert.Error(t, err, "expected validation error")
				if tt.wantError != "" {
					assert.Contains(t, err.Error(), tt.wantError, "error should mention field")
				}
			}
		})
	}
}

// TestSyncPromptPayload_Validation tests field validation for SyncPromptPayload.
// Covers spec scenarios S6 (empty content rejected) and S7 (invalid UUID rejected).
func TestSyncPromptPayload_Validation(t *testing.T) {
	t.Parallel()

	validSyncID := "550e8400-e29b-41d4-a716-446655440099"

	tests := []struct {
		name      string
		payload   SyncPromptPayload
		wantValid bool
		wantError string
	}{
		{
			name: "valid payload with all required fields",
			payload: SyncPromptPayload{
				SyncID:  validSyncID,
				Project: "test-project",
				Content: "Some useful prompt content",
			},
			wantValid: true,
		},
		{
			// S6: empty content must be rejected with validation error
			name: "invalid empty content rejected",
			payload: SyncPromptPayload{
				SyncID:  validSyncID,
				Project: "test-project",
				Content: "",
			},
			wantValid: false,
			wantError: "Content",
		},
		{
			// S7: invalid UUID sync_id must be rejected
			name: "invalid sync_id format rejected",
			payload: SyncPromptPayload{
				SyncID:  "not-a-uuid",
				Project: "test-project",
				Content: "Some content",
			},
			wantValid: false,
			wantError: "SyncID",
		},
		{
			// S7: missing sync_id must be rejected
			name: "missing sync_id rejected",
			payload: SyncPromptPayload{
				SyncID:  "",
				Project: "test-project",
				Content: "Some content",
			},
			wantValid: false,
			wantError: "SyncID",
		},
		{
			// Missing project must be rejected
			name: "missing project rejected",
			payload: SyncPromptPayload{
				SyncID:  validSyncID,
				Project: "",
				Content: "Some content",
			},
			wantValid: false,
			wantError: "Project",
		},
		{
			// Project exceeding max=100 must be rejected
			name: "project name too long rejected",
			payload: SyncPromptPayload{
				SyncID:  validSyncID,
				Project: string(make([]byte, 101)), // 101 chars, max is 100
				Content: "Some content",
			},
			wantValid: false,
			wantError: "Project",
		},
		{
			// FIX-6: content exceeding max=50000 chars must be rejected.
			// 50000 matches MaxObservationLength in daemon mcp/tools.go.
			name: "content too long rejected",
			payload: SyncPromptPayload{
				SyncID:  validSyncID,
				Project: "test-project",
				Content: string(make([]byte, 50001)), // 50001 chars, max is 50000
			},
			wantValid: false,
			wantError: "Content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateStruct(tt.payload)

			if tt.wantValid {
				assert.NoError(t, err, "expected valid payload")
			} else {
				assert.Error(t, err, "expected validation error")
				if tt.wantError != "" {
					assert.Contains(t, err.Error(), tt.wantError, "error should mention field")
				}
			}
		})
	}
}

// TestSyncRequest_BackwardCompat tests that SyncRequest without prompts field is valid
// and that PromptsPushed in SyncResponse defaults to zero.
// Covers spec scenario S9: old daemon (no prompts field) → prompts_pushed=0.
func TestSyncRequest_BackwardCompat(t *testing.T) {
	t.Parallel()

	t.Run("SyncRequest without prompts field is valid", func(t *testing.T) {
		t.Parallel()

		req := SyncRequest{
			Project: "test-project",
			// Prompts field omitted — old daemon behavior
		}

		err := validateStruct(req)
		assert.NoError(t, err, "SyncRequest without prompts should be valid (backward compat)")
		assert.Nil(t, req.Prompts, "Prompts should be nil when not provided")
	})

	t.Run("SyncResponse PromptsPushed defaults to zero", func(t *testing.T) {
		t.Parallel()

		// SyncResponse with no PromptsPushed set → zero value
		resp := SyncResponse{
			Pushed:    3,
			Pulled:    nil,
			Conflicts: 0,
		}

		// PromptsPushed should be zero when not explicitly set (S9)
		assert.Equal(t, 0, resp.PromptsPushed, "PromptsPushed should default to 0")
	})

	t.Run("SyncRequest with prompts field is valid", func(t *testing.T) {
		t.Parallel()

		// S10: new daemon with prompts — both sync in one round-trip
		validSyncID := "550e8400-e29b-41d4-a716-446655440088"
		req := SyncRequest{
			Project: "test-project",
			Prompts: []SyncPromptPayload{
				{
					SyncID:  validSyncID,
					Project: "test-project",
					Content: "A useful prompt",
				},
			},
		}

		err := validateStruct(req)
		assert.NoError(t, err, "SyncRequest with prompts should be valid")
		assert.Len(t, req.Prompts, 1, "Prompts should contain 1 entry")
	})
}

// TestSyncMemoryPayload_Validation tests field validation for SyncMemoryPayload.
func TestSyncMemoryPayload_Validation(t *testing.T) {
	t.Parallel()

	validSyncID := "550e8400-e29b-41d4-a716-446655440002"

	tests := []struct {
		name      string
		payload   SyncMemoryPayload
		wantValid bool
		wantError string
	}{
		{
			name: "valid payload with all required fields",
			payload: SyncMemoryPayload{
				SyncID:    validSyncID,
				Project:   "test-project",
				Category:  CatArchitecture,
				Title:     "Test Architecture Decision",
				Content:   "Test content",
				CreatedBy: "user@example.com",
			},
			wantValid: true,
		},
		{
			name: "invalid missing created_by",
			payload: SyncMemoryPayload{
				SyncID:   validSyncID,
				Project:  "test-project",
				Category: CatArchitecture,
				Title:    "Test Memory",
				Content:  "Test content",
			},
			wantValid: false,
			wantError: "CreatedBy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateStruct(tt.payload)

			if tt.wantValid {
				assert.NoError(t, err, "expected valid payload")
			} else {
				assert.Error(t, err, "expected validation error")
				if tt.wantError != "" {
					assert.Contains(t, err.Error(), tt.wantError, "error should mention field")
				}
			}
		})
	}
}
