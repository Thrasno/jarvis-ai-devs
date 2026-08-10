package projectidentity_test

import (
	"testing"

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

func TestCanonicalPreservesHistoricalSeparatorEquivalence(t *testing.T) {
	for _, input := range []string{" Foo.Bar ", "foo/bar", "foo_bar", "foo-bar", "FOO BAR"} {
		t.Run(input, func(t *testing.T) {
			if got, want := projectidentity.Canonical(input).String(), "foo-bar"; got != want {
				t.Fatalf("Canonical(%q) = %q, want %q", input, got, want)
			}
		})
	}
}
