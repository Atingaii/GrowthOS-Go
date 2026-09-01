package httpapi

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductionImportsStayInsideIdentityHTTPBoundary(t *testing.T) {
	const modulePath = "github.com/Atingaii/GrowthOS-Go/"
	allowed := map[string]bool{
		modulePath + "internal/identity/application":   true,
		modulePath + "internal/identity/domain":        true,
		modulePath + "internal/infrastructure/httpapi": true,
		modulePath + "internal/platform/fault":         true,
	}
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
			if strings.HasPrefix(importPath, modulePath+"internal/") && !allowed[importPath] {
				t.Errorf("%s imports forbidden internal package %q", path, importPath)
			}
			for _, forbidden := range []string{"mysql", "redis", "governance", "config", "passwordhash"} {
				if strings.Contains(importPath, forbidden) {
					t.Errorf("%s imports forbidden boundary %q", path, importPath)
				}
			}
		}
	}
}

func TestProductionVocabularyContainsNoAuthorizationProjection(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Clean(entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(content))
		for _, forbiddenJSON := range []string{
			"json:\"role", "json:\"permission", "json:\"scope", "json:\"capability",
			"json:\"policy", "json:\"account_id", "json:\"session_token",
		} {
			if strings.Contains(lower, forbiddenJSON) {
				t.Errorf("%s contains forbidden response field vocabulary %q", entry.Name(), forbiddenJSON)
			}
		}
	}
}
