// Command docsync mirrors the repository README and docs into a personal Obsidian vault.
// Vault edits are intentionally private and never flow back into the repo.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const stateDir = ".growthos-sync"
const projectReadmeMirror = "项目README.md"
const projectReadmeMirrorVersion = "v1"

type manifest map[string]string

type syncSource struct {
	repositoryRoot string
	docsRoot       string
}

func main() {
	vault := flag.String("vault", "", "Obsidian 目录路径")
	dryRun := flag.Bool("dry-run", false, "只显示计划，不写入文件")
	watch := flag.Bool("watch", false, "持续监听项目 README.md 和 docs/ 并自动同步")
	interval := flag.Duration("interval", 2*time.Second, "监听轮询间隔")
	flag.Parse()
	if strings.TrimSpace(*vault) == "" {
		fatal(errors.New("必须通过 --vault 指定 Obsidian 目录"))
	}
	if *interval <= 0 {
		fatal(errors.New("--interval 必须大于 0"))
	}
	source, err := repositorySource()
	if err != nil {
		fatal(err)
	}
	vaultPath, err := filepath.Abs(*vault)
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		fatal(fmt.Errorf("创建 Obsidian 目录失败: %w", err))
	}
	if err := syncOnce(source, vaultPath, *dryRun); err != nil {
		fatal(err)
	}
	if !*watch {
		return
	}
	fmt.Printf("开始监听项目 README.md 和 docs/，每 %s 同步一次；按 Ctrl+C 停止\n", interval.String())
	var previous manifest
	for {
		time.Sleep(*interval)
		current, err := collect(source)
		if err != nil {
			fmt.Fprintln(os.Stderr, "文档同步失败:", err)
			continue
		}
		if manifestsEqual(previous, current) {
			continue
		}
		if err := syncOnce(source, vaultPath, false); err != nil {
			fmt.Fprintln(os.Stderr, "文档同步失败:", err)
			continue
		}
		previous = current
	}
}

func syncOnce(sourceRoot syncSource, vaultPath string, dryRun bool) error {
	statePath := filepath.Join(vaultPath, stateDir, "manifest.json")
	source, err := collect(sourceRoot)
	if err != nil {
		return fmt.Errorf("读取项目文档失败: %w", err)
	}
	previous, err := readManifest(statePath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if dryRun {
		printPlan(source, previous)
		return nil
	}
	if err := mirror(sourceRoot, vaultPath, source, previous); err != nil {
		return err
	}
	if err := writeManifest(statePath, source); err != nil {
		return fmt.Errorf("写入同步状态失败: %w", err)
	}
	fmt.Printf("已同步项目 README.md + docs/ -> %s\n", vaultPath)
	return nil
}

func repositorySource() (syncSource, error) {
	dir, err := os.Getwd()
	if err != nil {
		return syncSource{}, err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return syncSource{repositoryRoot: dir, docsRoot: filepath.Join(dir, "docs")}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return syncSource{}, errors.New("找不到项目根目录")
		}
		dir = parent
	}
}

func collect(source syncSource) (manifest, error) {
	result := manifest{}
	err := filepath.WalkDir(source.docsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != source.docsRoot && (entry.Name() == ".obsidian" || entry.Name() == stateDir) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(source.docsRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, ".obsidian/") || strings.HasPrefix(rel, stateDir+"/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		result[rel] = hex.EncodeToString(digest[:])
		return nil
	})
	if err != nil {
		return nil, err
	}
	readmePath := filepath.Join(source.repositoryRoot, "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return nil, fmt.Errorf("读取根 README.md 失败: %w", err)
	}
	digest := sha256.Sum256(append(data, []byte("\x00"+projectReadmeMirrorVersion)...))
	result[projectReadmeMirror] = hex.EncodeToString(digest[:])
	return result, nil
}

func mirror(sourceRoot syncSource, targetRoot string, source, previous manifest) error {
	for _, path := range keys(source, previous) {
		sourceHash, sourceOK := source[path]
		previousHash, previousOK := previous[path]
		sourceChanged := !previousOK || sourceHash != previousHash
		if sourceOK && sourceChanged {
			var err error
			if path == projectReadmeMirror {
				err = copyProjectReadme(sourcePath(sourceRoot, path), filepath.Join(targetRoot, filepath.FromSlash(path)))
			} else {
				err = copyFile(sourcePath(sourceRoot, path), filepath.Join(targetRoot, filepath.FromSlash(path)))
			}
			if err != nil {
				return fmt.Errorf("同步文件失败 %s: %w", path, err)
			}
			continue
		}
		if !sourceOK && previousOK {
			if err := os.Remove(filepath.Join(targetRoot, filepath.FromSlash(path))); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("同步删除文件失败 %s: %w", path, err)
			}
		}
	}
	return nil
}

func sourcePath(source syncSource, path string) string {
	if path == projectReadmeMirror {
		return filepath.Join(source.repositoryRoot, "README.md")
	}
	return filepath.Join(source.docsRoot, filepath.FromSlash(path))
}

func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o644)
}

func copyProjectReadme(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	content := strings.NewReplacer(
		`src="docs/`, `src="`,
		`href="docs/`, `href="`,
		`](docs/`, `](`,
	).Replace(string(data))
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.WriteFile(to, []byte(content), 0o644)
}

func keys(groups ...manifest) []string {
	seen := map[string]bool{}
	for _, group := range groups {
		for path := range group {
			seen[path] = true
		}
	}
	result := make([]string, 0, len(seen))
	for path := range seen {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func manifestsEqual(left, right manifest) bool {
	if len(left) != len(right) {
		return false
	}
	for path, hash := range left {
		if right[path] != hash {
			return false
		}
	}
	return true
}

func readManifest(path string) (manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result manifest
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析同步状态失败: %w", err)
	}
	return result, nil
}

func writeManifest(path string, value manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func printPlan(source, previous manifest) {
	for _, path := range keys(source, previous) {
		if source[path] != previous[path] {
			fmt.Printf("将镜像: %s\n", path)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "文档同步失败:", err)
	os.Exit(1)
}
