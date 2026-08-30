package application

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const lesson27ModuleImportPrefix = "github.com/Atingaii/GrowthOS-Go/"

func TestLesson27MembershipRoutingKeepsContextOwnershipAndEngineStopLine(t *testing.T) {
	forbiddenTypes := map[string]struct{}{
		"Rule":              {},
		"RuleChain":         {},
		"RuleTree":          {},
		"RuleNode":          {},
		"RuleEdge":          {},
		"RuleEngine":        {},
		"DecisionEngine":    {},
		"EvaluationContext": {},
		"RulePriority":      {},
		"DSL":               {},
	}
	packages := []struct {
		name                 string
		directory            string
		allowedProjectImport string
	}{
		{name: "lottery domain", directory: filepath.Join("..", "domain")},
		{
			name:                 "lottery application",
			directory:            ".",
			allowedProjectImport: lesson27ModuleImportPrefix + "internal/lottery/domain",
		},
		{name: "participation domain", directory: filepath.Join("..", "..", "participation", "domain")},
		{
			name:                 "participation application",
			directory:            filepath.Join("..", "..", "participation", "application"),
			allowedProjectImport: lesson27ModuleImportPrefix + "internal/participation/domain",
		},
	}
	for _, checkedPackage := range packages {
		t.Run(checkedPackage.name, func(t *testing.T) {
			violations, parsedProductionFiles, err := lesson27ArchitectureViolations(
				checkedPackage.directory,
				checkedPackage.allowedProjectImport,
				forbiddenTypes,
			)
			if err != nil {
				t.Fatalf("inspect %s: %v", checkedPackage.directory, err)
			}
			for _, violation := range violations {
				t.Error(violation)
			}
			if parsedProductionFiles == 0 {
				t.Fatalf("no production Go files found in %s", checkedPackage.directory)
			}
		})
	}
}

func TestLesson27ArchitectureGuardScansNestedGenericFunctions(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "future", "routing")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested fixture directory: %v", err)
	}
	fixture := filepath.Join(nested, "generic_function.go")
	if err := os.WriteFile(fixture, []byte("package routing\nfunc route[T any](value T) T { return value }\n"), 0o600); err != nil {
		t.Fatalf("write generic function fixture: %v", err)
	}

	violations, parsedProductionFiles, err := lesson27ArchitectureViolations(root, "", map[string]struct{}{})
	if err != nil {
		t.Fatalf("inspect nested fixture: %v", err)
	}
	if parsedProductionFiles != 1 {
		t.Fatalf("parsed production files = %d, want 1", parsedProductionFiles)
	}
	if !strings.Contains(strings.Join(violations, "\n"), "prematurely declares generic function route") {
		t.Fatalf("violations = %q, want nested generic function rejection", violations)
	}
}

func lesson27ArchitectureViolations(
	root string,
	allowedProjectImport string,
	forbiddenTypes map[string]struct{},
) ([]string, int, error) {
	var violations []string
	parsedProductionFiles := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		parsedProductionFiles++
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import in %s: %w", path, err)
			}
			if strings.HasPrefix(importPath, lesson27ModuleImportPrefix) && importPath != allowedProjectImport {
				violations = append(violations, fmt.Sprintf("%s imports forbidden project package %q", path, importPath))
			}
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.GenDecl:
				if typed.Tok != token.TYPE {
					continue
				}
				for _, specification := range typed.Specs {
					typeSpecification := specification.(*ast.TypeSpec)
					if typeSpecification.TypeParams != nil && len(typeSpecification.TypeParams.List) > 0 {
						violations = append(violations, fmt.Sprintf("%s prematurely declares generic type %s", path, typeSpecification.Name.Name))
					}
					if _, forbidden := forbiddenTypes[typeSpecification.Name.Name]; forbidden {
						violations = append(violations, fmt.Sprintf("%s prematurely declares generic routing type %s", path, typeSpecification.Name.Name))
					}
				}
			case *ast.FuncDecl:
				if typed.Type.TypeParams != nil && len(typed.Type.TypeParams.List) > 0 {
					violations = append(violations, fmt.Sprintf("%s prematurely declares generic function %s", path, typed.Name.Name))
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			mapType, ok := node.(*ast.MapType)
			if !ok {
				return true
			}
			key, keyIsString := mapType.Key.(*ast.Ident)
			value, valueIsIdent := mapType.Value.(*ast.Ident)
			_, valueIsInterface := mapType.Value.(*ast.InterfaceType)
			if keyIsString && key.Name == "string" &&
				((valueIsIdent && value.Name == "any") || valueIsInterface) {
				violations = append(violations, fmt.Sprintf("%s prematurely declares an untyped string fact bag", path))
			}
			return true
		})
		return nil
	})
	return violations, parsedProductionFiles, err
}
