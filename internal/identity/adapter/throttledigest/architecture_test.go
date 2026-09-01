package throttledigest

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductionImportsStayInsideThrottleDigestBoundary(t *testing.T) {
	const modulePath = "github.com/Atingaii/GrowthOS-Go/"
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Clean(entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(importPath, modulePath+"internal/") &&
				importPath != modulePath+"internal/identity/domain" {
				t.Errorf("%s imports forbidden internal package %q", path, importPath)
			}
			if strings.Contains(importPath, "mysql") || strings.Contains(importPath, "gin") ||
				strings.Contains(importPath, "redis") {
				t.Errorf("%s imports forbidden adapter dependency %q", path, importPath)
			}
		}
	}
}
