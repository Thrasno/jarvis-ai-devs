package handler

import (
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hive-api/internal/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestProjectBlockAckSubjectFromClaimsUsesSignedAccountSubjectAndMetadata(t *testing.T) {
	claims := &model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: "user-uuid-123"},
		DaemonID:         "daemon-1",
		Client:           "hive-daemon",
	}

	got := projectBlockAckSubjectFromClaims(claims)

	require.Equal(t, model.ProjectBlockAckSubject{
		AuthSubject: "user-uuid-123",
		DaemonID:    "daemon-1",
		Client:      "hive-daemon",
	}, got)
}

func TestProjectBlockAckSubjectFromClaimsIgnoresMissingClaims(t *testing.T) {
	require.Equal(t, model.ProjectBlockAckSubject{}, projectBlockAckSubjectFromClaims(nil))
}
