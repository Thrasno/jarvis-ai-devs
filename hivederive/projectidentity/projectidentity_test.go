package projectidentity_test

import (
	"testing"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

func TestCanonicalConformanceVectors(t *testing.T) {
	if got, want := projectidentity.ContractVersion, "v1"; got != want {
		t.Fatalf("ContractVersion = %q, want %q", got, want)
	}

	for _, vector := range projectidentity.ConformanceVectors() {
		t.Run(vector.Name, func(t *testing.T) {
			key := projectidentity.Canonical(vector.Input)
			if got := key.String(); got != vector.Want {
				t.Fatalf("Canonical(%q) = %q, want %q", vector.Input, got, vector.Want)
			}
			if got := projectidentity.Canonical(key.String()); got != key {
				t.Fatalf("Canonical(Canonical(%q)) = %q, want %q", vector.Input, got, key)
			}
		})
	}
}

func TestDisplayNamePrecedence(t *testing.T) {
	oldest := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	newest := oldest.Add(time.Hour)
	registrations := []projectidentity.Registration{
		{Spelling: "Jarvis-Dev", RegisteredAt: oldest},
		{Spelling: "jarvis-dev", RegisteredAt: newest},
	}

	if got, want := projectidentity.DisplayName("JARVIS-DEV", registrations), "JARVIS-DEV"; got != want {
		t.Fatalf("DisplayName with remote spelling = %q, want %q", got, want)
	}
	if got, want := projectidentity.DisplayName("", registrations), "Jarvis-Dev"; got != want {
		t.Fatalf("DisplayName without remote spelling = %q, want %q", got, want)
	}
}

func TestDisplayNameUsesOldestRegistrationRegardlessOfInputOrder(t *testing.T) {
	oldest := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	registrations := []projectidentity.Registration{
		{Spelling: "jarvis-dev", RegisteredAt: oldest.Add(time.Hour)},
		{Spelling: "Jarvis-Dev", RegisteredAt: oldest},
	}

	if got, want := projectidentity.DisplayName("", registrations), "Jarvis-Dev"; got != want {
		t.Fatalf("DisplayName with unordered registrations = %q, want %q", got, want)
	}
}
