package projectkey_test

import (
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

func TestSharedProjectIdentityConformanceVectorsAreAvailable(t *testing.T) {
	vectors := projectidentity.ConformanceVectors()
	if len(vectors) == 0 {
		t.Fatal("ConformanceVectors returned no shared vectors")
	}
	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			if got := projectidentity.Canonical(vector.Input).String(); got != vector.Want {
				t.Fatalf("Canonical(%q) = %q, want %q", vector.Input, got, vector.Want)
			}
		})
	}
}
