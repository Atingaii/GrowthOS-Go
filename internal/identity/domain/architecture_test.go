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

func TestLesson32IdentityDomainRemainsPureAndSecretFree(t *testing.T) {
	t.Parallel()

	violations, files, err := identityDomainArchitectureViolations(".")
	if err != nil {
		t.Fatalf("inspect Identity domain: %v", err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
	if files == 0 {
		t.Fatal("Identity architecture guard parsed no production Go files")
	}
}

func TestLesson32ArchitectureGuardDetectsForbiddenBoundaries(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixture := filepath.Join(root, "leak.go")
	source := `package fixture
import "github.com/gin-gonic/gin"
type RawPassword []byte
type SessionToken string
type Cookie string
type Policy struct{}
type Permission struct{}
type Role struct{}
type Scope struct{}
type AttributeBag map[string]any
var _ = gin.Context{}
`
	if err := os.WriteFile(fixture, []byte(source), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	violations, files, err := identityDomainArchitectureViolations(root)
	if err != nil {
		t.Fatalf("inspect fixture: %v", err)
	}
	if files != 1 {
		t.Fatalf("parsed files = %d, want 1", files)
	}
	joined := strings.Join(violations, "\n")
	for _, expected := range []string{
		"imports package outside the pure Identity domain allowlist",
		"declares forbidden boundary RawPassword",
		"declares forbidden boundary SessionToken",
		"declares forbidden boundary Cookie",
		"declares forbidden boundary Policy",
		"declares forbidden boundary Permission",
		"declares forbidden boundary Role",
		"declares forbidden boundary Scope",
		"declares untyped string bag",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("violations %q do not contain %q", joined, expected)
		}
	}
}

func identityDomainArchitectureViolations(root string) ([]string, int, error) {
	allowedImports := map[string]struct{}{
		"errors":   {},
		"fmt":      {},
		"log/slog": {},
		"time":     {},
	}
	forbiddenIdentifiers := map[string]struct{}{
		"RawPassword":       {},
		"PlaintextPassword": {},
		"Password":          {},
		"RawToken":          {},
		"SessionToken":      {},
		"Cookie":            {},
		"HTTPRequest":       {},
		"HTTPResponse":      {},
		"SQL":               {},
		"Gin":               {},
		"Redis":             {},
		"Role":              {},
		"Permission":        {},
		"Scope":             {},
		"Policy":            {},
		"Authorization":     {},
		"JWT":               {},
		"Middleware":        {},
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
					"%s imports package outside the pure Identity domain allowlist %q",
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
						"%s declares forbidden boundary %s",
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
						"%s declares untyped string bag",
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
