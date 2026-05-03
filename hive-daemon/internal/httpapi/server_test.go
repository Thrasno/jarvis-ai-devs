package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/httpapi"
	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/models"
)

type mockPromptStore struct {
	savePromptFn func(ctx context.Context, content string) (*models.Prompt, error)
	called       bool
}

func (m *mockPromptStore) SavePrompt(ctx context.Context, content string) (*models.Prompt, error) {
	m.called = true
	if m.savePromptFn != nil {
		return m.savePromptFn(ctx, content)
	}
	return &models.Prompt{ID: 42, Content: content, CreatedAt: time.Now()}, nil
}

func newTestServer(store *mockPromptStore) *httpapi.Server {
	return httpapi.NewServer("127.0.0.1:0", store)
}

// TS-HTTP-1: POST /prompts with valid content returns 201
func TestPostPrompts_ValidContent_Returns201(t *testing.T) {
	store := &mockPromptStore{}
	srv := httpapi.NewServer("127.0.0.1:0", store)

	body := `{"content": "fix the auth bug"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if _, ok := resp["id"]; !ok {
		t.Error("response missing 'id'")
	}
	if _, ok := resp["created_at"]; !ok {
		t.Error("response missing 'created_at'")
	}
}

// TS-HTTP-2: Empty content returns 400, SavePrompt NOT called
func TestPostPrompts_EmptyContent_Returns400(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": ""}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if store.called {
		t.Error("SavePrompt should NOT have been called for empty content")
	}
}

// TS-HTTP-3: Whitespace content returns 400
func TestPostPrompts_WhitespaceContent_Returns400(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": "   "}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if store.called {
		t.Error("SavePrompt should NOT have been called for whitespace content")
	}
}

// TS-HTTP-4: DB error returns 500, error message is "internal error" (NOT the DB error)
func TestPostPrompts_DBError_Returns500WithGenericMessage(t *testing.T) {
	store := &mockPromptStore{
		savePromptFn: func(ctx context.Context, content string) (*models.Prompt, error) {
			return nil, errors.New("some internal db error")
		},
	}
	srv := newTestServer(store)

	body := `{"content": "valid content"}`
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	errMsg, ok := resp["error"].(string)
	if !ok {
		t.Fatal("response missing 'error' field")
	}
	if errMsg != "internal error" {
		t.Errorf("expected 'internal error', got %q (DB error must not leak)", errMsg)
	}
}

// TS-HTTP-5: Malformed JSON returns 400
func TestPostPrompts_MalformedJSON_Returns400(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	body := `{"content": `
	req := httptest.NewRequest(http.MethodPost, "/prompts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// TS-HTTP-6: Non-POST method returns 405
func TestPostPrompts_NonPOSTMethod_Returns405(t *testing.T) {
	store := &mockPromptStore{}
	srv := newTestServer(store)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/prompts", nil)
			rr := httptest.NewRecorder()

			srv.ServeHTTP(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405, got %d for method %s", rr.Code, method)
			}
		})
	}
}

// TS-HTTP-7: Server starts and accepts real HTTP connections
func TestServer_StartsAndAcceptsConnections(t *testing.T) {
	store := &mockPromptStore{}

	// Find a free port
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	srv := httpapi.NewServer(addr, store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Post("http://"+addr+"/prompts", "application/json", bytes.NewBufferString(`{"content":"hello"}`))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}
