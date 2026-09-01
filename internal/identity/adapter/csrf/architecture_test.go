package csrf

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductionImportsStayInsideCSRFBoundary(t *testing.T) {
	const modulePath = "github.com/Atingaii/GrowthOS-Go/"
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(
			token.NewFileSet(), filepath.Clean(entry.Name()), nil, parser.ImportsOnly,
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(importPath, modulePath+"internal/") &&
				importPath != modulePath+"internal/identity/domain" {
				t.Errorf("forbidden internal dependency %q", importPath)
			}
			if strings.Contains(importPath, "mysql") || strings.Contains(importPath, "gin") ||
				strings.Contains(importPath, "redis") {
				t.Errorf("forbidden adapter dependency %q", importPath)
			}
		}
	}
}
