package projectkey

import "testing"

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trims and lowercases", in: "  Jarvis-Dev  ", want: "jarvis-dev"},
		{name: "collapses whitespace to dash", in: "My Garbage Project", want: "my-garbage-project"},
		{name: "normalizes underscores and path separators", in: "Team_Project/Feature", want: "team-project-feature"},
		{name: "removes duplicate separators", in: "Team---Project___Feature", want: "team-project-feature"},
		{name: "preserves dots in names", in: "github.com/Org/Repo", want: "github.com-org-repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Canonicalize(tt.in); got != tt.want {
				t.Fatalf("Canonicalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
