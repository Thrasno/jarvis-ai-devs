package tui

import "fmt"

// RunTimeline prints the timeline placeholder for MVP 1.
// Full TUI timeline is deferred to MVP 2.
func RunTimeline() error {
	fmt.Println("Timeline coming in MVP 2. Use mem_search for now.")
	fmt.Println("To search memories: call mem_search from within Claude Code or OpenCode.")
	return nil
}
