package main

import (
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
