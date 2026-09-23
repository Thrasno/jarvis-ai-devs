package sddbinding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
)

type successorFileStore struct{}

func (successorFileStore) ReadOpenSpec(path string) (*Binding, error) { return ReadOpenSpec(path) }
func (successorFileStore) AdoptOpenSpec(path string, b Binding) (Binding, bool, error) {
	return AdoptOpenSpec(path, b)
}

// AdoptSuccessorGenesis binds a published successor to the predecessor's persisted
// store mode. Each selected source must authenticate its stored seal and
// published successor; a caller-supplied signed pair alone is not authority.
func (r LegacyResolver) AdoptSuccessorGenesis(ctx context.Context, project, predecessor string, seal, genesis applyprogress.Snapshot) (Resolution, error) {
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	project = hiveclient.CanonicalProjectKey(project)
	if project == "" || predecessor == "" || seal.Project != project || seal.Change != predecessor || genesis.Project != project || applyprogress.ValidateSuccessorGenesisPair(seal, genesis) != nil {
		return Resolution{}, fmt.Errorf("%w: invalid successor genesis coordinates or pair", ErrInvalidBinding)
	}
	target := filepath.Join(filepath.Dir(r.OpenSpecChangeDir), genesis.Change)
	if err := validateExistingChangeCoordinate(r.OpenSpecChangeDir, genesis.Change); err != nil { // The receiver must already point at the exact target.
		return Resolution{}, err
	}
	if r.OpenSpecChangeDir != target || filepath.Base(predecessor) != predecessor || predecessor == "." || predecessor == ".." {
		return Resolution{}, fmt.Errorf("%w: successor target or predecessor coordinate", ErrInvalidBinding)
	}
	prior := r
	prior.OpenSpecChangeDir = filepath.Join(filepath.Dir(target), predecessor)
	existing, err := prior.ResolveExisting(ctx, project, predecessor)
	if err != nil {
		return Resolution{}, err
	}
	// Authenticate publication independently on every selected backend before any
	// immutable write, including an exact replay or hybrid mirror repair.
	check := func(source sddstatus.ArtifactSource, hive bool) error {
		var projection sddstatus.SupersessionProjection
		var err error
		if hive {
			concrete, ok := source.(*sddstatus.HiveSource)
			if !ok || concrete == nil {
				return fmt.Errorf("%w: concrete Hive supersession source is required", ErrInvalidBinding)
			}
			projection, err = concrete.ResolveSupersession(ctx, predecessor, seal)
		} else {
			concrete, ok := source.(*sddstatus.OpenSpecSource)
			if !ok || concrete == nil {
				return fmt.Errorf("%w: concrete OpenSpec supersession source is required", ErrInvalidBinding)
			}
			projection, err = concrete.ResolveSupersession(ctx, predecessor, seal)
		}
		if err != nil {
			return err
		}
		if projection.State != sddstatus.SupersessionReady || projection.Change != genesis.Change || projection.Head == nil || projection.Head.Digest != genesis.Digest || applyprogress.ValidateSuccessorGenesisPair(seal, *projection.Head) != nil {
			return fmt.Errorf("%w: successor genesis is not authenticated and published", ErrInvalidBinding)
		}
		return nil
	}
	switch existing.Mode {
	case sddruntime.StoreModeHive:
		if err := check(r.HiveSource, true); err != nil {
			return Resolution{}, err
		}
	case sddruntime.StoreModeOpenSpec:
		if err := check(r.OpenSpecSource, false); err != nil {
			return Resolution{}, err
		}
	case sddruntime.StoreModeHybrid:
		if err := check(r.HiveSource, true); err != nil {
			return Resolution{}, err
		}
		if err := check(r.OpenSpecSource, false); err != nil {
			return Resolution{}, err
		}
	default:
		return Resolution{}, fmt.Errorf("%w: unsupported predecessor mode", ErrInvalidBinding)
	}
	if existing.Mode != sddruntime.StoreModeHive {
		found, err := existingDirectoryPath(target)
		if err != nil {
			return Resolution{}, err
		}
		if !found {
			return Resolution{}, fmt.Errorf("%w: successor directory missing", ErrUnsafePath)
		}
	}
	digest := sha256.Sum256([]byte(existing.Provenance))
	binding, err := New(existing.Mode, "supersession:"+seal.Digest+":"+hex.EncodeToString(digest[:]))
	if err != nil {
		return Resolution{}, err
	}
	// Immutable store operations detect foreign bindings and permit exact replay.
	local := r.OpenSpecBindings
	if local == nil {
		local = successorFileStore{}
	}
	hive, found, err := r.HiveBindings.GetSDDStoreBinding(ctx, project, genesis.Change)
	if err != nil {
		return Resolution{}, err
	}
	if found && (validateStoredHiveBinding(hive, project, genesis.Change) != nil || hive.Mode != hiveclient.SDDStoreMode(binding.Mode()) || hive.Provenance != binding.Provenance()) {
		return Resolution{}, fmt.Errorf("%w: foreign successor Hive binding", ErrBindingCopiesDiverged)
	}
	copy, err := local.ReadOpenSpec(target)
	if err != nil {
		return Resolution{}, err
	}
	if copy != nil && *copy != binding {
		return Resolution{}, fmt.Errorf("%w: foreign successor OpenSpec binding", ErrBindingCopiesDiverged)
	}
	return r.adopt(ctx, project, genesis.Change, binding, local)
}
