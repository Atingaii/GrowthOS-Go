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

const modulePath = "github.com/Atingaii/GrowthOS-Go/"

func TestProductionImportsStayInsideIdentityApplicationBoundary(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	allowedInternal := map[string]bool{
		modulePath + "internal/identity/domain":   true,
		modulePath + "internal/governance/domain": true,
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Clean(entry.Name())
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		for _, imported := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("unquote import in %s: %v", path, unquoteErr)
			}
			if strings.HasPrefix(importPath, modulePath+"internal/") &&
				!allowedInternal[importPath] {
				t.Errorf("%s imports forbidden internal package %q", path, importPath)
			}
			if importPath == "database/sql" || strings.Contains(importPath, "gin") ||
				strings.Contains(importPath, "redis") {
				t.Errorf("%s imports forbidden adapter dependency %q", path, importPath)
			}
		}
		ast.Inspect(parsed, func(ast.Node) bool { return true })
	}
}
