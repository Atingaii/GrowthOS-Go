package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectSkipsMetadata(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{
		"README.md": "readme", ".obsidian/app.json": "{}", ".growthos-sync/manifest.json": "{}", "nested/lesson.md": "lesson",
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := collect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["README.md"] == "" || got["nested/lesson.md"] == "" {
		t.Fatalf("unexpected manifest: %#v", got)
	}
}

func TestMirrorDoesNotImportVaultEdits(t *testing.T) {
	sourceRoot := t.TempDir()
	targetRoot := t.TempDir()
	write := func(root, name, content string) {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(sourceRoot, "lesson.md", "source v1")
	source, err := collect(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := mirror(sourceRoot, targetRoot, source, nil); err != nil {
		t.Fatal(err)
	}
	write(targetRoot, "private.md", "personal note")
	write(targetRoot, "lesson.md", "vault annotation")
	write(sourceRoot, "lesson.md", "source v2")
	updated, err := collect(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := mirror(sourceRoot, targetRoot, updated, source); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(targetRoot, "lesson.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "source v2" {
		t.Fatalf("lesson was not mirrored from source: %q", data)
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "private.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private vault note was imported into repo: %v", err)
	}
}
