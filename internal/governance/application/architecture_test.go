package application

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGovernanceApplicationImportsOnlyOwnedKernelAndStandardLibrary(t *testing.T) {
	t.Parallel()

	allowed := map[string]struct{}{
		"context": {},
		"errors":  {},
		"fmt":     {},
		"reflect": {},
		"time":    {},
		"github.com/Atingaii/GrowthOS-Go/internal/governance/domain": {},
	}
	files := 0
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		files++
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if _, ok := allowed[importPath]; !ok {
				t.Errorf("%s imports dependency outside Governance application boundary %q", path, imported.Path.Value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect Governance application: %v", err)
	}
	if files == 0 {
		t.Fatal("Governance application guard parsed no production files")
	}
}
