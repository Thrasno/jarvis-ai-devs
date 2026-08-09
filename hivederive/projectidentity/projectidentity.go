// Package projectidentity defines the shared canonical identity contract for Hive projects.
package projectidentity

import (
	"strings"

	"golang.org/x/text/cases"
)

const ContractVersion = "v1"

// CapabilityReproject is the exact sync capability string a client declares to
// receive reproject mutations, and the exact string hive-api matches on before
// it sends one.
//
// It lives in this shared module because a near miss is worse than silence: with
// two hand-maintained copies, one side renaming or mistyping the string makes
// the server withhold every reproject forever while both ends keep reporting a
// healthy sync. Referencing one constant makes that desynchronisation a compile
// error instead.
const CapabilityReproject = "mutation.reproject"

var fold = cases.Fold()

// Key is a canonical project identity suitable for storage and lookup.
type Key string

// Canonical returns the contract key for a project spelling.
func Canonical(project string) Key {
	parts := strings.FieldsFunc(fold.String(strings.TrimSpace(project)), isSeparator)
	return Key(strings.Join(parts, "-"))
}

func isSeparator(r rune) bool {
	switch r {
	case ' ', '/', '_', '.', '-':
		return true
	default:
		return false
	}
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
	{Name: "trims outer whitespace and folds dots", Input: " Foo.Bar ", Want: "foo-bar"},
	{Name: "folds unicode case", Input: "Straße", Want: "strasse"},
	{Name: "folds slash separator", Input: "foo/bar", Want: "foo-bar"},
	{Name: "folds underscore separator", Input: "foo_bar", Want: "foo-bar"},
	{Name: "folds dash separator", Input: "foo-bar", Want: "foo-bar"},
	{Name: "collapses separator runs", Input: "foo---bar___baz", Want: "foo-bar-baz"},
}
