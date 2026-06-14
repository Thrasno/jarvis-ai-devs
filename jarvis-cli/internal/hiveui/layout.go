package hiveui

import "github.com/Thrasno/jarvis-ai-devs/jarvis-cli/internal/terminalui"

// KeyHint is a key and description pair for the help bar.
// Kept as a local type so existing unkeyed struct literals in model.go compile
// without modification. The helpBar wrapper converts to terminalui.KeyHint.
type KeyHint struct {
	Key  string
	Desc string
}

// helpBar renders a footer help bar, converting the local KeyHint slice
// to terminalui.KeyHint so callers in model.go need no changes.
func helpBar(hints []KeyHint, mode string, termWidth int) string {
	th := make([]terminalui.KeyHint, len(hints))
	for i, h := range hints {
		th[i] = terminalui.KeyHint{Key: h.Key, Desc: h.Desc}
	}
	return terminalui.HelpBar(th, mode, termWidth)
}
