package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// planNewSupersessionSeal constructs a signed terminal head without publishing it.
func planNewSupersessionSeal(preflight newSupersessionPreflightResult, successor, actor, reason string, now time.Time, operationID string) (sddprogress.AdvanceRequest, error) {
	var zero sddprogress.AdvanceRequest
	old := preflight.Predecessor
	if err := validateSupersessionAttribution(actor, reason); err != nil {
		return zero, err
	}
	if !applyprogress.ValidID(successor) || successor == old.Change || !applyprogress.ValidID(operationID) || !applyprogress.ValidDigest(preflight.TaskManifest) || preflight.TaskManifest == old.TaskManifestSHA256 {
		return zero, fmt.Errorf("invalid supersession target, operation ID, or revised task manifest")
	}
	if err := applyprogress.VerifySnapshot(old); err != nil {
		return zero, fmt.Errorf("invalid signed predecessor: %w", err)
	}
	if old.Status != applyprogress.StatusPartial || old.Revision == ^uint64(0) || (old.Schema != applyprogress.SnapshotSchema && old.Schema != applyprogress.SupersessionSnapshotSchema) {
		return zero, fmt.Errorf("predecessor must be a non-overflowing signed PARTIAL head")
	}
	seal := old
	seal.Schema = applyprogress.SupersessionSnapshotSchema
	seal.Status = applyprogress.StatusSuperseded
	seal.Revision++
	seal.PreviousDigest = old.Digest
	seal.StreamSHA256, seal.NextEntryIndex, seal.NextEntryID = "", 0, ""
	seal.SealIntent = &applyprogress.SealIntent{SuccessorProject: old.Project, SuccessorChange: successor, SuccessorManifestSHA256: preflight.TaskManifest, Actor: actor, Reason: reason, Timestamp: now.UTC().Format(time.RFC3339Nano), OperationID: operationID}
	sealed, _, err := applyprogress.SealSnapshot(seal)
	if err != nil {
		return zero, fmt.Errorf("supersession seal: %w", err)
	}
	if !applyprogress.IsSupersessionSeal(old, sealed) {
		return zero, fmt.Errorf("invalid supersession seal transition")
	}
	return sddprogress.AdvanceRequest{RequestID: operationID, ExpectedGeneration: old.Generation, ExpectedRevision: old.Revision, ExpectedDigest: old.Digest, Snapshot: sealed}, nil
}

// retrySupersessionSealRequest reuses only the signed intent and exact stored seal.
func retrySupersessionSealRequest(storedSeal applyprogress.Snapshot, successor string, actorFlag, reasonFlag *string) (sddprogress.AdvanceRequest, error) {
	var zero sddprogress.AdvanceRequest
	intent := storedSeal.SealIntent
	if err := applyprogress.VerifySnapshot(storedSeal); err != nil {
		return zero, fmt.Errorf("invalid signed seal: %w", err)
	}
	if storedSeal.Schema != applyprogress.SupersessionSnapshotSchema || storedSeal.Status != applyprogress.StatusSuperseded || storedSeal.Revision == 0 || intent == nil || !applyprogress.ValidID(successor) || successor != intent.SuccessorChange || intent.SuccessorProject != storedSeal.Project || successor == storedSeal.Change || !applyprogress.ValidDigest(intent.SuccessorManifestSHA256) || intent.SuccessorManifestSHA256 == storedSeal.TaskManifestSHA256 || !applyprogress.ValidID(intent.OperationID) {
		return zero, fmt.Errorf("invalid signed supersession intention or target")
	}
	if err := validateSupersessionAttribution(intent.Actor, intent.Reason); err != nil {
		return zero, fmt.Errorf("invalid signed attribution: %w", err)
	}
	if actorFlag != nil && *actorFlag != intent.Actor || reasonFlag != nil && *reasonFlag != intent.Reason {
		return zero, fmt.Errorf("supplied attribution differs from SIGNED actor %q, reason %q, intention %q", intent.Actor, intent.Reason, intent.SuccessorChange)
	}
	return sddprogress.AdvanceRequest{RequestID: intent.OperationID, ExpectedGeneration: storedSeal.Generation, ExpectedRevision: storedSeal.Revision - 1, ExpectedDigest: storedSeal.PreviousDigest, Snapshot: storedSeal}, nil
}

// validateSupersessionAttribution checks the exact values to be signed for a
// new seal. Actor attribution does not authenticate the actor.
func validateSupersessionAttribution(actor, reason string) error {
	if err := validateSupersessionAttributionField("actor", actor, 128); err != nil {
		return err
	}
	return validateSupersessionAttributionField("reason", reason, 1024)
}

func validateSupersessionAttributionField(name, value string, maxRunes int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("supersession %s must not be empty", name)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("supersession %s must be valid UTF-8", name)
	}
	if !norm.NFC.IsNormalString(value) {
		return fmt.Errorf("supersession %s must be NFC-normalized", name)
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("supersession %s exceeds %d runes", name, maxRunes)
	}
	for _, r := range value {
		if unicode.Is(unicode.C, r) {
			return fmt.Errorf("supersession %s must not contain Unicode control characters", name)
		}
	}
	return nil
}
