package applyprogress

import (
	"errors"
	"testing"
)

func TestConvertLegacy(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input LegacyProgress
		want  ValidationCode
	}{
		{"converts unambiguous completed tasks", LegacyProgress{Tasks: []Task{{Path: "1.1", Text: "  write\rtest "}}, Completed: []string{"1.1"}}, ""},
		{"rejects task without path", LegacyProgress{Tasks: []Task{{Text: "write test"}}}, CodeLegacyAmbiguous},
		{"rejects malformed explicit-ID path", LegacyProgress{Tasks: []Task{{ID: "explicit", Path: "not a path", Text: "write test"}}}, CodeLegacyAmbiguous},
		{"rejects duplicate task path", LegacyProgress{Tasks: []Task{{Path: "1.1", Text: "same"}, {Path: "1.1", Text: "same"}}}, CodeLegacyAmbiguous},
		{"rejects repeated completion", LegacyProgress{Tasks: []Task{{Path: "1.1", Text: "same"}}, Completed: []string{"1.1", "1.1"}}, CodeLegacyAmbiguous},
		{"rejects unknown completion", LegacyProgress{Tasks: []Task{{Path: "1.1", Text: "same"}}, Completed: []string{"9.9"}}, CodeLegacyAmbiguous},
		{"rejects path and ID completion collision", LegacyProgress{Tasks: []Task{{ID: "1.2", Path: "1.1", Text: "first"}, {ID: "1.1", Path: "2.1", Text: "second"}}, Completed: []string{"1.1"}}, CodeLegacyAmbiguous},
		{"rejects collision after another completion", LegacyProgress{Tasks: []Task{{ID: "1.2", Path: "1.1", Text: "first"}, {ID: "1.1", Path: "2.1", Text: "second"}}, Completed: []string{"2.1", "1.1"}}, CodeLegacyAmbiguous},
	} {
		t.Run(tt.name, func(t *testing.T) {
			converted, err := ConvertLegacy(tt.input)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				if got, want := converted.Tasks[0].Text, "write test"; got != want {
					t.Fatalf("text = %q, want %q", got, want)
				}
				if len(converted.Completed) != 1 || converted.Completed[0] != converted.Tasks[0].ID {
					t.Fatalf("completed = %#v, want converted task ID", converted.Completed)
				}
				return
			}
			var outcome *ValidationError
			if !errors.As(err, &outcome) || outcome.Code != tt.want {
				t.Fatalf("ConvertLegacy() error = %#v, want %q", err, tt.want)
			}
		})
	}
}
