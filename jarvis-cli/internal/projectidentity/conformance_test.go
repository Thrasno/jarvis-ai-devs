package projectidentity_test

import (
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

func TestSharedProjectIdentityConformanceVectors(t *testing.T) {
	for _, vector := range projectidentity.ConformanceVectors() {
		t.Run(vector.Name, func(t *testing.T) {
			if got := projectidentity.Canonical(vector.Input).String(); got != vector.Want {
				t.Fatalf("Canonical(%q) = %q, want %q", vector.Input, got, vector.Want)
			}
		})
	}
}
