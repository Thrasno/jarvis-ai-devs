package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/agent"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/sddruntime"
	"github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"
)

// ──────────────────────────────────────────────────────────────────────────────
// Step 0: Configuration reset (issue #767 T7)
//
// The very first step the wizard can show, and only when at least one agent
// was detected: there is nothing a reset could touch otherwise. It is a
// consented, default-No, all-or-nothing reset of exactly the contract-owned
// surfaces sddruntime.ResetInventory declares for that agent's platform.
// Declining leaves every later step exactly as it behaved before this step
// existed; only accepting wires agent.ApplyReset into the apply sequence.
// ──────────────────────────────────────────────────────────────────────────────

// resetAgentInventory is the per-agent projection the reset step shows and
// decides from. DisplayLines is always the exact, contract-owned
// sddruntime.ResetInventory(Platform) Display text: the step never
// hand-writes what a reset touches. Plan additionally reports what a reset
// would concretely find on this machine (user-added agent files, OpenCode
// entries outside Jarvis's owned set, and legacy 4R residue), so the wizard
// can be concrete, not just contractual. PlanErr (e.g. an unparseable
// settings.json) forces the step's answer to No: a reset must never proceed
// blind.
type resetAgentInventory struct {
	AgentName    string
	Platform     sddruntime.Platform
	ConfigDir    string
	DisplayLines []string
	Plan         agent.ResetPlan
	PlanErr      error
}

// initializeConfigResetStep computes the reset step's per-agent inventory
// from m.Agents. It must run after m.Agents is populated (agent.Detect), and
// is idempotent: calling it again recomputes the same inventory and resets
// the captured choice back to the default No.
func initializeConfigResetStep(m Model) Model {
	m.resetChoice = 0
	m.resetConsented = false
	if len(m.Agents) == 0 {
		m.resetInventories = nil
		return m
	}

	// The reset's durable-snapshot exclusion check is confinement-derived
	// (internal/agent.withinBackupAllowedRoots), so it needs the same home
	// directory ApplyReset later receives (opts.Home), computed the same way
	// every other wizard call site resolves it.
	home, _ := os.UserHomeDir()

	inventories := make([]resetAgentInventory, 0, len(m.Agents))
	for _, a := range m.Agents {
		inv := resetAgentInventory{AgentName: a.Name(), ConfigDir: a.ConfigDir()}
		platform, err := agent.PlatformForAgentName(a.Name())
		if err != nil {
			inv.PlanErr = err
			inventories = append(inventories, inv)
			continue
		}
		inv.Platform = platform
		for _, surface := range sddruntime.ResetInventory(platform) {
			inv.DisplayLines = append(inv.DisplayLines, surface.Display)
		}
		plan, planErr := agent.PlanReset(platform, a.ConfigDir(), home)
		if planErr != nil {
			inv.PlanErr = planErr
		} else {
			inv.Plan = plan
		}
		inventories = append(inventories, inv)
	}
	m.resetInventories = inventories
	return m
}

// resetPlanFailed reports whether any detected agent's reset plan could not
// be computed (e.g. an existing settings.json/opencode.json failed to
// parse). While true, the step refuses to offer Yes: it forces and keeps the
// answer at No.
func (m Model) resetPlanFailed() bool {
	for _, inv := range m.resetInventories {
		if inv.PlanErr != nil {
			return true
		}
	}
	return false
}

func updateConfigReset(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Nothing detected means nothing this step could offer to reset; move on
	// exactly as if the step were never shown.
	if len(m.Agents) == 0 {
		m.Step = StepScope
		return m, nil
	}

	failed := m.resetPlanFailed()
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown:
		if !failed {
			m.resetChoice = 1 - m.resetChoice
		}
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "j", "k":
			if !failed {
				m.resetChoice = 1 - m.resetChoice
			}
		}
	case tea.KeyEnter:
		m.Err = nil
		if failed {
			// A plan failure forces No regardless of where the cursor sat:
			// the wizard must never accept a reset it could not fully plan.
			m.resetChoice = 0
		}
		m.resetConsented = m.resetChoice == 1
		m.Step = StepScope
	}
	return m, nil
}

// replacedEntirelySurfaceDisplays returns the exact Display text of every
// ResetInventory surface marked ReplacedEntirely for platform, in contract
// order. Both the reset step's warning paragraph and the apply summary
// derive the "what gets entirely replaced" wording from here, so neither one
// can drift from the contract or from each other (issue #767 hardening
// R2-002/R2-003: this text used to be hand-written and named only Claude's
// agents/ directory, which was simply wrong for an OpenCode-only machine).
func replacedEntirelySurfaceDisplays(platform sddruntime.Platform) []string {
	var out []string
	for _, s := range sddruntime.ResetInventory(platform) {
		if s.ReplacedEntirely {
			out = append(out, s.Display)
		}
	}
	return out
}

// configResetSummaryLines projects the per-agent apply results into the
// final apply summary's reset section: one line per agent the reset
// actually ran for, and nothing at all when no agent had a reset applied
// (the wizard's decline path, or a machine with nothing to reset, says
// nothing about a reset -- matching the summary's tone before issue #767
// introduced this step).
//
// Every agent whose reset ran gets a line here, whether or not that agent's
// overall setup went on to fail: the snapshot ID is exactly what a human
// would need to find a durable backup by hand, so it must never disappear
// just because a later, unrelated step also failed (issue #767 hardening
// R4-001/R2-002/R2-003 -- the previous version silently dropped this line
// whenever res.Err was set, hiding the snapshot ID along with it).
func configResetSummaryLines(results []AgentApplyResult) []string {
	var lines []string
	for _, res := range results {
		if !res.ResetApplied {
			continue
		}
		lines = append(lines, fmt.Sprintf("[%s] configuration reset applied before install (snapshot %s)%s",
			res.AgentName, res.ResetSnapshotID, configResetOutcomeSuffix(res)))
	}
	return lines
}

// configResetOutcomeSuffix reports, for one agent whose reset ran, exactly
// what happened to it afterward: rolled back (a later step failed and this
// reset was undone), left in place after a rollback could not fully restore
// it, kept because a later failure deliberately did not roll it back (the
// runtime-verification-after-complete-reinstall case), or the ordinary
// entirely-replaced description when nothing downstream failed.
func configResetOutcomeSuffix(res AgentApplyResult) string {
	switch {
	case res.ResetRollbackErr != nil:
		return fmt.Sprintf(
			"; a later failure rolled it back, but rollback could not fully restore it: %v",
			res.ResetRollbackErr,
		)
	case res.ResetRolledBack:
		return "; a later failure rolled it back to its pre-reset configuration"
	case res.Err != nil:
		return "; the reinstall completed, so it was kept even though a later step failed"
	default:
		replaced := "every contract-owned surface listed in the reset step"
		if platform, err := agent.PlatformForAgentName(res.AgentName); err == nil {
			if displays := replacedEntirelySurfaceDisplays(platform); len(displays) > 0 {
				replaced = strings.Join(displays, "; ")
			}
		}
		return ": entirely replaced " + replaced
	}
}

func viewConfigReset(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	sb.WriteString(terminalui.HeaderRow("Setup › Configuration reset", terminalui.ModeBadge("normal"), m.width) + "\n\n")

	var contentSB strings.Builder
	contentSB.WriteString(terminalui.TitleStyle.Render("Optional: reset Jarvis-managed configuration before continuing") + "\n\n")
	contentSB.WriteString(
		"Accepting runs before the rest of setup. Declining leaves your current\n" +
			"configuration exactly as it is; nothing below is touched.\n\n",
	)

	// The "entirely replaced" warning is derived per detected platform from
	// the same contract ResetInventory the reset itself reads, never
	// hand-written here: it names exactly what is entirely replaced (an
	// agent file or entry a user added there themselves is lost), for
	// whichever platforms were actually detected on this machine.
	seenPlatform := make(map[sddruntime.Platform]bool)
	var replacedLines []string
	for _, inv := range m.resetInventories {
		if inv.PlanErr != nil || inv.Platform == "" || seenPlatform[inv.Platform] {
			continue
		}
		seenPlatform[inv.Platform] = true
		replacedLines = append(replacedLines, replacedEntirelySurfaceDisplays(inv.Platform)...)
	}
	if len(replacedLines) > 0 {
		contentSB.WriteString("Accepting entirely replaces (anything you added there yourself is lost):\n")
		for _, line := range replacedLines {
			contentSB.WriteString("  - " + line + "\n")
		}
		contentSB.WriteString("\n")
	}

	failed := m.resetPlanFailed()
	for _, inv := range m.resetInventories {
		contentSB.WriteString(terminalui.ColumnHeaderStyle.Render(strings.ToUpper(inv.AgentName)+" ("+inv.ConfigDir+")") + "\n")
		if inv.PlanErr != nil {
			contentSB.WriteString(errorStyle.Render("  Cannot plan a reset: "+inv.PlanErr.Error()) + "\n\n")
			continue
		}
		for _, line := range inv.DisplayLines {
			contentSB.WriteString("  - " + line + "\n")
		}
		if len(inv.Plan.Changes) > 0 {
			contentSB.WriteString(terminalui.DimTextStyle.Render("  Found on this machine:") + "\n")
			for _, c := range inv.Plan.Changes {
				contentSB.WriteString(terminalui.DimTextStyle.Render("    - "+c.Path+": "+c.Detail) + "\n")
			}
		}
		if len(inv.Plan.NotInDurableSnapshot) > 0 {
			contentSB.WriteString(errorStyle.Render(
				"  These files will only be recoverable through the in-process rollback of this run, "+
					"not from the durable backup afterward:",
			) + "\n")
			for _, path := range inv.Plan.NotInDurableSnapshot {
				contentSB.WriteString(errorStyle.Render("    - "+path) + "\n")
			}
		}
		contentSB.WriteString("\n")
	}

	if failed {
		contentSB.WriteString(errorStyle.Render(
			"A configuration file could not be parsed, so a reset cannot be planned safely. "+
				"The answer is forced to No; fix the file above to enable a reset.",
		) + "\n\n")
	}

	noLine := "  No, keep my current configuration"
	yesLine := "  Yes, reset it before continuing"
	if m.resetChoice == 0 {
		noLine = terminalui.SelectedRow("No, keep my current configuration", w)
	} else {
		yesLine = terminalui.SelectedRow("Yes, reset it before continuing", w)
	}
	contentSB.WriteString(noLine + "\n" + yesLine + "\n")

	sb.WriteString(terminalui.BorderedPanel(contentSB.String(), w) + "\n")

	hints := []terminalui.KeyHint{
		{Key: "↑/↓", Desc: "cambiar"},
		{Key: "Enter", Desc: "confirmar"},
	}
	sb.WriteString(terminalui.HelpBar(hints, "normal", m.width))
	return sb.String()
}
