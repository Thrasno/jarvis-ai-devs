package httpapi

import (
	"context"
	"errors"

	hivesync "github.com/Thrasno/jarvis-ai-devs/hive-daemon/internal/sync"
)

// SyncServiceAdapter wraps a *sync.Service and implements the ConfigService
// interface, mapping sync-package types and errors to httpapi-package types.
type SyncServiceAdapter struct {
	svc *hivesync.Service
}

// NewSyncServiceAdapter wraps svc so it satisfies ConfigService.
func NewSyncServiceAdapter(svc *hivesync.Service) *SyncServiceAdapter {
	return &SyncServiceAdapter{svc: svc}
}

func (a *SyncServiceAdapter) Status(ctx context.Context) (ConfigServiceStatus, error) {
	s, err := a.svc.Status(ctx)
	if err != nil {
		return ConfigServiceStatus{}, mapSyncErr(err)
	}
	return syncStatusToHTTPStatus(s), nil
}

func (a *SyncServiceAdapter) Update(ctx context.Context, req ConfigServiceUpdate) (ConfigServiceStatus, error) {
	syncReq := hivesync.ConfigUpdate{
		APIURL:   req.APIURL,
		Email:    req.Email,
		Password: req.Password,
		AutoSync: req.AutoSync,
	}
	s, err := a.svc.Update(ctx, syncReq)
	if err != nil {
		return ConfigServiceStatus{}, mapSyncErr(err)
	}
	return syncStatusToHTTPStatus(s), nil
}

func (a *SyncServiceAdapter) Test(ctx context.Context, req ConfigServiceUpdate) (ConfigServiceTestResult, error) {
	syncReq := hivesync.ConfigUpdate{
		APIURL:   req.APIURL,
		Email:    req.Email,
		Password: req.Password,
		AutoSync: req.AutoSync,
	}
	r, err := a.svc.Test(ctx, syncReq)
	if err != nil {
		return ConfigServiceTestResult{}, mapSyncErr(err)
	}
	return ConfigServiceTestResult{OK: r.OK, Message: r.Message}, nil
}

// mapSyncErr converts sync-package sentinel errors to httpapi sentinels so
// writeConfigError can map them to the correct HTTP status codes.
func mapSyncErr(err error) error {
	switch {
	case errors.Is(err, hivesync.ErrConfigInvalidURL):
		return ErrConfigInvalidURL
	case errors.Is(err, hivesync.ErrConfigEmailRequired):
		return ErrConfigEmailRequired
	case errors.Is(err, hivesync.ErrNoStoredSecret):
		return ErrNoStoredSecret
	default:
		return err
	}
}

// syncStatusToHTTPStatus maps a sync.ConfigStatus to httpapi.ConfigServiceStatus.
func syncStatusToHTTPStatus(s hivesync.ConfigStatus) ConfigServiceStatus {
	return ConfigServiceStatus{
		Configured:     s.Configured,
		Source:         s.Source,
		APIURL:         s.APIURL,
		Email:          s.Email,
		PasswordSet:    s.PasswordSet,
		PasswordMasked: s.PasswordMasked,
		AutoSync:       s.AutoSync,
		EnvActive:      s.EnvActive,
		RestartHint:    s.RestartHint,
		Warnings:       s.Warnings,
	}
}
