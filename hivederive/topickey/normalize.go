// Package topickey owns the canonical representation of Hive topic keys.
package topickey

import "strings"

// Normalize trims surrounding whitespace and maps blank values to nil.
func Normalize(topicKey *string) *string {
	if topicKey == nil {
		return nil
	}
	normalized := strings.TrimSpace(*topicKey)
	if normalized == "" {
		return nil
	}
	return &normalized
}
