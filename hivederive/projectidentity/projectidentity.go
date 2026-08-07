// Package projectidentity defines the shared canonical identity contract for Hive projects.
package projectidentity

import (
	"strings"
	"time"

	"golang.org/x/text/cases"
)

const ContractVersion = "v1"

var fold = cases.Fold()

// Key is a canonical project identity suitable for storage and lookup.
type Key string

// Canonical returns the contract key for a project spelling.
func Canonical(project string) Key {
	return Key(fold.String(strings.TrimSpace(project)))
}

func (key Key) String() string {
	return string(key)
}

// Vector is a shared canonicalization example for module conformance tests.
type Vector struct {
	Name  string
	Input string
	Want  string
}

// ConformanceVectors returns a copy of the contract examples.
func ConformanceVectors() []Vector {
	return append([]Vector(nil), conformanceVectors...)
}

var conformanceVectors = []Vector{
	{Name: "trims outer whitespace", Input: " Foo.Bar ", Want: "foo.bar"},
	{Name: "folds unicode case", Input: "Straße", Want: "strasse"},
	{Name: "preserves dot separator", Input: "foo.bar", Want: "foo.bar"},
	{Name: "preserves underscore separator", Input: "foo_bar", Want: "foo_bar"},
	{Name: "preserves dash separator", Input: "foo-bar", Want: "foo-bar"},
}

// Registration records a project spelling observed at a point in time.
type Registration struct {
	Spelling     string
	RegisteredAt time.Time
}

// DisplayName prefers the remote spelling, otherwise the oldest registered spelling.
func DisplayName(remote string, registrations []Registration) string {
	if remote = strings.TrimSpace(remote); remote != "" {
		return remote
	}
	if len(registrations) == 0 {
		return ""
	}

	oldest := registrations[0]
	for _, registration := range registrations[1:] {
		if registration.RegisteredAt.Before(oldest.RegisteredAt) {
			oldest = registration
		}
	}
	return oldest.Spelling
}
