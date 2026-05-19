// Package apiclient provides an HTTP client for the Hive Cloud API.
package apiclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 15 * time.Second

const invalidLoginEmailMessage = "Email inválido. Ingresa un email válido, por ejemplo usuario@dominio.com."

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

type apiErrorResponse struct {
	Error string `json:"error"`
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
	normalizedEmail, err := NormalizeLoginEmail(email)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(LoginRequest{Email: normalizedEmail, Password: password})
	if err != nil {
		return nil, fmt.Errorf("marshal login request: %w", err)
	}

	resp, err := c.httpClient.Post(c.BaseURL+"/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("POST /auth/login: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		apiErr := decodeAPIError(resp)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("invalid credentials — check your email and password")
		case http.StatusForbidden:
			if strings.Contains(strings.ToLower(apiErr), "inactivo") || strings.Contains(strings.ToLower(apiErr), "inactive") {
				return nil, fmt.Errorf("your account is inactive — contact your workspace admin")
			}
			return nil, fmt.Errorf("access denied: %s", apiErr)
		case http.StatusInternalServerError:
			return nil, fmt.Errorf("server error during login — try again in a moment")
		default:
			if resp.StatusCode == http.StatusBadRequest && isBackendEmailValidationError(apiErr) {
				return nil, invalidLoginEmailError(normalizedEmail)
			}
			if apiErr != "" {
				return nil, fmt.Errorf("unexpected status from /auth/login: %d (%s)", resp.StatusCode, apiErr)
			}
			return nil, fmt.Errorf("unexpected status from /auth/login: %d", resp.StatusCode)
		}
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, fmt.Errorf("decode login response: %w", err)
	}

	// Store the token for subsequent requests
	c.Token = loginResp.Token

	return &loginResp, nil
}

// NormalizeLoginEmail trims and validates the login email before it reaches the API.
func NormalizeLoginEmail(input string) (string, error) {
	email := strings.TrimSpace(input)
	if email == "" {
		return "", invalidLoginEmailError("")
	}
	if strings.ContainsAny(email, " \t\r\n") {
		return "", invalidLoginEmailError("")
	}
	if strings.Count(email, "@") != 1 {
		return "", invalidLoginEmailError("")
	}
	local, domain, _ := strings.Cut(email, "@")
	if local == "" || domain == "" || !strings.Contains(domain, ".") {
		return "", invalidLoginEmailError("")
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", invalidLoginEmailError("")
	}
	return email, nil
}

func invalidLoginEmailError(email string) error {
	if isSafeEmailForError(email) {
		return fmt.Errorf("Email inválido %q. Ingresa un email válido, por ejemplo usuario@dominio.com.", email)
	}
	return errors.New(invalidLoginEmailMessage)
}

func isSafeEmailForError(email string) bool {
	if email == "" || len(email) > 120 {
		return false
	}
	for _, r := range email {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func isBackendEmailValidationError(apiErr string) bool {
	lower := strings.ToLower(apiErr)
	return strings.Contains(lower, "loginrequest.email") && strings.Contains(lower, "email") && strings.Contains(lower, "tag")
}

func decodeAPIError(resp *http.Response) string {
	var payload apiErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Error)
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
	defer func() {
		_ = resp.Body.Close()
	}()

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
