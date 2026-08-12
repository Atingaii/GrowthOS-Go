package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindRepositoryRoot(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repositoryRoot, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	nested := filepath.Join(repositoryRoot, "testdata", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(repositoryRoot, "testdata")) })
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root != repositoryRoot {
		t.Fatalf("root = %q, want %q", root, repositoryRoot)
	}
}

func TestCompletedLessonsHaveAPIRecords(t *testing.T) {
	repositoryRoot, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := checkCompletedLessonAPIRecords(repositoryRoot); len(problems) > 0 {
		for _, problem := range problems {
			t.Error(problem)
		}
	}
}

func TestCheckMarkdownLinksSkipsCcb(t *testing.T) {
	root := t.TempDir()

	normal := filepath.Join(root, "docs")
	if err := os.MkdirAll(normal, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(normal, "a.md"), []byte("[missing](not-there.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ccb := filepath.Join(root, ".ccb", "agents", "example", "skills")
	if err := os.MkdirAll(ccb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ccb, "SKILL.md"), []byte("[broken](../../../../missing.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	problems := checkMarkdownLinks(root)
	var normalReported, ccbReported bool
	for _, problem := range problems {
		if strings.Contains(problem.Error(), "a.md") {
			normalReported = true
		}
		if strings.Contains(problem.Error(), "SKILL.md") {
			ccbReported = true
		}
	}
	if !normalReported {
		t.Errorf(".ccb 之外的断链 a.md 未被报告：%v", problems)
	}
	if ccbReported {
		t.Errorf(".ccb 下的断链 SKILL.md 不应被报告：%v", problems)
	}
}
