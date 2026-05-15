package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
)

const ledgerVersion = "v1"

type LedgerStore struct {
	homeDir string
}

func NewLedgerStore(homeDir string) LedgerStore {
	return LedgerStore{homeDir: homeDir}
}

func (s LedgerStore) LoadOrBootstrap(provider string) (Ledger, bool, error) {
	path := s.path()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		ledger := defaultLedger()
		if err := s.save(ledger); err != nil {
			return Ledger{}, false, err
		}
		return ledger, true, nil
	}
	if err != nil {
		return Ledger{}, false, fmt.Errorf("read ledger: %w", err)
	}

	var ledger Ledger
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return Ledger{}, false, fmt.Errorf("decode ledger: %w", err)
	}
	if ledger.Version != ledgerVersion {
		return Ledger{}, false, fmt.Errorf("incompatible ledger version %q", ledger.Version)
	}

	if ledger.ProviderSchemaVersion == "" {
		ledger.ProviderSchemaVersion = providerSchemaFor(provider)
		if err := s.save(ledger); err != nil {
			return Ledger{}, false, err
		}
	}

	return ledger, false, nil
}

func (s LedgerStore) LoadReadOnly(provider string) (Ledger, bool, error) {
	path := s.path()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaultLedger(), true, nil
	}
	if err != nil {
		return Ledger{}, false, fmt.Errorf("read ledger: %w", err)
	}

	var ledger Ledger
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return Ledger{}, false, fmt.Errorf("decode ledger: %w", err)
	}
	if ledger.Version != ledgerVersion {
		return Ledger{}, false, fmt.Errorf("incompatible ledger version %q", ledger.Version)
	}
	if ledger.ProviderSchemaVersion == "" {
		ledger.ProviderSchemaVersion = providerSchemaFor(provider)
	}
	return ledger, false, nil
}

func (s LedgerStore) save(ledger Ledger) error {
	path := s.path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create ledger dir: %w", err)
	}
	raw, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ledger: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write ledger: %w", err)
	}
	return nil
}

func (s LedgerStore) path() string {
	return filepath.Join(s.homeDir, ".jarvis", "managed-state.json")
}

func (s LedgerStore) remove() error {
	err := os.Remove(s.path())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove ledger: %w", err)
	}
	return nil
}

func defaultLedger() Ledger {
	c := sddruntime.DefaultContract()
	return Ledger{
		Version:               ledgerVersion,
		JarvisVersion:         c.JarvisVersion,
		ContractVersion:       c.ContractVersion,
		ProviderSchemaVersion: c.ProviderSchemaVersion,
	}
}

func providerSchemaFor(provider string) string {
	_ = provider
	return "v1"
}
