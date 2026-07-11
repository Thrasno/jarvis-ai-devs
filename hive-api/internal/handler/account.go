package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/middleware"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/service"
	"github.com/gin-gonic/gin"
)

type AccountService interface {
	ChangePassword(ctx context.Context, claimsSubject, currentPassword, newPassword string) error
}

type AccountHandler struct{ svc AccountService }

func NewAccountHandler(svc AccountService) *AccountHandler { return &AccountHandler{svc: svc} }

func (h *AccountHandler) ChangePassword(c *gin.Context) {
	var req model.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		accountError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid password change request")
		return
	}
	raw, ok := c.Get(middleware.ClaimsKey)
	claims, ok := raw.(*model.Claims)
	if !ok || claims.Subject == "" {
		accountError(c, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), claims.Subject, req.CurrentPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidPasswordLength):
			accountError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid password change request")
		case errors.Is(err, service.ErrInvalidCurrentPassword):
			accountError(c, http.StatusBadRequest, "CURRENT_PASSWORD_INVALID", "current password could not be confirmed")
		case errors.Is(err, service.ErrUserInactive):
			accountError(c, http.StatusForbidden, "ACCOUNT_INACTIVE", "account inactive")
		default:
			accountError(c, http.StatusInternalServerError, "SERVER_ERROR", "internal server error")
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func accountError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "error": message})
}
