package tui

import (
	"fmt"
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
		plan, planErr := agent.PlanReset(platform, a.ConfigDir())
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

// configResetSummaryLines projects the per-agent apply results into the
// final apply summary's reset section: one line per agent the reset
// actually ran for, naming its durable snapshot ID, and nothing at all when
// no agent had a reset applied (the wizard's decline path, or a machine with
// nothing to reset, says nothing about a reset -- matching the summary's
// tone before issue #767 introduced this step).
func configResetSummaryLines(results []AgentApplyResult) []string {
	var lines []string
	for _, res := range results {
		if !res.ResetApplied {
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"[%s] configuration reset applied before install (snapshot %s): replaced the agents/ directory and every other contract-owned surface listed in the reset step",
			res.AgentName, res.ResetSnapshotID,
		))
	}
	return lines
}

func viewConfigReset(m Model) string {
	w := terminalui.PanelWidth(m.width)
	var sb strings.Builder

	sb.WriteString(terminalui.HeaderRow("Setup › Configuration reset", terminalui.ModeBadge("normal"), m.width) + "\n\n")

	var contentSB strings.Builder
	contentSB.WriteString(terminalui.TitleStyle.Render("Optional: reset Jarvis-managed configuration before continuing") + "\n\n")
	contentSB.WriteString(
		"Accepting entirely replaces ~/.claude/agents/ and the \"agent\" section of\n" +
			"opencode.json before the rest of setup runs: any agent files or entries you\n" +
			"added there yourself are lost. Declining leaves your current configuration\n" +
			"exactly as it is; nothing below is touched.\n\n",
	)

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
