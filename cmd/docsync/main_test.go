package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectSkipsMetadata(t *testing.T) {
	repositoryRoot := t.TempDir()
	docsRoot := filepath.Join(repositoryRoot, "docs")
	for path, content := range map[string]string{
		"README.md": "docs readme", ".obsidian/app.json": "{}", ".growthos-sync/manifest.json": "{}", "nested/lesson.md": "lesson",
	} {
		full := filepath.Join(docsRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repositoryRoot, "README.md"), []byte("project readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := collect(syncSource{repositoryRoot: repositoryRoot, docsRoot: docsRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["README.md"] == "" || got["nested/lesson.md"] == "" || got[projectReadmeMirror] == "" {
		t.Fatalf("unexpected manifest: %#v", got)
	}
}

func TestMirrorDoesNotImportVaultEdits(t *testing.T) {
	repositoryRoot := t.TempDir()
	docsRoot := filepath.Join(repositoryRoot, "docs")
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
	write(repositoryRoot, "README.md", `<img src="docs/assets/hero.svg"> [文档](docs/README.md)`)
	write(docsRoot, "lesson.md", "source v1")
	sourceRoot := syncSource{repositoryRoot: repositoryRoot, docsRoot: docsRoot}
	source, err := collect(sourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := mirror(sourceRoot, targetRoot, source, nil); err != nil {
		t.Fatal(err)
	}
	write(targetRoot, "private.md", "personal note")
	write(targetRoot, "lesson.md", "vault annotation")
	write(docsRoot, "lesson.md", "source v2")
	write(repositoryRoot, "README.md", `<img src="docs/assets/hero-v2.svg"> [课程](docs/course/README.md)`)
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
	readme, err := os.ReadFile(filepath.Join(targetRoot, projectReadmeMirror))
	if err != nil {
		t.Fatal(err)
	}
	if string(readme) != `<img src="assets/hero-v2.svg"> [课程](course/README.md)` {
		t.Fatalf("project README was not mirrored from source: %q", readme)
	}
	if _, err := os.Stat(filepath.Join(docsRoot, "private.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private vault note was imported into repo: %v", err)
	}
}

func TestMirrorRejectsPathEscapingVault(t *testing.T) {
	repositoryRoot := t.TempDir()
	targetRoot := t.TempDir()
	outsidePath := filepath.Join(filepath.Dir(targetRoot), "growthos-outside-"+filepath.Base(targetRoot))
	if err := os.WriteFile(outsidePath, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outsidePath) })

	previous := manifest{"../" + filepath.Base(outsidePath): "untrusted"}
	err := mirror(syncSource{repositoryRoot: repositoryRoot}, targetRoot, manifest{}, previous)
	if err == nil || !strings.Contains(err.Error(), "离开 Vault") {
		t.Fatalf("mirror error = %v, want unsafe path rejection", err)
	}
	data, readErr := os.ReadFile(outsidePath)
	if readErr != nil || string(data) != "keep" {
		t.Fatalf("outside file was changed: data=%q err=%v", data, readErr)
	}
}

func TestMirrorRejectsVaultMetadataPath(t *testing.T) {
	targetRoot := t.TempDir()
	previous := manifest{stateDir + "/manifest.json": "untrusted"}
	err := mirror(syncSource{}, targetRoot, manifest{}, previous)
	if err == nil || !strings.Contains(err.Error(), "私有元数据") {
		t.Fatalf("mirror error = %v, want metadata path rejection", err)
	}
}

func TestMirrorRejectsSymlinkTraversal(t *testing.T) {
	targetRoot := t.TempDir()
	outsideRoot := t.TempDir()
	link := filepath.Join(targetRoot, "linked")
	if err := os.Symlink(outsideRoot, link); err != nil {
		t.Skipf("symlink is unavailable: %v", err)
	}

	previous := manifest{"linked/private.md": "untrusted"}
	err := mirror(syncSource{}, targetRoot, manifest{}, previous)
	if err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("mirror error = %v, want symlink rejection", err)
	}
}
