package hook

import (
	"log"
	"os"
)

// logger is the package-level stderr-only logger for hook diagnostics.
//
// Hook stdout is reserved for the JSON protocol response the caller parses, so
// all observability output goes to stderr and never interferes with the hook
// contract. Failures in fire-and-forget daemon notifications are non-fatal, but
// they are logged here so silent sessions become diagnosable instead of
// vanishing.
var logger = log.New(os.Stderr, "[jarvis] ", log.LstdFlags)
