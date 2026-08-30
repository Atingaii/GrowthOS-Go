package application

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const moduleImportPrefix = "github.com/Atingaii/GrowthOS-Go/"

func TestLesson26ParticipationArchitectureKeepsBoundedEligibilityChain(t *testing.T) {
	forbiddenTypes := map[string]struct{}{
		"Rule":              {},
		"RuleChain":         {},
		"RuleEngine":        {},
		"RuleTree":          {},
		"Specification":     {},
		"EvaluationContext": {},
		"RulePriority":      {},
		"DSL":               {},
	}
	packages := []struct {
		name                 string
		directory            string
		allowedProjectImport string
	}{
		{name: "domain", directory: filepath.Join("..", "domain")},
		{
			name:                 "application",
			directory:            ".",
			allowedProjectImport: moduleImportPrefix + "internal/participation/domain",
		},
	}
	for _, checkedPackage := range packages {
		t.Run(checkedPackage.name, func(t *testing.T) {
			entries, err := os.ReadDir(checkedPackage.directory)
			if err != nil {
				t.Fatalf("read %s package: %v", checkedPackage.name, err)
			}
			parsedProductionFiles := 0
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				parsedProductionFiles++
				path := filepath.Join(checkedPackage.directory, entry.Name())
				file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
				if err != nil {
					t.Fatalf("parse %s: %v", path, err)
				}
				for _, imported := range file.Imports {
					importPath, err := strconv.Unquote(imported.Path.Value)
					if err != nil {
						t.Fatalf("unquote import in %s: %v", path, err)
					}
					if !strings.HasPrefix(importPath, moduleImportPrefix) {
						continue
					}
					if importPath != checkedPackage.allowedProjectImport {
						t.Errorf("%s imports forbidden project package %q", path, importPath)
					}
				}
				for _, declaration := range file.Decls {
					general, ok := declaration.(*ast.GenDecl)
					if !ok || general.Tok != token.TYPE {
						continue
					}
					for _, specification := range general.Specs {
						typeSpecification := specification.(*ast.TypeSpec)
						if typeSpecification.TypeParams != nil && len(typeSpecification.TypeParams.List) > 0 {
							t.Errorf("%s prematurely declares generic type %s", path, typeSpecification.Name.Name)
						}
						if _, forbidden := forbiddenTypes[typeSpecification.Name.Name]; forbidden {
							t.Errorf("%s prematurely declares generic type %s", path, typeSpecification.Name.Name)
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
						t.Errorf("%s prematurely declares an untyped string fact bag", path)
					}
					return true
				})
			}
			if parsedProductionFiles == 0 {
				t.Fatalf("no production Go files found in %s", checkedPackage.directory)
			}
		})
	}
}
