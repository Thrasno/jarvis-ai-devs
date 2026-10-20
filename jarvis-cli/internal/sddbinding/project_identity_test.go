package sddbinding

import (
	"context"
	"errors"
	"testing"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

func TestLegacyBindingReportsStoredProjectMismatch(t *testing.T) {
	foreign := hiveBinding("hive", "selected:hive")
	foreign.Project = "foreign"
	hive := &fakeHiveBindingStore{binding: foreign, found: true}
	resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: &fakeOpenSpecBindingStore{}, HiveSource: &fakeLegacySource{}, OpenSpecSource: &fakeLegacySource{}, OpenSpecChangeDir: "change-dir"}

	_, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"})
	if !errors.Is(err, sddstatus.ErrProjectMismatch) || !errors.Is(err, ErrUnsupportedStoredBinding) {
		t.Fatalf("err = %v, want project mismatch on an unsupported stored binding", err)
	}
	if len(hive.adopted) != 0 {
		t.Fatalf("project mismatch adopted a binding: %#v", hive.adopted)
	}
}

func TestLegacyBindingCanonicalKeyWithoutAliasStillResolves(t *testing.T) {
	hive := &fakeHiveBindingStore{binding: hiveBinding("hive", "selected:hive"), found: true}
	resolver := LegacyResolver{HiveBindings: hive, OpenSpecBindings: &fakeOpenSpecBindingStore{}, HiveSource: &fakeLegacySource{}, OpenSpecSource: &fakeLegacySource{}, OpenSpecChangeDir: "change-dir"}

	got, err := resolver.ResolveAndAdopt(context.Background(), "project", "change", InitialSelection{Mode: sddruntime.StoreModeHive, Provenance: "ignored"})
	if err != nil || got.Mode != sddruntime.StoreModeHive || !got.Persisted {
		t.Fatalf("resolution = %#v, err = %v", got, err)
	}
}
