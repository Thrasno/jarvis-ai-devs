package projectkey

import "testing"

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trims and lowercases", in: "  Jarvis-Dev  ", want: "jarvis-dev"},
		{name: "preserves interior whitespace", in: "My Garbage Project", want: "my garbage project"},
		{name: "preserves underscore and slash separators", in: "Team_Project/Feature", want: "team_project/feature"},
		{name: "preserves repeated separators", in: "Team---Project___Feature", want: "team---project___feature"},
		{name: "preserves dots and slashes", in: "github.com/Org/Repo", want: "github.com/org/repo"},
		{name: "uses unicode case folding", in: "Straße", want: "strasse"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Canonicalize(tt.in); got != tt.want {
				t.Fatalf("Canonicalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
