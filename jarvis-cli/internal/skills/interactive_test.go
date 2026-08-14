package skills

import "testing"

func TestIsInteractive(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "zoho deluge is stack specific", id: "zoho-deluge", want: true},
		{name: "phpunit testing is stack specific", id: "phpunit-testing", want: true},
		{name: "laravel architecture is stack specific", id: "laravel-architecture", want: true},
		{name: "go testing is stack specific", id: "go-testing", want: true},
		{name: "sdd-spec is auto installed despite scope optional", id: "sdd-spec", want: false},
		{name: "an unknown id is not stack specific", id: "hand-written-skill", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInteractive(tt.id); got != tt.want {
				t.Fatalf("IsInteractive(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
