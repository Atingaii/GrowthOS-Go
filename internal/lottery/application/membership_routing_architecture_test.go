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
		"Rule":               {},
		"RuleChain":          {},
		"RuleTree":           {},
		"RuleNode":           {},
		"RuleEdge":           {},
		"RuleEngine":         {},
		"DecisionEngine":     {},
		"EvaluationContext":  {},
		"RulePriority":       {},
		"Registry":           {},
		"RuleRegistry":       {},
		"OperatorRegistry":   {},
		"EvaluationRegistry": {},
		"FactBag":            {},
		"Expression":         {},
		"ExpressionEngine":   {},
		"ScriptEngine":       {},
		"DSL":                {},
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

func TestLesson29GraphEvaluationHasNoRuntimeOrHTTPAssembly(t *testing.T) {
	forbiddenIdentifiers := map[string]struct{}{
		"StrategyRoutingGraphEvaluationService":    {},
		"NewStrategyRoutingGraphEvaluationService": {},
		"EvaluateStrategyRoutingGraph":             {},
		"StrategyRoutingGraphDecision":             {},
		"StrategyRoutingGraphStepBudget":           {},
	}
	for _, root := range []string{
		filepath.Join("..", "..", "..", "cmd"),
		filepath.Join("..", "adapter", "httpapi"),
		filepath.Join("..", "..", "infrastructure", "httpapi"),
		filepath.Join("..", "..", "infrastructure", "httpserver"),
		filepath.Join("..", "..", "platform", "appconfig"),
	} {
		violations, parsedProductionFiles, err := lesson29ForbiddenIdentifierViolations(
			root,
			forbiddenIdentifiers,
		)
		if err != nil {
			t.Fatalf("inspect %s: %v", root, err)
		}
		for _, violation := range violations {
			t.Error(violation)
		}
		if parsedProductionFiles == 0 {
			t.Fatalf("no production Go files found in %s", root)
		}
	}

	sourceRoots := []lesson29SourceRoot{
		{
			path: filepath.Join("..", "..", "..", "deploy", "compose"),
			include: func(path string, entry fs.DirEntry) bool {
				name := entry.Name()
				return (strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) &&
					!strings.Contains(name, "acceptance")
			},
		},
		{
			path: filepath.Join("..", "..", "..", "deploy", "docker"),
			include: func(_ string, entry fs.DirEntry) bool {
				return strings.HasPrefix(entry.Name(), "Dockerfile") ||
					strings.HasSuffix(entry.Name(), ".conf") ||
					strings.HasSuffix(entry.Name(), ".sh")
			},
		},
		{
			path: filepath.Join("..", "..", "..", "web"),
			include: func(path string, entry fs.DirEntry) bool {
				if strings.Contains(path, string(filepath.Separator)+"node_modules"+string(filepath.Separator)) ||
					strings.Contains(path, string(filepath.Separator)+"mocks"+string(filepath.Separator)) {
					return false
				}
				name := entry.Name()
				if strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
					return false
				}
				extension := filepath.Ext(name)
				return extension == ".ts" || extension == ".tsx" ||
					extension == ".js" || extension == ".jsx" || extension == ".json"
			},
		},
	}
	for _, root := range sourceRoots {
		violations, scannedProductionFiles, err := lesson29ForbiddenSourceViolations(
			root,
			forbiddenIdentifiers,
		)
		if err != nil {
			t.Fatalf("inspect production sources %s: %v", root.path, err)
		}
		for _, violation := range violations {
			t.Error(violation)
		}
		if scannedProductionFiles == 0 {
			t.Fatalf("no production source files found in %s", root.path)
		}
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

func lesson29ForbiddenIdentifierViolations(
	root string,
	forbiddenIdentifiers map[string]struct{},
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
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if _, forbidden := forbiddenIdentifiers[identifier.Name]; forbidden {
				violations = append(violations, fmt.Sprintf(
					"%s prematurely wires graph evaluation identifier %s",
					path,
					identifier.Name,
				))
			}
			return true
		})
		return nil
	})
	return violations, parsedProductionFiles, err
}

type lesson29SourceRoot struct {
	path    string
	include func(path string, entry fs.DirEntry) bool
}

func lesson29ForbiddenSourceViolations(
	root lesson29SourceRoot,
	forbiddenIdentifiers map[string]struct{},
) ([]string, int, error) {
	var violations []string
	scannedProductionFiles := 0
	err := filepath.WalkDir(root.path, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".vite" {
				return filepath.SkipDir
			}
			return nil
		}
		if !root.include(path, entry) {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		scannedProductionFiles++
		for identifier := range forbiddenIdentifiers {
			if strings.Contains(string(source), identifier) {
				violations = append(violations, fmt.Sprintf(
					"%s prematurely wires graph evaluation identifier %s",
					path,
					identifier,
				))
			}
		}
		return nil
	})
	return violations, scannedProductionFiles, err
}
