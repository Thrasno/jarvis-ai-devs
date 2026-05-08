package sanitize_test

import (
	"testing"

	"github.com/Thrasno/jarvis-dev/hive-daemon/internal/sanitize"
)

func TestStrip_placeholder(t *testing.T) {
	r := sanitize.Strip("")
	if r.Clean != "" {
		t.Errorf("expected empty Clean, got %q", r.Clean)
	}
	if r.Count != 0 {
		t.Errorf("expected Count 0, got %d", r.Count)
	}
}
