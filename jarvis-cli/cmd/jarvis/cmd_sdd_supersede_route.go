package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/Thrasno/jarvis-ai-devs/hivederive/applyprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/hiveclient"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddbinding"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddprogress"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddstatus"
	"github.com/spf13/cobra"
)

// This command remains unregistered pending route audit.
func newSddSupersedeCommand() *cobra.Command {
	var predecessor, successor, root, project, actor, reason string
	command := &cobra.Command{
		Use: "supersede", Short: "Supersede a bound SDD change", Args: cobra.NoArgs,
		SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if predecessor == "" {
				return fmt.Errorf("predecessor --change is required")
			}
			if successor == "" {
				return fmt.Errorf("--successor is required")
			}
			workspace, openRoot, err := progressBindingWorkspace(root, predecessor)
			if err != nil {
				return err
			}
			project, err = resolveSddProject(project, workspace)
			if err != nil {
				return err
			}
			if err = validateProgressBindingCoordinates(project, predecessor); err != nil {
				return err
			}
			if err = validateProgressBindingCoordinates(project, successor); err != nil {
				return err
			}
			client, err := hiveclient.NewFromEnv()
			if err != nil {
				return err
			}
			binding, err := (sddbinding.LegacyResolver{OpenSpecChangeDir: openRoot, HiveBindings: client, OpenSpecBindings: optionalPreflightOpenSpecBinding{}}).ResolveExisting(cmd.Context(), project, predecessor)
			if err != nil {
				return fmt.Errorf("persisted predecessor binding: %w", err)
			}
			if !binding.Persisted || (binding.Mode != sddruntime.StoreModeOpenSpec && binding.Mode != sddruntime.StoreModeHive && binding.Mode != sddruntime.StoreModeHybrid) {
				return fmt.Errorf("supersession mode %s not yet available", binding.Mode)
			}
			open := sddprogress.OpenSpec{Root: openRoot}
			hive := hiveProgressAdvancer{client: client, ctx: cmd.Context()}
			hybrid := sddprogress.Hybrid{Root: openRoot, OpenSpec: open, Hive: hive}
			if binding.Mode == sddruntime.StoreModeHybrid {
				seal, err := hybridRetrySeal(cmd.Context(), workspace, project, predecessor, successor, client, open)
				if err != nil {
					return err
				}
				if seal != nil {
					var actorFlag, reasonFlag *string
					if cmd.Flags().Changed("actor") {
						actorFlag = &actor
					}
					if cmd.Flags().Changed("reason") {
						reasonFlag = &reason
					}
					request, err := retrySupersessionSealRequest(*seal, successor, actorFlag, reasonFlag)
					if err != nil {
						return err
					}
					local, localErr := open.InspectPublication()
					remote, remoteErr := readSupersessionHiveHead(cmd.Context(), client, project, predecessor)
					if localErr == nil && remoteErr == nil && local != nil && remote != nil && local.Status == applyprogress.StatusSuperseded && remote.Status == applyprogress.StatusSuperseded {
						done, checkErr := completedAdvancedRetry(cmd.Context(), workspace, project, predecessor, successor, client, binding.Mode, binding.Provenance, *seal)
						if checkErr != nil {
							return checkErr
						}
						if done {
							_, err = fmt.Fprintf(cmd.OutOrStdout(), "Superseded %s; successor %s already bound and advanced\n", predecessor, successor)
							return err
						}
					}
					if _, err = hybrid.Advance(request); err != nil {
						return fmt.Errorf("hybrid seal replay failed: %w", err)
					}
					return publishAndBindHybridSuccessor(cmd.Context(), workspace, project, predecessor, successor, client, hybrid, *seal, cmd.OutOrStdout())
				}
			}
			// Inspect the selected stored head before NEW preflight: retry never prompts.
			var head *applyprogress.Snapshot
			var headErr error
			if binding.Mode == sddruntime.StoreModeHive {
				head, headErr = readSupersessionHiveHead(cmd.Context(), client, project, predecessor)
				if headErr == nil && (head == nil || applyprogress.VerifySnapshot(*head) != nil || head.Project != project || head.Change != predecessor) {
					return fmt.Errorf("Hive predecessor head missing or invalid")
				}
			} else if binding.Mode == sddruntime.StoreModeOpenSpec {
				head, headErr = open.InspectPublication()
			}
			if headErr == nil && head != nil && head.Status == applyprogress.StatusSuperseded {
				var actorFlag, reasonFlag *string
				if cmd.Flags().Changed("actor") {
					actorFlag = &actor
				}
				if cmd.Flags().Changed("reason") {
					reasonFlag = &reason
				}
				request, err := retrySupersessionSealRequest(*head, successor, actorFlag, reasonFlag)
				if err != nil {
					return err
				}
				var projection sddstatus.SupersessionProjection
				if binding.Mode == sddruntime.StoreModeHive {
					projection, err = sddstatus.NewHiveSource(client, project).ResolveSupersession(cmd.Context(), predecessor, *head)
				} else {
					projection, err = sddstatus.NewOpenSpecSourceForProject(workspace, project).ResolveSupersession(cmd.Context(), predecessor, *head)
				}
				if err != nil {
					return fmt.Errorf("stored successor: %w", err)
				}
				if projection.Change != successor || (projection.State != sddstatus.SupersessionPending && projection.State != sddstatus.SupersessionReady) {
					return fmt.Errorf("foreign successor state")
				}
				done, checkErr := completedAdvancedRetry(cmd.Context(), workspace, project, predecessor, successor, client, binding.Mode, binding.Provenance, *head)
				if checkErr != nil {
					return checkErr
				}
				if done {
					_, err = fmt.Fprintf(cmd.OutOrStdout(), "Superseded %s; successor %s already bound and advanced\n", predecessor, successor)
					return err
				}
				if binding.Mode == sddruntime.StoreModeHive {
					if _, err = hive.Advance(request); err != nil {
						return fmt.Errorf("Hive seal replay failed: %w", err)
					}
					return publishAndBindHiveSuccessor(cmd.Context(), workspace, project, predecessor, successor, client, hive, request.Snapshot, projection.State == sddstatus.SupersessionReady, cmd.OutOrStdout())
				}
				if _, err = open.Advance(request); err != nil {
					return fmt.Errorf("seal replay failed: %w", err)
				}
				return publishAndBindOpenSpecSuccessor(cmd.Context(), workspace, project, predecessor, successor, client, open, request.Snapshot, projection.State == sddstatus.SupersessionReady, cmd.OutOrStdout())
			}
			if headErr != nil && binding.Mode == sddruntime.StoreModeHive {
				return fmt.Errorf("inspect Hive predecessor: %w", headErr)
			}
			if binding.Mode == sddruntime.StoreModeOpenSpec && headErr != nil && headErr != sddprogress.ErrLegacyMigration {
				// A revised task manifest makes InspectPublication reject an otherwise
				// authentic PARTIAL head. The strict preflight validates that case.
				if _, err = open.InspectSealablePredecessor(project, predecessor); err != nil {
					return fmt.Errorf("inspect predecessor: %w", headErr)
				}
			}
			if !cmd.Flags().Changed("actor") || !cmd.Flags().Changed("reason") {
				return fmt.Errorf("--actor and --reason are required for a new seal")
			}
			if err = validateSupersessionAttribution(actor, reason); err != nil {
				return err
			}
			first, err := newSupersessionPreflight(cmd.Context(), workspace, project, predecessor, successor, client)
			if err != nil {
				return err
			}
			if first.Mode != binding.Mode {
				return fmt.Errorf("persisted supersession mode changed")
			}
			// Check deterministic seal capacity before requesting consent. The real ID
			// and timestamp are minted only after consent succeeds.
			if _, err = planNewSupersessionSeal(first, successor, actor, reason, time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC), "supersession-00000000000000000000000000000000"); err != nil {
				return err
			}
			if err = requireSupersessionConsent(first.Predecessor, successor, cmd.InOrStdin(), cmd.OutOrStdout()); err != nil {
				return err
			}
			second, err := newSupersessionPreflight(cmd.Context(), workspace, project, predecessor, successor, client)
			if err != nil {
				return fmt.Errorf("preflight changed after consent: %w", err)
			}
			if first.Mode != second.Mode || first.Predecessor.Digest != second.Predecessor.Digest || first.TaskManifest != second.TaskManifest || first.TasksContent != second.TasksContent {
				return fmt.Errorf("preflight changed after consent")
			}
			idBytes := make([]byte, 16)
			if _, err = rand.Read(idBytes); err != nil {
				return err
			}
			request, err := planNewSupersessionSeal(second, successor, actor, reason, time.Now().UTC(), "supersession-"+hex.EncodeToString(idBytes))
			if err != nil {
				return err
			}
			if binding.Mode == sddruntime.StoreModeHive {
				if _, err = hive.Advance(request); err != nil {
					return fmt.Errorf("Hive seal failed; retry using stored state: %w", err)
				}
				return publishAndBindHiveSuccessor(cmd.Context(), workspace, project, predecessor, successor, client, hive, request.Snapshot, false, cmd.OutOrStdout())
			}
			if binding.Mode == sddruntime.StoreModeHybrid {
				if _, err = hybrid.Advance(request); err != nil {
					return fmt.Errorf("hybrid seal failed; retry using stored state: %w", err)
				}
				return publishAndBindHybridSuccessor(cmd.Context(), workspace, project, predecessor, successor, client, hybrid, request.Snapshot, cmd.OutOrStdout())
			}
			if _, err = open.Advance(request); err != nil {
				return fmt.Errorf("seal failed; retry using stored state: %w", err)
			}
			return publishAndBindOpenSpecSuccessor(cmd.Context(), workspace, project, predecessor, successor, client, open, request.Snapshot, false, cmd.OutOrStdout())
		},
	}
	command.Flags().StringVar(&predecessor, "change", "", "predecessor change")
	command.Flags().StringVar(&successor, "successor", "", "successor change")
	command.Flags().StringVar(&root, "root", "", "canonical predecessor change directory")
	command.Flags().StringVar(&project, "project", "", "project")
	command.Flags().StringVar(&actor, "actor", "", "actor signing a new seal")
	command.Flags().StringVar(&reason, "reason", "", "reason for a new seal")
	return command
}

// readSupersessionHiveHead rejects noncanonical or inconsistent GET envelopes before writes.
func readSupersessionHiveHead(ctx context.Context, client *hiveclient.Client, project, change string) (*applyprogress.Snapshot, error) {
	result, err := client.GetApplyProgress(ctx, project, change)
	if err != nil {
		return nil, err
	}
	head := result.State.Snapshot
	if result.Outcome != "committed" || result.Code != "ok" || !result.State.CoordinatesPresent || head.Project != project || head.Change != change || result.State.Generation != head.Generation || result.State.Revision != head.Revision || result.State.Digest != head.Digest || applyprogress.VerifySnapshot(head) != nil {
		return nil, fmt.Errorf("Hive apply-progress GET is not a matching committed/ok signed head")
	}
	return &head, nil
}

// completedAdvancedRetry succeeds only for an already bound, independently
// authenticated successor beyond its immutable 1/1 genesis.
func completedAdvancedRetry(ctx context.Context, workspace, project, predecessor, successor string, client *hiveclient.Client, mode sddruntime.StoreMode, provenance string, seal applyprogress.Snapshot) (bool, error) {
	localSource := sddstatus.NewOpenSpecSourceForProject(workspace, project)
	hiveSource := sddstatus.NewHiveSource(client, project)
	projections := make([]sddstatus.SupersessionProjection, 0, 2)
	if mode != sddruntime.StoreModeHive {
		projection, err := localSource.ResolveSupersession(ctx, predecessor, seal)
		if err != nil {
			return false, fmt.Errorf("OpenSpec successor authority: %w", err)
		}
		projections = append(projections, projection)
	}
	if mode != sddruntime.StoreModeOpenSpec {
		projection, err := hiveSource.ResolveSupersession(ctx, predecessor, seal)
		if err != nil {
			return false, fmt.Errorf("Hive successor authority: %w", err)
		}
		projections = append(projections, projection)
	}
	advanced := false
	for _, projection := range projections {
		if projection.Change != successor || (projection.State != sddstatus.SupersessionPending && projection.State != sddstatus.SupersessionReady) {
			return false, fmt.Errorf("foreign successor projection")
		}
		if projection.State == sddstatus.SupersessionReady && projection.Head != nil && (projection.Head.Generation != 1 || projection.Head.Revision != 1 || projection.Head.Status != applyprogress.StatusPartial) {
			advanced = true
		}
	}
	if !advanced {
		return false, nil
	}
	for _, projection := range projections {
		if projection.State != sddstatus.SupersessionReady || projection.Head == nil {
			return false, fmt.Errorf("advanced successor not published on every selected store")
		}
	}
	if len(projections) == 2 {
		left, right := projections[0].Head, projections[1].Head
		if left.Digest != right.Digest || left.Generation != right.Generation || left.Revision != right.Revision {
			return false, fmt.Errorf("advanced hybrid successor heads diverged")
		}
	}
	digest := sha256.Sum256([]byte(provenance))
	expected := "supersession:" + seal.Digest + ":" + hex.EncodeToString(digest[:])
	target := filepath.Join(workspace, "openspec", "changes", successor)
	binding, err := (sddbinding.LegacyResolver{OpenSpecChangeDir: target, HiveBindings: client, OpenSpecBindings: optionalPreflightOpenSpecBinding{}}).ResolveExisting(ctx, project, successor)
	if err != nil || !binding.Persisted || binding.Mode != mode || binding.Provenance != expected {
		return false, fmt.Errorf("advanced successor binding missing or foreign: %v", err)
	}
	for index, projection := range projections {
		var stored *applyprogress.Snapshot
		if mode != sddruntime.StoreModeHive && index == 0 {
			stored, err = (sddprogress.OpenSpec{Root: target}).InspectPublication()
		} else {
			stored, err = readSupersessionHiveHead(ctx, client, project, successor)
		}
		if err != nil || stored == nil || stored.Digest != projection.Head.Digest || stored.Generation != projection.Head.Generation || stored.Revision != projection.Head.Revision {
			return false, fmt.Errorf("advanced successor stored head diverged: %v", err)
		}
	}
	return true, nil
}

// hybridRetrySeal authenticates each concrete selected source before a replay can mutate either.
func hybridRetrySeal(ctx context.Context, workspace, project, predecessor, successor string, client *hiveclient.Client, open sddprogress.OpenSpec) (*applyprogress.Snapshot, error) {
	local, localErr := open.InspectPublication()
	if localErr != nil {
		local, localErr = open.InspectSealablePredecessor(project, predecessor)
	}
	if localErr != nil || local == nil || applyprogress.VerifySnapshot(*local) != nil || local.Project != project || local.Change != predecessor {
		return nil, fmt.Errorf("hybrid OpenSpec predecessor invalid: %v", localErr)
	}
	remote, err := readSupersessionHiveHead(ctx, client, project, predecessor)
	if err != nil || remote == nil || applyprogress.VerifySnapshot(*remote) != nil || remote.Project != project || remote.Change != predecessor {
		return nil, fmt.Errorf("hybrid Hive predecessor invalid: %v", err)
	}
	localSealed := local.Status == applyprogress.StatusSuperseded
	remoteSealed := remote.Status == applyprogress.StatusSuperseded
	if !localSealed && local.Status != applyprogress.StatusPartial || !remoteSealed && remote.Status != applyprogress.StatusPartial {
		return nil, fmt.Errorf("hybrid predecessor is neither PARTIAL nor sealed")
	}
	if !localSealed && !remoteSealed {
		if local.Digest != remote.Digest {
			return nil, fmt.Errorf("hybrid partial predecessor heads diverged")
		}
		// The full NEW preflight authenticates both evidence sets and target vacancy.
		return nil, nil
	}
	seal := *local
	if !localSealed {
		seal = *remote
	}
	if localSealed && remoteSealed && (local.Digest != remote.Digest || local.Generation != remote.Generation || local.Revision != remote.Revision) {
		return nil, fmt.Errorf("hybrid signed predecessor heads diverged")
	}
	if !localSealed {
		partial, err := open.InspectSealablePredecessor(project, predecessor)
		if err != nil || !applyprogress.IsSupersessionSeal(*partial, seal) {
			return nil, fmt.Errorf("hybrid OpenSpec partial mirror differs from signed seal: %v", err)
		}
		if err = open.InspectSuccessorVacancy(sddprogress.OpenSpec{Root: filepath.Join(workspace, "openspec", "changes", successor)}, successor); err != nil {
			return nil, fmt.Errorf("hybrid OpenSpec successor occupied: %w", err)
		}
	}
	if !remoteSealed {
		partial, err := sddstatus.NewHiveSource(client, project).InspectSealablePredecessor(ctx, predecessor)
		if err != nil || !applyprogress.IsSupersessionSeal(partial, seal) {
			return nil, fmt.Errorf("hybrid Hive partial mirror differs from signed seal: %v", err)
		}
		occupancy, err := client.GetApplyProgressSuccessorOccupancy(ctx, project, successor)
		if err != nil {
			return nil, err
		}
		if occupancy.Occupied {
			return nil, fmt.Errorf("hybrid Hive successor occupied: %s", occupancy.Category)
		}
	}
	if localSealed {
		projection, err := sddstatus.NewOpenSpecSourceForProject(workspace, project).ResolveSupersession(ctx, predecessor, seal)
		if err != nil || projection.Change != successor || (projection.State != sddstatus.SupersessionPending && projection.State != sddstatus.SupersessionReady) {
			return nil, fmt.Errorf("hybrid OpenSpec successor foreign: %v", err)
		}
	}
	if remoteSealed {
		projection, err := sddstatus.NewHiveSource(client, project).ResolveSupersession(ctx, predecessor, seal)
		if err != nil || projection.Change != successor || (projection.State != sddstatus.SupersessionPending && projection.State != sddstatus.SupersessionReady) {
			return nil, fmt.Errorf("hybrid Hive successor foreign: %v", err)
		}
	}
	return &seal, nil
}

func publishAndBindHybridSuccessor(ctx context.Context, workspace, project, predecessor, successor string, client *hiveclient.Client, hybrid sddprogress.Hybrid, seal applyprogress.Snapshot, output io.Writer) error {
	localSource := sddstatus.NewOpenSpecSourceForProject(workspace, project)
	hiveSource := sddstatus.NewHiveSource(client, project)
	left, leftErr := localSource.ResolveSupersession(ctx, predecessor, seal)
	right, rightErr := hiveSource.ResolveSupersession(ctx, predecessor, seal)
	if leftErr != nil || rightErr != nil || left.Change != successor || right.Change != successor {
		return fmt.Errorf("hybrid successor state invalid: OpenSpec %v, Hive %v", leftErr, rightErr)
	}
	if left.State != sddstatus.SupersessionReady || right.State != sddstatus.SupersessionReady {
		if _, err := hybrid.PublishSuccessorGenesis(); err != nil {
			return fmt.Errorf("hybrid seal published; successor publication requires retry: %w", err)
		}
	}
	target := sddprogress.OpenSpec{Root: filepath.Join(workspace, "openspec", "changes", successor)}
	local, err := target.InspectPublication()
	if err != nil || local == nil {
		return fmt.Errorf("hybrid OpenSpec genesis missing: %v", err)
	}
	remote, err := readSupersessionHiveHead(ctx, client, project, successor)
	if err != nil || remote == nil || local.Digest != remote.Digest || applyprogress.ValidateSuccessorGenesisPair(seal, *local) != nil || applyprogress.ValidateSuccessorGenesisPair(seal, *remote) != nil {
		return fmt.Errorf("hybrid published genesis diverged: %v", err)
	}
	resolver := sddbinding.LegacyResolver{OpenSpecChangeDir: target.Root, HiveBindings: client, HiveSource: hiveSource, OpenSpecSource: localSource}
	if _, err = resolver.AdoptSuccessorGenesis(ctx, project, predecessor, seal, *local); err != nil {
		return fmt.Errorf("hybrid successor published; binding adoption requires retry: %w", err)
	}
	_, err = fmt.Fprintf(output, "Superseded %s; successor %s published and bound\n", predecessor, successor)
	return err
}

func publishAndBindHiveSuccessor(ctx context.Context, workspace, project, predecessor, successor string, client *hiveclient.Client, hive hiveProgressAdvancer, seal applyprogress.Snapshot, ready bool, output io.Writer) error {
	var genesis applyprogress.Snapshot
	if ready {
		stored, err := readSupersessionHiveHead(ctx, client, project, successor)
		if err != nil || stored == nil {
			return fmt.Errorf("Hive successor genesis missing: %v", err)
		}
		genesis = *stored
	} else {
		published, err := hive.PublishSuccessorGenesis(seal)
		if err != nil {
			return fmt.Errorf("Hive seal published; successor publication requires retry: %w", err)
		}
		genesis = published
	}
	if err := applyprogress.ValidateSuccessorGenesisPair(seal, genesis); err != nil {
		return fmt.Errorf("Hive successor genesis invalid: %w", err)
	}
	resolver := sddbinding.LegacyResolver{OpenSpecChangeDir: filepath.Join(workspace, "openspec", "changes", successor), HiveBindings: client, OpenSpecBindings: optionalPreflightOpenSpecBinding{}, HiveSource: sddstatus.NewHiveSource(client, project), OpenSpecSource: sddstatus.NewOpenSpecSourceForProject(workspace, project)}
	if _, err := resolver.AdoptSuccessorGenesis(ctx, project, predecessor, seal, genesis); err != nil {
		return fmt.Errorf("Hive successor published; binding adoption requires retry: %w", err)
	}
	_, err := fmt.Fprintf(output, "Superseded %s; successor %s published and bound\n", predecessor, successor)
	return err
}

func publishAndBindOpenSpecSuccessor(ctx context.Context, workspace, project, predecessor, successor string, client *hiveclient.Client, open sddprogress.OpenSpec, seal applyprogress.Snapshot, ready bool, output io.Writer) error {
	target := sddprogress.OpenSpec{Root: filepath.Join(workspace, "openspec", "changes", successor)}
	if !ready {
		if _, err := open.PublishSuccessorGenesis(target); err != nil {
			return fmt.Errorf("seal published; successor publication requires retry: %w", err)
		}
	}
	genesis, err := target.InspectPublication()
	if err != nil || genesis == nil {
		return fmt.Errorf("successor publication could not be authenticated: %v", err)
	}
	if err = applyprogress.ValidateSuccessorGenesisPair(seal, *genesis); err != nil {
		return err
	}
	resolver := sddbinding.LegacyResolver{OpenSpecChangeDir: target.Root, HiveBindings: client, OpenSpecSource: sddstatus.NewOpenSpecSourceForProject(workspace, project), HiveSource: sddstatus.NewHiveSource(client, project)}
	if _, err = resolver.AdoptSuccessorGenesis(ctx, project, predecessor, seal, *genesis); err != nil {
		return fmt.Errorf("successor published; binding adoption requires retry: %w", err)
	}
	_, err = fmt.Fprintf(output, "Superseded %s; successor %s published and bound\n", predecessor, successor)
	return err
}
