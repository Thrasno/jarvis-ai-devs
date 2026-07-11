package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accountServiceSpy struct {
	gotSubject string
	gotCurrent string
	gotNew     string
	err        error
}

func (s *accountServiceSpy) ChangePassword(_ context.Context, subject, currentPassword, newPassword string) error {
	s.gotSubject = subject
	s.gotCurrent = currentPassword
	s.gotNew = newPassword
	return s.err
}

func TestAccountHandlerChangePassword(t *testing.T) {
	const subject = "claims-user"
	newPassword := "new-password"

	for _, tt := range []struct {
		name       string
		body       string
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{name: "success uses claims subject", body: `{"current_password":"current-password","new_password":"new-password","user_id":"other-user","username":"other","email":"other@example.com"}`, wantStatus: http.StatusNoContent},
		{name: "wrong current password is generic", body: `{"current_password":"wrong","new_password":"new-password"}`, serviceErr: service.ErrInvalidCurrentPassword, wantStatus: http.StatusBadRequest, wantCode: "CURRENT_PASSWORD_INVALID"},
		{name: "password validation error", body: `{"current_password":"current-password","new_password":"short"}`, serviceErr: service.ErrInvalidPasswordLength, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_ERROR"},
		{name: "inactive user", body: `{"current_password":"current-password","new_password":"new-password"}`, serviceErr: service.ErrUserInactive, wantStatus: http.StatusForbidden, wantCode: "ACCOUNT_INACTIVE"},
		{name: "internal error is generic", body: `{"current_password":"current-password","new_password":"new-password"}`, serviceErr: errors.New("database password hash leaked"), wantStatus: http.StatusInternalServerError, wantCode: "SERVER_ERROR"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			accountSvc := &accountServiceSpy{err: tt.serviceErr}
			authSvc := &mockAuthSvc{}
			authSvc.On("ValidateToken", "valid-token").Return(&model.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: subject}}, nil)
			router := NewRouter(RouterDeps{AuthSvc: authSvc, AccountSvc: accountSvc})

			req := httptest.NewRequest(http.MethodPatch, "/account/password", bytes.NewBufferString(tt.body))
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusNoContent {
				assert.Empty(t, w.Body.String())
				assert.Equal(t, subject, accountSvc.gotSubject)
				assert.Equal(t, "current-password", accountSvc.gotCurrent)
				assert.Equal(t, newPassword, accountSvc.gotNew)
			} else {
				var response map[string]string
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.wantCode, response["code"])
				assert.NotContains(t, w.Body.String(), "database password hash leaked")
			}
			authSvc.AssertExpectations(t)
		})
	}
}

func TestAccountHandlerChangePasswordRejectsInvalidBearer(t *testing.T) {
	for _, tt := range []struct{ name, authorization string }{
		{name: "missing bearer"},
		{name: "invalid bearer", authorization: "Bearer invalid-token"},
		{name: "revoked bearer", authorization: "Bearer revoked-token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			authSvc := &mockAuthSvc{}
			if tt.authorization != "" {
				token := tt.authorization[len("Bearer "):]
				authSvc.On("ValidateToken", token).Return(nil, errors.New("invalid token"))
			}
			router := NewRouter(RouterDeps{AuthSvc: authSvc, AccountSvc: &accountServiceSpy{}})
			req := httptest.NewRequest(http.MethodPatch, "/account/password", bytes.NewBufferString(`{"current_password":"current-password","new_password":"new-password"}`))
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			authSvc.AssertExpectations(t)
		})
	}
}

func TestAccountHandlerChangePasswordRejectsMalformedAndMissingBodies(t *testing.T) {
	authSvc := &mockAuthSvc{}
	authSvc.On("ValidateToken", "valid-token").Return(&model.Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "claims-user"}}, nil)
	accountSvc := &accountServiceSpy{}
	router := NewRouter(RouterDeps{AuthSvc: authSvc, AccountSvc: accountSvc})

	for _, body := range []string{"", `{`, `{"current_password":"current-password"}`} {
		req := httptest.NewRequest(http.MethodPatch, "/account/password", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"code":"VALIDATION_ERROR","error":"invalid password change request"}`, w.Body.String())
	}
	assert.Empty(t, accountSvc.gotSubject)
	authSvc.AssertExpectations(t)
}
