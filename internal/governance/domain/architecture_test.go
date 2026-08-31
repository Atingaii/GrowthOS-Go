package domain

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

const lesson31DomainImport = "github.com/Atingaii/GrowthOS-Go/internal/governance/domain"

func TestLesson31GovernanceDomainRemainsPureTypedAndBounded(t *testing.T) {
	t.Parallel()

	forbiddenIdentifiers := map[string]struct{}{
		"RuleEngine":          {},
		"PolicyEngine":        {},
		"ExpressionEngine":    {},
		"Registry":            {},
		"PolicyRegistry":      {},
		"Plugin":              {},
		"DSL":                 {},
		"FactBag":             {},
		"AttributeBag":        {},
		"IsAdmin":             {},
		"SuperAdmin":          {},
		"AllowAll":            {},
		"PrincipalPermission": {},
		"DirectPermission":    {},
		"Session":             {},
		"Credential":          {},
		"Password":            {},
		"Token":               {},
		"JWT":                 {},
		"Middleware":          {},
	}
	violations, files, err := lesson31DomainArchitectureViolations(
		".",
		forbiddenIdentifiers,
	)
	if err != nil {
		t.Fatalf("inspect Governance domain: %v", err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
	if files == 0 {
		t.Fatal("Governance architecture guard parsed no production Go files")
	}
}

func TestLesson31GovernancePolicyKernelRemainsOutsideRuntimeComposition(t *testing.T) {
	t.Parallel()

	repositoryRoot := lesson31RepositoryRoot(t)
	domainRoot := filepath.Clean(filepath.Join(repositoryRoot, "internal", "governance", "domain")) +
		string(filepath.Separator)
	violations, files, err := lesson31ExternalGovernanceImportViolations(
		repositoryRoot,
		domainRoot,
	)
	if err != nil {
		t.Fatalf("inspect runtime composition: %v", err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
	if files == 0 {
		t.Fatal("runtime architecture guard parsed no external production Go files")
	}
}

func TestLesson31DomainGuardDetectsNestedThirdPartyImportAndUntypedBag(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "future", "policy")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested fixture: %v", err)
	}
	fixture := filepath.Join(nested, "engine.go")
	source := `package policy
import "example.com/policy/engine"
type AttributeBag map[string]any
var _ = engine.Decide
`
	if err := os.WriteFile(fixture, []byte(source), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	violations, files, err := lesson31DomainArchitectureViolations(
		root,
		map[string]struct{}{"AttributeBag": {}},
	)
	if err != nil {
		t.Fatalf("inspect fixture: %v", err)
	}
	if files != 1 {
		t.Fatalf("parsed files = %d, want 1", files)
	}
	joined := strings.Join(violations, "\n")
	for _, expected := range []string{
		"imports package outside the reviewed pure-domain allowlist",
		"declares forbidden authorization shortcut AttributeBag",
		"declares untyped string policy bag",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("violations %q do not contain %q", joined, expected)
		}
	}
}

func TestLesson31DomainGuardDetectsPrematureAuthenticationVocabulary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixture := filepath.Join(root, "session.go")
	source := `package domain
type Session struct{}
type Credential struct{}
type Password string
type Token string
type JWT string
type Middleware func()
`
	if err := os.WriteFile(fixture, []byte(source), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	forbidden := map[string]struct{}{
		"Session": {}, "Credential": {}, "Password": {},
		"Token": {}, "JWT": {}, "Middleware": {},
	}
	violations, files, err := lesson31DomainArchitectureViolations(root, forbidden)
	if err != nil {
		t.Fatalf("inspect fixture: %v", err)
	}
	if files != 1 {
		t.Fatalf("parsed files = %d, want 1", files)
	}
	joined := strings.Join(violations, "\n")
	for identifier := range forbidden {
		if !strings.Contains(joined, "declares forbidden authorization shortcut "+identifier) {
			t.Fatalf("violations %q do not contain %q", joined, identifier)
		}
	}
}

func TestLesson31RuntimeGuardDetectsNestedGovernanceImport(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "cmd", "future")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested fixture: %v", err)
	}
	fixture := filepath.Join(nested, "main.go")
	source := `package main
import governance "github.com/Atingaii/GrowthOS-Go/internal/governance/domain/subpackage"
var _ = governance.Decision{}
`
	if err := os.WriteFile(fixture, []byte(source), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	violations, files, err := lesson31ExternalGovernanceImportViolations(root, "")
	if err != nil {
		t.Fatalf("inspect fixture: %v", err)
	}
	if files != 1 {
		t.Fatalf("parsed files = %d, want 1", files)
	}
	if joined := strings.Join(violations, "\n"); !strings.Contains(joined, "prematurely imports the Lesson 31 policy kernel") {
		t.Fatalf("violations = %q", joined)
	}
}

func lesson31DomainArchitectureViolations(
	root string,
	forbiddenIdentifiers map[string]struct{},
) ([]string, int, error) {
	allowedImports := map[string]struct{}{
		"cmp":    {},
		"errors": {},
		"fmt":    {},
		"slices": {},
		"time":   {},
	}
	var violations []string
	parsedFiles := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		parsedFiles++
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import in %s: %w", path, err)
			}
			if _, allowed := allowedImports[importPath]; !allowed {
				violations = append(violations, fmt.Sprintf(
					"%s imports package outside the reviewed pure-domain allowlist %q",
					path,
					importPath,
				))
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.Ident:
				if _, forbidden := forbiddenIdentifiers[typed.Name]; forbidden {
					violations = append(violations, fmt.Sprintf(
						"%s declares forbidden authorization shortcut %s",
						path,
						typed.Name,
					))
				}
			case *ast.MapType:
				key, keyIsIdentifier := typed.Key.(*ast.Ident)
				value, valueIsIdentifier := typed.Value.(*ast.Ident)
				_, valueIsInterface := typed.Value.(*ast.InterfaceType)
				if keyIsIdentifier && key.Name == "string" &&
					((valueIsIdentifier && value.Name == "any") || valueIsInterface) {
					violations = append(violations, fmt.Sprintf(
						"%s declares untyped string policy bag",
						path,
					))
				}
			}
			return true
		})
		return nil
	})
	return violations, parsedFiles, err
}

func lesson31ExternalGovernanceImportViolations(
	root string,
	domainRoot string,
) ([]string, int, error) {
	var violations []string
	parsedFiles := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor", "dist":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		cleanPath := filepath.Clean(path)
		if domainRoot != "" && strings.HasPrefix(cleanPath, domainRoot) {
			return nil
		}
		parsedFiles++
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import in %s: %w", path, err)
			}
			if importPath == lesson31DomainImport ||
				strings.HasPrefix(importPath, lesson31DomainImport+"/") {
				violations = append(violations, fmt.Sprintf(
					"%s prematurely imports the Lesson 31 policy kernel",
					path,
				))
			}
		}
		return nil
	})
	return violations, parsedFiles, err
}

func lesson31RepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect go.mod in %s: %v", directory, err)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not locate repository root from Governance domain")
		}
		directory = parent
	}
}
