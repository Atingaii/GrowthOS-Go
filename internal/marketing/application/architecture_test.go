package application

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const lesson30ModulePrefix = "github.com/Atingaii/GrowthOS-Go/"

func TestLesson30ApplicationAndLotteryACLKeepBoundedContextImports(t *testing.T) {
	checks := []struct {
		name    string
		root    string
		allowed map[string]struct{}
	}{
		{
			name: "Marketing application",
			root: ".",
			allowed: map[string]struct{}{
				lesson30ModulePrefix + "internal/marketing/domain": {},
			},
		},
		{
			name: "Lottery ACL",
			root: filepath.Join("..", "adapter", "lotteryconfig"),
			allowed: map[string]struct{}{
				lesson30ModulePrefix + "internal/marketing/application": {},
				lesson30ModulePrefix + "internal/lottery/application":   {},
				lesson30ModulePrefix + "internal/lottery/domain":        {},
			},
		},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			violations, files, err := lesson30ImportViolations(check.root, check.allowed)
			if err != nil {
				t.Fatalf("inspect %s: %v", check.root, err)
			}
			for _, violation := range violations {
				t.Error(violation)
			}
			if files == 0 {
				t.Fatalf("no production Go files found in %s", check.root)
			}
		})
	}
}

func TestLesson30ServicesRemainOutsideRuntimeComposition(t *testing.T) {
	forbidden := map[string]struct{}{
		"NewCreateDraftService":      {},
		"NewPublishActivityService":  {},
		"NewRollbackActivityService": {},
		"NewRetireActivityService":   {},
		"NewResolveActivityService":  {},
	}
	root := filepath.Join("..", "..", "..")
	applicationRoot := filepath.Clean(filepath.Join(root, "internal", "marketing", "application")) + string(filepath.Separator)
	aclRoot := filepath.Clean(filepath.Join(root, "internal", "marketing", "adapter", "lotteryconfig")) + string(filepath.Separator)
	var violations []string
	parsed := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		clean := filepath.Clean(path)
		if strings.HasPrefix(clean, applicationRoot) || strings.HasPrefix(clean, aclRoot) {
			return nil
		}
		parsed++
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if _, blocked := forbidden[identifier.Name]; blocked {
				violations = append(violations, fmt.Sprintf(
					"%s prematurely composes unprotected Lesson 30 service %s",
					path,
					identifier.Name,
				))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("inspect runtime composition: %v", err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
	if parsed == 0 {
		t.Fatal("runtime architecture guard parsed no production Go files")
	}
}

func lesson30ImportViolations(
	root string,
	allowed map[string]struct{},
) ([]string, int, error) {
	var violations []string
	parsed := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		parsed++
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote %s import: %w", path, err)
			}
			if !strings.HasPrefix(importPath, lesson30ModulePrefix) {
				continue
			}
			if _, ok := allowed[importPath]; !ok {
				violations = append(violations, fmt.Sprintf(
					"%s imports forbidden project package %q",
					path,
					importPath,
				))
			}
		}
		return nil
	})
	return violations, parsed, err
}
