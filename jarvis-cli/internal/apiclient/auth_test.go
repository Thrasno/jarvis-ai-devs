package apiclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogin(t *testing.T) {
	tests := []struct {
		name       string
		email      string
		password   string
		serverFunc func(w http.ResponseWriter, r *http.Request)
		wantErr    bool
		wantErrHas string
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
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "credenciales inválidas"})
			},
			wantErr:    true,
			wantErrHas: "invalid credentials",
		},
		{
			name:     "inactive user returns actionable error",
			email:    "inactive@example.com",
			password: "secret",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "usuario inactivo"})
			},
			wantErr:    true,
			wantErrHas: "inactive",
		},
		{
			name:     "server error returns error",
			email:    "user@example.com",
			password: "pass",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
			},
			wantErr:    true,
			wantErrHas: "server error",
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
				if tt.wantErrHas != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErrHas)) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErrHas, err.Error())
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

func TestLogin_InvalidEmailReturnsLocalErrorWithoutCallingServer(t *testing.T) {
	tests := []struct {
		name  string
		email string
	}{
		{name: "empty", email: "  "},
		{name: "missing at", email: "invalid-email"},
		{name: "missing domain dot", email: "user@example"},
		{name: "embedded whitespace", email: "user @example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := New(server.URL)
			_, err := client.Login(tt.email, "secret")
			if err == nil {
				t.Fatal("expected local validation error")
			}
			if called {
				t.Fatal("invalid email must not call server")
			}
			if !strings.Contains(err.Error(), "usuario@dominio.com") {
				t.Fatalf("expected friendly email example, got %q", err.Error())
			}
			if strings.Contains(err.Error(), "jarvis --no-tui") {
				t.Fatalf("invalid email error must not recommend no-TUI fallback: %v", err)
			}
		})
	}
}

func TestLogin_NormalizesEmailAndPreservesPassword(t *testing.T) {
	var request LoginRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/login" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"abc","user":{"email":"trimmed@example.com"}}`))
	}))
	defer server.Close()

	client := New(server.URL)
	_, err := client.Login("  trimmed@example.com\t", " secret \n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if request.Email != "trimmed@example.com" {
		t.Fatalf("email was not normalized in request: %q", request.Email)
	}
	if request.Password != " secret \n" {
		t.Fatalf("password bytes were not preserved: %q", request.Password)
	}
}

func TestLogin_BackendEmailTag400ReturnsFriendlyEmailError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Key: 'LoginRequest.Email' Error:Field validation for 'Email' failed on the 'email' tag"})
	}))
	defer server.Close()

	client := New(server.URL)
	_, err := client.Login("  user@example.com  ", "secret")
	if err == nil {
		t.Fatal("expected friendly validation error")
	}
	if !strings.Contains(err.Error(), `"user@example.com"`) || !strings.Contains(err.Error(), "usuario@dominio.com") {
		t.Fatalf("expected friendly sanitized email error, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error must not include password: %q", err.Error())
	}
	if strings.Contains(err.Error(), "jarvis --no-tui") {
		t.Fatalf("invalid email error must not recommend no-TUI fallback: %v", err)
	}
}

func TestLoginDebug_DisabledCreatesNoTempLogFile(t *testing.T) {
	tmp := t.TempDir()
	setLoginDebugTempDir(t, tmp)
	t.Setenv("JARVIS_LOGIN_DEBUG", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"abc","user":{"email":"user@example.com"}}`))
	}))
	defer server.Close()

	client := New(server.URL)
	_, err := client.Login(" user@example.com ", "secret-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, loginDebugLogFile)); !os.IsNotExist(err) {
		t.Fatalf("expected no debug log file, stat error: %v", err)
	}
}

func TestLoginDebug_EnabledLogsNormalizedEmailWithoutPassword(t *testing.T) {
	tmp := t.TempDir()
	setLoginDebugTempDir(t, tmp)
	t.Setenv("JARVIS_LOGIN_DEBUG", "true")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"abc","user":{"email":"trimmed@example.com"}}`))
	}))
	defer server.Close()

	client := New(server.URL)
	_, err := client.Login("  trimmed@example.com\t", "secret-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logText := readLoginDebugLog(t, tmp)
	if !strings.Contains(logText, `event=before_post`) || !strings.Contains(logText, `email="trimmed@example.com"`) {
		t.Fatalf("expected normalized email before POST in debug log, got:\n%s", logText)
	}
	if !strings.Contains(logText, `email_runes=[U+0074 U+0072`) {
		t.Fatalf("expected rune diagnostics in debug log, got:\n%s", logText)
	}
	if strings.Contains(logText, "secret-password") {
		t.Fatalf("debug log must not contain password, got:\n%s", logText)
	}
}

func TestLoginDebug_LogsBackendStatusAndLocalInvalidEmail(t *testing.T) {
	t.Run("backend status", func(t *testing.T) {
		tmp := t.TempDir()
		setLoginDebugTempDir(t, tmp)
		t.Setenv("JARVIS_LOGIN_DEBUG", "1")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"bad credentials"}`))
		}))
		defer server.Close()

		client := New(server.URL)
		_, err := client.Login("user@example.com", "secret-password")
		if err == nil {
			t.Fatal("expected login error")
		}

		logText := readLoginDebugLog(t, tmp)
		if !strings.Contains(logText, `event=response`) || !strings.Contains(logText, `status=401`) || !strings.Contains(logText, `class=http_401`) {
			t.Fatalf("expected backend status diagnostics, got:\n%s", logText)
		}
		if strings.Contains(logText, "secret-password") || strings.Contains(logText, "bad credentials") {
			t.Fatalf("debug log must not contain password or response body, got:\n%s", logText)
		}
	})

	t.Run("local invalid email", func(t *testing.T) {
		tmp := t.TempDir()
		setLoginDebugTempDir(t, tmp)
		t.Setenv("JARVIS_LOGIN_DEBUG", "1")

		client := New("https://api.example.test")
		_, err := client.Login("invalid-email", "secret-password")
		if err == nil {
			t.Fatal("expected local validation error")
		}

		logText := readLoginDebugLog(t, tmp)
		if !strings.Contains(logText, `event=local_validation_failed`) || !strings.Contains(logText, `email="invalid-email"`) || !strings.Contains(logText, `class=invalid_email`) {
			t.Fatalf("expected local invalid-email diagnostics, got:\n%s", logText)
		}
		if strings.Contains(logText, "secret-password") {
			t.Fatalf("debug log must not contain password, got:\n%s", logText)
		}
	})
}

func setLoginDebugTempDir(t *testing.T, tmp string) {
	t.Helper()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
}

func readLoginDebugLog(t *testing.T, tmp string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(tmp, loginDebugLogFile))
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	return string(b)
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
				defer func() { _ = recover() }() // recover from panic in test
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

func TestLogin_AdditionalStatusBranches(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErrHas string
	}{
		{name: "forbidden non-inactive returns access denied", status: http.StatusForbidden, body: `{"error":"forbidden by policy"}`, wantErrHas: "access denied: forbidden by policy"},
		{name: "unexpected status with api error includes payload", status: http.StatusTeapot, body: `{"error":"teapot"}`, wantErrHas: "unexpected status from /auth/login: 418 (teapot)"},
		{name: "unexpected status without parseable api error omits payload", status: http.StatusBadGateway, body: `gateway down`, wantErrHas: "unexpected status from /auth/login: 502"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := New(server.URL)
			_, err := client.Login("user@example.com", "secret")
			if err == nil {
				t.Fatal("expected login error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.wantErrHas)) {
				t.Fatalf("expected %q in error, got %q", tt.wantErrHas, err.Error())
			}
		})
	}
}

func TestMe_UnexpectedStatusAndDecodeError(t *testing.T) {
	t.Run("non-unauthorized non-200 returns generic status error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()

		client := New(server.URL)
		_, err := client.Me()
		if err == nil || !strings.Contains(err.Error(), "unexpected status from /auth/me: 502") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("ok status with invalid json returns decode error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{invalid json"))
		}))
		defer server.Close()

		client := New(server.URL)
		_, err := client.Me()
		if err == nil || !strings.Contains(err.Error(), "decode /auth/me response") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDecodeAPIError_InvalidJSONReturnsEmpty(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("not-json"))}
	if got := decodeAPIError(resp); got != "" {
		t.Fatalf("expected empty decode error, got %q", got)
	}
}
