package main

import (
	"fmt"
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
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	repositoryRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repositoryRoot, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repositoryRoot, "testdata", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	repositoryInfo, err := os.Stat(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(rootInfo, repositoryInfo) {
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

func TestCompletedLessonsHaveLearningRecords(t *testing.T) {
	repositoryRoot, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := checkCompletedLessonLearningRecords(repositoryRoot); len(problems) > 0 {
		for _, problem := range problems {
			t.Error(problem)
		}
	}
}

func TestRequiresLearningRecords(t *testing.T) {
	for lesson := 1; lesson <= 12; lesson++ {
		if requiresLearningRecords(lesson) {
			t.Fatalf("lesson %d unexpectedly requires pre-rule learning records", lesson)
		}
	}
	if requiresLearningRecords(14) {
		t.Fatal("lesson 14 is an explicitly tracked historical backfill")
	}
	for _, lesson := range []int{13, 15, 16, 96, 101} {
		if !requiresLearningRecords(lesson) {
			t.Fatalf("lesson %d must require learning records", lesson)
		}
	}
}

func TestExpectedCoursePartBoundaries(t *testing.T) {
	testCases := []struct {
		lesson int
		part   int
	}{
		{lesson: 1, part: 1},
		{lesson: 8, part: 1},
		{lesson: 9, part: 2},
		{lesson: 16, part: 2},
		{lesson: 17, part: 3},
		{lesson: 24, part: 3},
		{lesson: 25, part: 4},
		{lesson: 37, part: 4},
		{lesson: 38, part: 5},
		{lesson: 45, part: 5},
		{lesson: 46, part: 6},
		{lesson: 53, part: 6},
		{lesson: 54, part: 7},
		{lesson: 61, part: 7},
		{lesson: 62, part: 8},
		{lesson: 69, part: 8},
		{lesson: 70, part: 9},
		{lesson: 77, part: 9},
		{lesson: 78, part: 10},
		{lesson: 85, part: 10},
		{lesson: 86, part: 11},
		{lesson: 93, part: 11},
		{lesson: 94, part: 12},
		{lesson: 101, part: 12},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("lesson_%d", testCase.lesson), func(t *testing.T) {
			part, ok := expectedCoursePart(testCase.lesson)
			if !ok {
				t.Fatalf("lesson %d has no registered part", testCase.lesson)
			}
			if part != testCase.part {
				t.Fatalf("lesson %d part = %d, want %d", testCase.lesson, part, testCase.part)
			}
		})
	}

	for _, lesson := range []int{0, 102} {
		if part, ok := expectedCoursePart(lesson); ok {
			t.Fatalf("out-of-range lesson %d unexpectedly belongs to part %d", lesson, part)
		}
	}
}

func TestCoursePartRangesAreContinuous(t *testing.T) {
	if problems := validateCoursePartRanges(); len(problems) > 0 {
		for _, problem := range problems {
			t.Error(problem)
		}
	}

	for lesson := 1; lesson <= courseLessonCount; lesson++ {
		matches := 0
		for _, partRange := range coursePartRanges {
			if lesson >= partRange.start && lesson <= partRange.end {
				matches++
			}
		}
		if matches != 1 {
			t.Errorf("lesson %d belongs to %d ranges; want exactly one", lesson, matches)
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
