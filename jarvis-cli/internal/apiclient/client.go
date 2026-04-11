// Package apiclient provides an HTTP client for the Hive Cloud API.
package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const defaultTimeout = 15 * time.Second

// Client is a minimal HTTP client for the Hive Cloud API.
type Client struct {
	BaseURL    string
	Token      string
	httpClient *http.Client
}

// LoginRequest is the POST /auth/login request body.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is the POST /auth/login response body.
type LoginResponse struct {
	Token string `json:"token"`
	User  struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Level    string `json:"level"`
	} `json:"user"`
}

// UserResponse is the GET /auth/me response body.
type UserResponse struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Level    string `json:"level"`
}

// New creates a new Hive API client for the given base URL.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// Login authenticates with the Hive Cloud API and returns a token.
// Returns a descriptive error on 401 (wrong credentials) or network failures.
func (c *Client) Login(email, password string) (*LoginResponse, error) {
	body, err := json.Marshal(LoginRequest{Email: email, Password: password})
	if err != nil {
		return nil, fmt.Errorf("marshal login request: %w", err)
	}

	resp, err := c.httpClient.Post(c.BaseURL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("POST /auth/login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("invalid credentials — check your email and password")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from /auth/login: %d", resp.StatusCode)
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, fmt.Errorf("decode login response: %w", err)
	}

	// Store the token for subsequent requests
	c.Token = loginResp.Token

	return &loginResp, nil
}

// Me validates the current token by calling GET /auth/me.
// Returns the user info or an error if the token is invalid/expired.
func (c *Client) Me() (*UserResponse, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/auth/me", nil)
	if err != nil {
		return nil, fmt.Errorf("build /auth/me request: %w", err)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /auth/me: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("token invalid or expired — run 'jarvis login' to re-authenticate")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from /auth/me: %d", resp.StatusCode)
	}

	var user UserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode /auth/me response: %w", err)
	}

	return &user, nil
}

// doJSON performs an HTTP request and decodes the JSON response into target.
// Used internally for requests that need custom headers.
func (c *Client) doJSON(req *http.Request, target any) error {
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error %d", resp.StatusCode)
	}

	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}
