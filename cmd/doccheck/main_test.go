package main

import (
	"os"
	"path/filepath"
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

func TestExpectedPartForLesson(t *testing.T) {
	tests := map[int]int{
		1:  1,
		8:  1,
		9:  2,
		80: 10,
		81: 11,
		90: 11,
		91: 12,
		96: 12,
	}
	for lesson, want := range tests {
		if got := expectedPartForLesson(lesson); got != want {
			t.Errorf("expectedPartForLesson(%d) = %d, want %d", lesson, got, want)
		}
	}
}
