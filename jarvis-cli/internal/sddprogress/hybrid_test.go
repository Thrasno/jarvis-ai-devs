package sddprogress

import (
	"errors"
	"testing"
)

func TestResolveHybridFailsClosedOnDifferentMissingOrInvalidSides(t *testing.T) {
	first := request(t, "first", "apb-00000000000000000000000000000001", 1, 1, "")
	other := request(t, "other", "apb-00000000000000000000000000000002", 1, 1, "")
	for name, hive := range map[string]func() (ResolvedProgress, error){
		"different valid snapshot": func() (ResolvedProgress, error) { return ResolvedProgress{Snapshot: other.Snapshot}, nil },
		"same digest different state": func() (ResolvedProgress, error) {
			divergent := first.Snapshot
			divergent.Generation++
			return ResolvedProgress{Snapshot: divergent}, nil
		},
		"missing side": func() (ResolvedProgress, error) { return ResolvedProgress{}, ErrMissingBackend },
		"invalid side": func() (ResolvedProgress, error) { return ResolvedProgress{}, errors.New("invalid snapshot") },
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveHybrid(func() (ResolvedProgress, error) { return ResolvedProgress{Snapshot: first.Snapshot}, nil }, hive)
			if !errors.Is(err, ErrBackendDiverged) {
				t.Fatalf("ResolveHybrid() error = %v, want backend divergence", err)
			}
		})
	}
}

func TestResolveHybridAcceptsOnlyEqualValidatedSnapshots(t *testing.T) {
	request := request(t, "same", "apb-00000000000000000000000000000001", 1, 1, "")
	got, err := ResolveHybrid(
		func() (ResolvedProgress, error) { return ResolvedProgress{Snapshot: request.Snapshot}, nil },
		func() (ResolvedProgress, error) { return ResolvedProgress{Snapshot: request.Snapshot}, nil },
	)
	if err != nil || got.Snapshot.Digest != request.Snapshot.Digest {
		t.Fatalf("ResolveHybrid() = %#v, %v; want matching digest %q", got, err, request.Snapshot.Digest)
	}
}
