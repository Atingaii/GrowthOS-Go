// Command docsync synchronizes repository docs with an external vault.
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
)

const stateDir = ".growthos-sync"

type manifest map[string]string

func main() {
	vault := flag.String("vault", "", "Obsidian 目录路径")
	dryRun := flag.Bool("dry-run", false, "只显示计划，不写入文件")
	flag.Parse()
	if strings.TrimSpace(*vault) == "" {
		fatal(errors.New("必须通过 --vault 指定 Obsidian 目录"))
	}
	repoDocs, err := repositoryDocs()
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
	basePath := filepath.Join(vaultPath, stateDir, "manifest.json")
	base, err := readManifest(basePath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fatal(err)
	}
	source, err := collect(repoDocs)
	if err != nil {
		fatal(fmt.Errorf("读取项目 docs 失败: %w", err))
	}
	target, err := collect(vaultPath)
	if err != nil {
		fatal(fmt.Errorf("读取 Obsidian 目录失败: %w", err))
	}
	if base == nil {
		if len(target) > 0 {
			fatal(errors.New("首次同步前目标目录已有文档；请先清理冲突，或手动整理成同一基线"))
		}
		if *dryRun {
			fmt.Printf("首次同步：将项目 docs/ 写入 %s\n", vaultPath)
			return
		}
		if err := apply(repoDocs, vaultPath, source, target, nil); err != nil {
			fatal(err)
		}
		if err := writeManifest(basePath, source); err != nil {
			fatal(err)
		}
		fmt.Printf("首次同步完成：项目 docs/ -> %s\n", vaultPath)
		return
	}
	union := keys(source, target, base)
	var conflicts []string
	for _, path := range union {
		sourceHash, sourceOK := source[path]
		targetHash, targetOK := target[path]
		baseHash, baseOK := base[path]
		sourceChanged := sourceOK != baseOK || sourceHash != baseHash
		targetChanged := targetOK != baseOK || targetHash != baseHash
		if sourceChanged && targetChanged && (sourceOK != targetOK || sourceHash != targetHash) {
			conflicts = append(conflicts, path)
		}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		fatal(fmt.Errorf("检测到双向冲突，请人工处理后重试: %s", strings.Join(conflicts, ", ")))
	}
	if *dryRun {
		printPlan(source, target, base)
		return
	}
	if err := apply(repoDocs, vaultPath, source, target, base); err != nil {
		fatal(err)
	}
	updated, err := collect(repoDocs)
	if err != nil {
		fatal(fmt.Errorf("同步后读取项目 docs 失败: %w", err))
	}
	if err := writeManifest(basePath, updated); err != nil {
		fatal(err)
	}
	fmt.Printf("双向同步完成：%s <-> %s\n", repoDocs, vaultPath)
}

func repositoryDocs() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "docs"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("找不到项目根目录")
		}
		dir = parent
	}
}

func collect(root string) (manifest, error) {
	result := manifest{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".obsidian" || entry.Name() == stateDir) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
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
	return result, err
}

func apply(sourceRoot, targetRoot string, source, target, base manifest) error {
	for _, path := range keys(source, target, nil) {
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(path))
		targetPath := filepath.Join(targetRoot, filepath.FromSlash(path))
		sourceHash, sourceOK := source[path]
		targetHash, targetOK := target[path]
		if sourceOK && (!targetOK || sourceHash != targetHash) {
			if !targetOK && base[path] != "" {
				if err := os.Remove(sourcePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("同步删除项目文件失败 %s: %w", path, err)
				}
				continue
			}
			if err := copyFile(sourcePath, targetPath); err != nil {
				return fmt.Errorf("同步到 Obsidian 失败 %s: %w", path, err)
			}
		}
		if !sourceOK && targetOK {
			if base[path] != "" {
				if err := os.Remove(targetPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("同步删除 Obsidian 文件失败 %s: %w", path, err)
				}
				continue
			}
			if err := copyFile(targetPath, sourcePath); err != nil {
				return fmt.Errorf("同步回项目失败 %s: %w", path, err)
			}
		}
	}
	return nil
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

func readManifest(path string) (manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result manifest
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("解析同步基线失败: %w", err)
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

func printPlan(source, target, base manifest) {
	for _, path := range keys(source, target, base) {
		if source[path] != target[path] {
			fmt.Printf("将同步: %s\n", path)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "文档同步失败:", err)
	os.Exit(1)
}
