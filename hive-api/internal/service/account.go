package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCurrentPassword = errors.New("invalid current password")
	ErrInvalidPasswordLength  = errors.New("password must be between 8 and 72 bytes")
)

type AccountService interface {
	ChangePassword(ctx context.Context, claimsSubject, currentPassword, newPassword string) error
}

type accountService struct {
	userRepo  repository.UserRepository
	auditRepo repository.AuditRepository
	tx        repository.TxManager
}

func NewAccountService(userRepo repository.UserRepository, auditRepo repository.AuditRepository, tx repository.TxManager) AccountService {
	return &accountService{userRepo: userRepo, auditRepo: auditRepo, tx: tx}
}

func (s *accountService) ChangePassword(ctx context.Context, claimsSubject, currentPassword, newPassword string) error {
	if !validPasswordLength(currentPassword) || !validPasswordLength(newPassword) {
		return ErrInvalidPasswordLength
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context, repos repository.TxRepositories) error {
		user, err := repos.Users.GetByIDForUpdate(ctx, claimsSubject)
		if err != nil {
			return err
		}
		if !user.IsActive {
			return ErrUserInactive
		}
		if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)) != nil {
			return ErrInvalidCurrentPassword
		}
		cost, err := bcrypt.Cost([]byte(user.Password))
		if err != nil {
			return ErrInvalidCurrentPassword
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), cost)
		if err != nil {
			return fmt.Errorf("hash new password: %w", err)
		}
		if err := repos.Users.UpdatePasswordAndIncrementSecurityVersion(ctx, user.ID, string(hash)); err != nil {
			return err
		}
		actorID := user.ID
		return repos.Audit.Insert(ctx, &model.AuditEntry{
			ActorUserID: &actorID,
			Action:      model.AuditActionUserPasswordChange,
			Outcome:     model.AuditOutcomeSuccess,
			Metadata:    model.SanitizeAuditMetadata(model.AuditActionUserPasswordChange, model.AuditMetadata{"actor_username": user.Username}),
		})
	})
}

func validPasswordLength(password string) bool {
	length := len([]byte(password))
	return length >= 8 && length <= 72
}
