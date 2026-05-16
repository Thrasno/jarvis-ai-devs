package tui

// CockpitActionID is the stable dispatch identifier for a cockpit action.
type CockpitActionID string

const (
	CockpitActionInstall        CockpitActionID = "install-reconfigure"
	CockpitActionPersona        CockpitActionID = "persona"
	CockpitActionConfig         CockpitActionID = "config"
	CockpitActionHiveCloudLogin CockpitActionID = "hive-cloud-login"
	CockpitActionDoctor         CockpitActionID = "doctor"
	CockpitActionVerify         CockpitActionID = "verify"
	CockpitActionBackup         CockpitActionID = "backup"
	CockpitActionRestore        CockpitActionID = "restore"
	CockpitActionReconcile      CockpitActionID = "reconcile"
	CockpitActionUninstall      CockpitActionID = "uninstall"
	CockpitActionExit           CockpitActionID = "exit"
)

// CockpitActionKind describes how an action will be routed once handlers exist.
type CockpitActionKind string

const (
	CockpitActionKindWizard   CockpitActionKind = "wizard"
	CockpitActionKindReadOnly CockpitActionKind = "read-only"
	CockpitActionKindMutating CockpitActionKind = "mutating"
	CockpitActionKindExit     CockpitActionKind = "exit"
)

// CockpitDangerLevel marks actions that require stronger confirmation later.
type CockpitDangerLevel string

const (
	CockpitDangerNone   CockpitDangerLevel = "none"
	CockpitDangerStrong CockpitDangerLevel = "strong"
)

// CockpitAction defines the static menu and future dispatch contract.
type CockpitAction struct {
	ID          CockpitActionID
	Label       string
	Description string
	Kind        CockpitActionKind
	Danger      CockpitDangerLevel
}

var defaultCockpitActions = []CockpitAction{
	{ID: CockpitActionInstall, Label: "Install/Reconfigure", Description: "Start or resume the setup wizard.", Kind: CockpitActionKindWizard, Danger: CockpitDangerNone},
	{ID: CockpitActionPersona, Label: "Persona", Description: "Review and apply persona presets.", Kind: CockpitActionKindMutating, Danger: CockpitDangerNone},
	{ID: CockpitActionConfig, Label: "Config", Description: "Show current configuration.", Kind: CockpitActionKindReadOnly, Danger: CockpitDangerNone},
	{ID: CockpitActionHiveCloudLogin, Label: "Hive Cloud Login", Description: "Authenticate against Hive Cloud.", Kind: CockpitActionKindMutating, Danger: CockpitDangerNone},
	{ID: CockpitActionDoctor, Label: "Doctor", Description: "Run diagnostics.", Kind: CockpitActionKindReadOnly, Danger: CockpitDangerNone},
	{ID: CockpitActionVerify, Label: "Verify", Description: "Verify installed agent runtime contracts.", Kind: CockpitActionKindReadOnly, Danger: CockpitDangerNone},
	{ID: CockpitActionBackup, Label: "Backup", Description: "Create lifecycle backups.", Kind: CockpitActionKindMutating, Danger: CockpitDangerNone},
	{ID: CockpitActionRestore, Label: "Restore", Description: "Restore from a lifecycle backup.", Kind: CockpitActionKindMutating, Danger: CockpitDangerStrong},
	{ID: CockpitActionReconcile, Label: "Reconcile", Description: "Preview and apply reconciliation plans.", Kind: CockpitActionKindMutating, Danger: CockpitDangerStrong},
	{ID: CockpitActionUninstall, Label: "Uninstall", Description: "Remove installed Jarvis artifacts.", Kind: CockpitActionKindMutating, Danger: CockpitDangerStrong},
	{ID: CockpitActionExit, Label: "Exit", Description: "Leave the cockpit.", Kind: CockpitActionKindExit, Danger: CockpitDangerNone},
}

// CockpitActions returns the approved cockpit action registry in display order.
func CockpitActions() []CockpitAction {
	actions := make([]CockpitAction, len(defaultCockpitActions))
	copy(actions, defaultCockpitActions)
	return actions
}

// CockpitActionByID finds an action by its stable dispatch identifier.
func CockpitActionByID(actions []CockpitAction, id CockpitActionID) (CockpitAction, bool) {
	for _, action := range actions {
		if action.ID == id {
			return action, true
		}
	}
	return CockpitAction{}, false
}
