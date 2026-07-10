package sddruntime

import (
	"errors"
	"os"
	"testing"
)

func TestResolveStoreMode_AllowsSupportedModes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  StoreMode
	}{
		{name: "hive", input: "hive", want: StoreModeHive},
		{name: "openspec", input: "openspec", want: StoreModeOpenSpec},
		{name: "hybrid", input: "hybrid", want: StoreModeHybrid},
		{name: "none", input: "none", want: StoreModeNone},
		{name: "trim and normalize", input: "  HyBrId  ", want: StoreModeHybrid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveStoreMode(tt.input)
			if err != nil {
				t.Fatalf("ResolveStoreMode(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ResolveStoreMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveStoreMode_RejectsUnsupportedModes(t *testing.T) {
	tests := []string{"", "memory", "engram", "open-spec"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ResolveStoreMode(input)
			if err == nil {
				t.Fatalf("expected error for input %q", input)
			}
			if !errors.Is(err, ErrInvalidStoreMode) {
				t.Fatalf("expected ErrInvalidStoreMode for input %q, got %v", input, err)
			}
		})
	}
}

func TestResolveStoreContract_UsesDeterministicReadWriteMatrix(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantMode  StoreMode
		wantRead  []string
		wantWrite []string
	}{
		{name: "hive", mode: "hive", wantMode: StoreModeHive, wantRead: []string{"hive"}, wantWrite: []string{"hive"}},
		{name: "openspec", mode: "openspec", wantMode: StoreModeOpenSpec, wantRead: []string{"openspec"}, wantWrite: []string{"openspec"}},
		{name: "hybrid", mode: "hybrid", wantMode: StoreModeHybrid, wantRead: []string{"hive", "openspec"}, wantWrite: []string{"hive", "openspec"}},
		{name: "none", mode: "none", wantMode: StoreModeNone, wantRead: nil, wantWrite: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract, err := ResolveStoreContract(tt.mode)
			if err != nil {
				t.Fatalf("ResolveStoreContract(%q) error = %v", tt.mode, err)
			}
			if contract.Mode != tt.wantMode {
				t.Fatalf("mode mismatch: got %q want %q", contract.Mode, tt.wantMode)
			}
			assertStringSliceEqual(t, contract.ReadFrom, tt.wantRead)
			assertStringSliceEqual(t, contract.WriteTo, tt.wantWrite)
		})
	}
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice length mismatch: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slice mismatch at index %d: got=%v want=%v", i, got, want)
		}
	}
}

func TestResolveRuntimeStoreContract_UsesDefaultWhenEnvUnset(t *testing.T) {
	t.Setenv(RuntimeStoreModeEnv, "")

	contract, err := ResolveRuntimeStoreContract(StoreModeOpenSpec)
	if err != nil {
		t.Fatalf("ResolveRuntimeStoreContract(default openspec) error = %v", err)
	}
	if contract.Mode != StoreModeOpenSpec {
		t.Fatalf("mode mismatch: got %q want %q", contract.Mode, StoreModeOpenSpec)
	}
	assertStringSliceEqual(t, contract.ReadFrom, []string{"openspec"})
	assertStringSliceEqual(t, contract.WriteTo, []string{"openspec"})
}

func TestResolveRuntimeStoreContract_UsesEnvOverrideWhenSet(t *testing.T) {
	t.Setenv(RuntimeStoreModeEnv, "hybrid")

	contract, err := ResolveRuntimeStoreContract(StoreModeHive)
	if err != nil {
		t.Fatalf("ResolveRuntimeStoreContract(env hybrid) error = %v", err)
	}
	if contract.Mode != StoreModeHybrid {
		t.Fatalf("mode mismatch: got %q want %q", contract.Mode, StoreModeHybrid)
	}
	assertStringSliceEqual(t, contract.ReadFrom, []string{"hive", "openspec"})
	assertStringSliceEqual(t, contract.WriteTo, []string{"hive", "openspec"})
}

func TestResolveRuntimeStoreContract_RejectsInvalidEnvValue(t *testing.T) {
	t.Setenv(RuntimeStoreModeEnv, "memory")

	_, err := ResolveRuntimeStoreContract(StoreModeHive)
	if err == nil {
		t.Fatal("expected invalid store mode error")
	}
	if !errors.Is(err, ErrInvalidStoreMode) {
		t.Fatalf("expected ErrInvalidStoreMode, got %v", err)
	}

	if got := os.Getenv(RuntimeStoreModeEnv); got != "memory" {
		t.Fatalf("expected env to remain set for runtime resolution, got %q", got)
	}
}
