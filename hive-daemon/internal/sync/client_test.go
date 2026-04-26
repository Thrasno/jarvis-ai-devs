package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_Login tests the login method with httptest server.
func TestClient_Login(t *testing.T) {
	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		wantErr        bool
		wantToken      string
		wantStatusCode int
	}{
		{
			name: "successful login returns token",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/auth/login", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Return success response
				w.WriteHeader(http.StatusOK)
				resp := map[string]interface{}{
					"token":      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
					"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				}
				require.NoError(t, json.NewEncoder(w).Encode(resp))
			},
			wantErr:   false,
			wantToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test",
		},
		{
			name: "login failure with 401",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, err := w.Write([]byte("invalid credentials"))
				require.NoError(t, err)
			},
			wantErr: true,
		},
		{
			name: "login failure with 500",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, err := w.Write([]byte("server error"))
				require.NoError(t, err)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			cfg := &Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}
			client := newClient(cfg)

			token, expiresAt, err := client.login(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantToken, token)
				assert.False(t, expiresAt.IsZero(), "expiresAt should not be zero")
			}
		})
	}
}

// TestClient_Sync tests the sync method with httptest server.
func TestClient_Sync(t *testing.T) {
	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		toSend        []*models.Memory
		lastSync      *time.Time
		wantErr       bool
		wantPushed    int
		wantPulled    int
	}{
		{
			name: "successful sync with observations",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/sync", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Contains(t, r.Header.Get("Authorization"), "Bearer test-token")

				// Return success response
				w.WriteHeader(http.StatusOK)
				resp := syncResponse{
					Pushed: 2,
					Pulled: []apiMemory{
						{
							SyncID:        "remote-sync-1",
							Project:       "test-project",
							Category:      "architecture",
							Title:         "Remote Memory 1",
							Content:       "Content from server",
							Tags:          []string{"remote"},
							FilesAffected: []string{"file.go"},
							CreatedBy:     "server-user",
							CreatedAt:     time.Now().UTC(),
							UpdatedAt:     time.Now().UTC(),
							Confidence:    0.85,
							ImpactScore:   8.0,
						},
					},
					Conflicts: 0,
				}
				require.NoError(t, json.NewEncoder(w).Encode(resp))
			},
			toSend: []*models.Memory{
				createTestSyncMemory("local-sync-1"),
				createTestSyncMemory("local-sync-2"),
			},
			lastSync:   nil,
			wantErr:    false,
			wantPushed: 2,
			wantPulled: 1,
		},
		{
			name: "successful sync with no new observations",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				resp := syncResponse{
					Pushed:    0,
					Pulled:    []apiMemory{},
					Conflicts: 0,
				}
				require.NoError(t, json.NewEncoder(w).Encode(resp))
			},
			toSend:     []*models.Memory{},
			lastSync:   nil,
			wantErr:    false,
			wantPushed: 0,
			wantPulled: 0,
		},
		{
			name: "sync with lastSync timestamp",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				var req syncRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				require.NoError(t, err)
				assert.NotNil(t, req.LastSync, "lastSync should be sent")

				w.WriteHeader(http.StatusOK)
				resp := syncResponse{Pushed: 0, Pulled: []apiMemory{}, Conflicts: 0}
				require.NoError(t, json.NewEncoder(w).Encode(resp))
			},
			toSend: []*models.Memory{},
			lastSync: func() *time.Time {
				t := time.Now().Add(-1 * time.Hour)
				return &t
			}(),
			wantErr:    false,
			wantPushed: 0,
			wantPulled: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			cfg := &Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}
			client := newClient(cfg)

			resp, err := client.sync(context.Background(), "test-token", "test-project", tt.toSend, tt.lastSync)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPushed, resp.Pushed)
				assert.Len(t, resp.Pulled, tt.wantPulled)

				// Verify observations deserialize correctly
				if tt.wantPulled > 0 {
					pulled := resp.Pulled[0]
					assert.NotEmpty(t, pulled.SyncID)
					assert.NotEmpty(t, pulled.Title)
					assert.NotEmpty(t, pulled.Content)
					assert.False(t, pulled.CreatedAt.IsZero())
					assert.False(t, pulled.UpdatedAt.IsZero())
				}
			}
		})
	}
}

// TestClient_Sync_AuthFailure tests that 401 errors are properly propagated.
func TestClient_Sync_AuthFailure(t *testing.T) {
	tests := []struct {
		name           string
		serverHandler  http.HandlerFunc
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "401 unauthorized returns auth error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, err := w.Write([]byte("token expired"))
				require.NoError(t, err)
			},
			wantErr:        true,
			wantErrContain: "401",
		},
		{
			name: "403 forbidden returns auth error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, err := w.Write([]byte("insufficient permissions"))
				require.NoError(t, err)
			},
			wantErr:        true,
			wantErrContain: "403",
		},
		{
			name: "500 server error returns error",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, err := w.Write([]byte("internal server error"))
				require.NoError(t, err)
			},
			wantErr:        true,
			wantErrContain: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverHandler)
			defer server.Close()

			cfg := &Config{
				APIURL:   server.URL,
				Email:    "test@example.com",
				Password: "password123",
			}
			client := newClient(cfg)

			_, err := client.sync(context.Background(), "invalid-token", "test-project", []*models.Memory{}, nil)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContain, "error should contain status code")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// createTestSyncMemory creates a test memory for sync operations.
func createTestSyncMemory(syncID string) *models.Memory {
	return &models.Memory{
		SyncID:        syncID,
		Project:       "test-project",
		Category:      "test",
		Title:         "Test Memory",
		Content:       "Test content for " + syncID,
		Tags:          []string{"test"},
		FilesAffected: []string{},
		CreatedBy:     "test-user",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
		Confidence:    "high",
		ImpactScore:   5,
	}
}
