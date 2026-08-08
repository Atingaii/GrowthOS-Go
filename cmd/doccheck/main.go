// Command doccheck validates documentation links, ADR registration, and course evidence.
package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var markdownLink = regexp.MustCompile(`!?\[[^]]*\]\(([^)]+)\)`)

func main() {
	root, err := findRepositoryRoot()
	if err != nil {
		fail([]error{err})
	}

	var problems []error
	problems = append(problems, checkRequiredFiles(root)...)
	problems = append(problems, checkMarkdownLinks(root)...)
	problems = append(problems, checkADRIndex(root)...)
	problems = append(problems, checkCourseStatus(root)...)
	if len(problems) > 0 {
		fail(problems)
	}

	fmt.Println("documentation checks passed")
}

func findRepositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root with go.mod not found")
		}
		dir = parent
	}
}

func checkRequiredFiles(root string) []error {
	required := []string{
		"README.md",
		"docs/README.md",
		"docs/architecture/repository-map.md",
		"docs/course/README.md",
		"docs/course/status.csv",
		"docs/decisions/README.md",
		"docs/product/product-brief.md",
		"docs/qa/README.md",
		"docs/standards/definition-of-done.md",
		"docs/standards/documentation-governance.md",
	}
	var problems []error
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			problems = append(problems, fmt.Errorf("required file %s: %w", name, err))
		}
	}
	return problems
}

func checkMarkdownLinks(root string) []error {
	var problems []error
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range markdownLink.FindAllStringSubmatch(string(content), -1) {
			target := strings.TrimSpace(match[1])
			if target == "" || strings.HasPrefix(target, "#") || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			if index := strings.IndexByte(target, '#'); index >= 0 {
				target = target[:index]
			}
			target = strings.Trim(target, "<>")
			if target == "" {
				continue
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(target)))
			if _, err := os.Stat(resolved); err != nil {
				rel, _ := filepath.Rel(root, path)
				problems = append(problems, fmt.Errorf("%s links to missing %s", filepath.ToSlash(rel), target))
			}
		}
		return nil
	})
	if err != nil {
		problems = append(problems, err)
	}
	return problems
}

func checkADRIndex(root string) []error {
	dir := filepath.Join(root, "docs", "decisions")
	index, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		return []error{err}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return []error{err}
	}
	var problems []error
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") || !strings.HasPrefix(name, "ADR-") {
			continue
		}
		if !strings.Contains(string(index), "("+name+")") {
			problems = append(problems, fmt.Errorf("ADR %s is not registered in docs/decisions/README.md", name))
		}
	}
	return problems
}

func checkCourseStatus(root string) []error {
	path := filepath.Join(root, "docs", "course", "status.csv")
	file, err := os.Open(path)
	if err != nil {
		return []error{err}
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReader(file))
	records, err := reader.ReadAll()
	if err != nil {
		return []error{fmt.Errorf("parse docs/course/status.csv: %w", err)}
	}
	if len(records) != 97 {
		return []error{fmt.Errorf("course registry must contain one header and 96 lessons; got %d rows", len(records))}
	}
	expectedHeader := []string{"lesson", "part", "title", "status", "document", "qa"}
	if len(records[0]) != len(expectedHeader) {
		return []error{fmt.Errorf("course registry header must have %d columns", len(expectedHeader))}
	}
	for i, value := range expectedHeader {
		if records[0][i] != value {
			return []error{fmt.Errorf("course registry column %d must be %q", i+1, value)}
		}
	}

	allowedStatus := map[string]bool{"planned": true, "in_progress": true, "completed": true}
	var problems []error
	for i, record := range records[1:] {
		row := i + 2
		if len(record) != len(expectedHeader) {
			problems = append(problems, fmt.Errorf("course registry row %d has %d columns", row, len(record)))
			continue
		}
		lesson, err := strconv.Atoi(record[0])
		if err != nil || lesson != i+1 {
			problems = append(problems, fmt.Errorf("course registry row %d must be lesson %d", row, i+1))
		}
		expectedPart := expectedPartForLesson(i + 1)
		part, err := strconv.Atoi(record[1])
		if err != nil || part != expectedPart {
			problems = append(problems, fmt.Errorf("lesson %d must belong to part %d", lesson, expectedPart))
		}
		status := record[3]
		if !allowedStatus[status] {
			problems = append(problems, fmt.Errorf("lesson %d has invalid status %q", lesson, status))
		}
		if status != "completed" {
			continue
		}
		if record[4] == "" || record[5] == "" {
			problems = append(problems, fmt.Errorf("completed lesson %d requires document and QA evidence", lesson))
			continue
		}
		for _, registeredPath := range record[4:6] {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(registeredPath))); err != nil {
				problems = append(problems, fmt.Errorf("completed lesson %d references missing %s", lesson, registeredPath))
			}
		}
	}
	return problems
}

func expectedPartForLesson(lesson int) int {
	if lesson <= 80 {
		return ((lesson - 1) / 8) + 1
	}
	if lesson <= 90 {
		return 11
	}
	return 12
}

func fail(problems []error) {
	sort.Slice(problems, func(i, j int) bool { return problems[i].Error() < problems[j].Error() })
	fmt.Fprintln(os.Stderr, "documentation checks failed:")
	for _, problem := range problems {
		fmt.Fprintf(os.Stderr, "- %v\n", problem)
	}
	os.Exit(1)
}
