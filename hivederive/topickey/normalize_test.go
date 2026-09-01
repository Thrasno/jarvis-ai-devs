package topickey

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   *string
		want *string
	}{
		{name: "nil remains nil"},
		{name: "surrounding whitespace is removed", in: stringPtr(" \t sdd/change/spec \n"), want: stringPtr("sdd/change/spec")},
		{name: "blank becomes nil", in: stringPtr(" \t\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Normalize(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func stringPtr(value string) *string { return &value }
