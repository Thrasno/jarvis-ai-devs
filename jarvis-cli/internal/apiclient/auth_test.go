package apiclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogin(t *testing.T) {
	tests := []struct {
		name       string
		email      string
		password   string
		serverFunc func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
		checkResp  func(t *testing.T, resp *LoginResponse)
	}{
		{
			name:     "successful login returns token",
			email:    "test@example.com",
			password: "secret",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/auth/login" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(LoginResponse{
					Token: "test-jwt-token",
					User: struct {
						Username string `json:"username"`
						Email    string `json:"email"`
						Level    string `json:"level"`
					}{Username: "testuser", Email: "test@example.com", Level: "free"},
				})
			},
			checkResp: func(t *testing.T, resp *LoginResponse) {
				if resp.Token != "test-jwt-token" {
					t.Errorf("expected token 'test-jwt-token', got %q", resp.Token)
				}
				if resp.User.Username != "testuser" {
					t.Errorf("expected username 'testuser', got %q", resp.User.Username)
				}
			},
		},
		{
			name:     "wrong credentials returns error",
			email:    "bad@example.com",
			password: "wrong",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: true,
		},
		{
			name:     "server error returns error",
			email:    "user@example.com",
			password: "pass",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := New(server.URL)
			resp, err := client.Login(tt.email, tt.password)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkResp != nil {
				tt.checkResp(t, resp)
			}
		})
	}
}

func TestMe(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		serverFunc func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
		checkResp  func(t *testing.T, resp *UserResponse)
	}{
		{
			name:  "valid token returns user info",
			token: "valid-jwt",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/auth/me" {
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				auth := r.Header.Get("Authorization")
				if auth != "Bearer valid-jwt" {
					t.Errorf("expected Authorization header 'Bearer valid-jwt', got %q", auth)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(UserResponse{
					Username: "andres",
					Email:    "andres@example.com",
					Level:    "pro",
				})
			},
			checkResp: func(t *testing.T, resp *UserResponse) {
				if resp.Username != "andres" {
					t.Errorf("expected username 'andres', got %q", resp.Username)
				}
			},
		},
		{
			name:  "expired token returns error",
			token: "expired-jwt",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: true,
		},
		{
			name:  "network error returns error",
			token: "any",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				// Close connection abruptly to simulate network error
				panic(http.ErrAbortHandler)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer func() { recover() }() // recover from panic in test
				tt.serverFunc(w, r)
			}))
			defer server.Close()

			client := New(server.URL)
			client.Token = tt.token

			resp, err := client.Me()

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkResp != nil {
				tt.checkResp(t, resp)
			}
		})
	}
}
