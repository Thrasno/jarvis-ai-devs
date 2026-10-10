package applyprogress

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestPlanOutcomesListsEveryDeclaredOutcome keeps PlanOutcomes exhaustive, so
// callers that map every outcome fail their own tests when a new one appears.
func TestPlanOutcomesListsEveryDeclaredOutcome(t *testing.T) {
	names, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var declared []PlanOutcome
	for _, name := range names {
		if !strings.HasSuffix(name.Name(), ".go") || strings.HasSuffix(name.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				if ident, ok := value.Type.(*ast.Ident); !ok || ident.Name != "PlanOutcome" {
					continue
				}
				for _, literal := range value.Values {
					text, err := strconv.Unquote(literal.(*ast.BasicLit).Value)
					if err != nil {
						t.Fatal(err)
					}
					declared = append(declared, PlanOutcome(text))
				}
			}
		}
	}
	listed := PlanOutcomes()
	slices.Sort(declared)
	sorted := slices.Clone(listed)
	slices.Sort(sorted)
	if len(declared) == 0 || !slices.Equal(declared, sorted) {
		t.Fatalf("PlanOutcomes() = %v, declared PlanOutcome constants = %v", listed, declared)
	}
}
