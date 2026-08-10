package project

import (
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/projectidentity"
)

func TestNormalizeNameMatchesSharedConformanceVectors(t *testing.T) {
	for _, vector := range projectidentity.ConformanceVectors() {
		t.Run(vector.Name, func(t *testing.T) {
			if got := normalizeName(vector.Input); got != vector.Want {
				t.Fatalf("normalizeName(%q) = %q, want %q", vector.Input, got, vector.Want)
			}
		})
	}
}
