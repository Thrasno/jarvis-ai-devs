package sddruntime

import (
	"fmt"
	"os"
	"strings"
)

type StoreMode string

const (
	StoreModeHive     StoreMode = "hive"
	StoreModeOpenSpec StoreMode = "openspec"
	StoreModeHybrid   StoreMode = "hybrid"
	StoreModeNone     StoreMode = "none"
)

var ErrInvalidStoreMode = fmt.Errorf("invalid store mode")

const RuntimeStoreModeEnv = "JARVIS_SDD_STORE_MODE"

type StoreContract struct {
	Mode     StoreMode
	ReadFrom []string
	WriteTo  []string
}

func ResolveStoreMode(input string) (StoreMode, error) {
	mode := StoreMode(strings.ToLower(strings.TrimSpace(input)))
	switch mode {
	case StoreModeHive, StoreModeOpenSpec, StoreModeHybrid, StoreModeNone:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidStoreMode, input)
	}
}

func ResolveStoreContract(input string) (StoreContract, error) {
	mode, err := ResolveStoreMode(input)
	if err != nil {
		return StoreContract{}, err
	}

	switch mode {
	case StoreModeHive:
		return StoreContract{Mode: mode, ReadFrom: []string{"hive"}, WriteTo: []string{"hive"}}, nil
	case StoreModeOpenSpec:
		return StoreContract{Mode: mode, ReadFrom: []string{"openspec"}, WriteTo: []string{"openspec"}}, nil
	case StoreModeHybrid:
		return StoreContract{Mode: mode, ReadFrom: []string{"hive", "openspec"}, WriteTo: []string{"hive", "openspec"}}, nil
	case StoreModeNone:
		return StoreContract{Mode: mode, ReadFrom: nil, WriteTo: nil}, nil
	default:
		return StoreContract{}, fmt.Errorf("%w: %q", ErrInvalidStoreMode, input)
	}
}

func ResolveRuntimeStoreContract(defaultMode StoreMode) (StoreContract, error) {
	selected := strings.TrimSpace(os.Getenv(RuntimeStoreModeEnv))
	if selected == "" {
		selected = string(defaultMode)
	}
	return ResolveStoreContract(selected)
}
