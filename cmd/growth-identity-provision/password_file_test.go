package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadEnrollmentPasswordPreservesContentAndRemovesOneTransportEnding(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{name: "no ending", input: []byte("  GrowthOS-password-32  "), want: []byte("  GrowthOS-password-32  ")},
		{name: "line feed", input: []byte("GrowthOS-password-32\n"), want: []byte("GrowthOS-password-32")},
		{name: "crlf", input: []byte("GrowthOS-password-32\r\n"), want: []byte("GrowthOS-password-32")},
		{name: "lone carriage return", input: []byte("GrowthOS-password-32\r"), want: []byte("GrowthOS-password-32\r")},
		{name: "only one ending", input: []byte("GrowthOS-password-32\n\n"), want: []byte("GrowthOS-password-32\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writePasswordFixture(t, test.input)
			got, err := readEnrollmentPassword(path)
			if err != nil {
				t.Fatalf("readEnrollmentPassword() error = %v", err)
			}
			defer clear(got)
			if !bytes.Equal(got, test.want) {
				t.Fatalf("readEnrollmentPassword() did not preserve the reviewed transport semantics")
			}
			if cap(got) != len(got) {
				t.Fatalf("returned capacity = %d, want exact length %d", cap(got), len(got))
			}
		})
	}
}

func TestReadEnrollmentPasswordAcceptsMaximumPayloadWithCRLF(t *testing.T) {
	input := append(bytes.Repeat([]byte{'p'}, 512), '\r', '\n')
	password, err := readEnrollmentPassword(writePasswordFixture(t, input))
	if err != nil {
		t.Fatalf("read maximum password: %v", err)
	}
	defer clear(password)
	if len(password) != 512 || !bytes.Equal(password, input[:512]) {
		t.Fatal("maximum password payload changed")
	}
}

func TestReadEnrollmentPasswordRejectsUnsafeFilesAndSizes(t *testing.T) {
	directory := t.TempDir()
	regular := filepath.Join(directory, "regular")
	if err := os.WriteFile(regular, []byte("GrowthOS-password-32"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "symlink")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Fatal(err)
	}
	oversize := filepath.Join(directory, "oversize")
	if err := os.WriteFile(oversize, bytes.Repeat([]byte{'p'}, maximumEnrollmentPasswordFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	contentOversize := filepath.Join(directory, "content-oversize")
	if err := os.WriteFile(contentOversize, bytes.Repeat([]byte{'p'}, 513), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(directory, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	newlineOnly := filepath.Join(directory, "newline-only")
	if err := os.WriteFile(newlineOnly, []byte("\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"",
		filepath.Join(directory, "missing"),
		directory,
		symlink,
		empty,
		newlineOnly,
		oversize,
		contentOversize,
	} {
		if value, err := readEnrollmentPassword(path); !errors.Is(err, errEnrollmentPasswordFile) || value != nil {
			clear(value)
			t.Fatalf("readEnrollmentPassword(unsafe path) = %v, %v", value, err)
		}
	}
}

func TestReadEnrollmentPasswordRejectsFileReplacementBetweenChecks(t *testing.T) {
	directory := t.TempDir()
	checkedPath := filepath.Join(directory, "checked")
	openedPath := filepath.Join(directory, "opened")
	if err := os.WriteFile(checkedPath, []byte("GrowthOS-password-32"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(openedPath, []byte("attacker-password-32"), 0o600); err != nil {
		t.Fatal(err)
	}

	value, err := readEnrollmentPasswordWith(checkedPath, passwordFileDependencies{
		lstat: os.Lstat,
		open: func(string) (enrollmentPasswordFile, error) {
			return os.Open(openedPath)
		},
	})
	if !errors.Is(err, errEnrollmentPasswordFile) || value != nil {
		clear(value)
		t.Fatalf("replacement result = %v, %v", value, err)
	}
}

func TestReadEnrollmentPasswordRejectsReadCloseAndTypedNilFailures(t *testing.T) {
	path := writePasswordFixture(t, []byte("GrowthOS-password-32"))
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	private := errors.New("private-password-file-detail")

	tests := []struct {
		name string
		open func(string) (enrollmentPasswordFile, error)
	}{
		{name: "open", open: func(string) (enrollmentPasswordFile, error) { return nil, private }},
		{name: "typed nil", open: func(string) (enrollmentPasswordFile, error) {
			var file *os.File
			return file, nil
		}},
		{name: "stat", open: func(string) (enrollmentPasswordFile, error) {
			return &stubEnrollmentPasswordFile{info: info, statErr: private}, nil
		}},
		{name: "read", open: func(string) (enrollmentPasswordFile, error) {
			return &stubEnrollmentPasswordFile{info: info, readErr: private}, nil
		}},
		{name: "close", open: func(string) (enrollmentPasswordFile, error) {
			return &stubEnrollmentPasswordFile{info: info, reader: strings.NewReader("GrowthOS-password-32"), closeErr: private}, nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, readErr := readEnrollmentPasswordWith(path, passwordFileDependencies{
				lstat: func(string) (fs.FileInfo, error) { return info, nil },
				open:  test.open,
			})
			if !errors.Is(readErr, errEnrollmentPasswordFile) || value != nil {
				clear(value)
				t.Fatalf("failure result = %v, %v", value, readErr)
			}
			for _, rendered := range []string{
				fmt.Sprint(readErr),
				fmt.Sprintf("%#v", readErr),
				slog.AnyValue(readErr).Resolve().String(),
			} {
				if strings.Contains(rendered, path) || strings.Contains(rendered, private.Error()) {
					t.Fatalf("error rendering leaked private detail: %q", rendered)
				}
			}
		})
	}
}

func TestReadEnrollmentPasswordRejectsInvalidDependencies(t *testing.T) {
	for _, dependencies := range []passwordFileDependencies{
		{},
		{lstat: os.Lstat},
		{open: func(string) (enrollmentPasswordFile, error) { return nil, nil }},
	} {
		if value, err := readEnrollmentPasswordWith("ignored", dependencies); !errors.Is(err, errEnrollmentPasswordFile) || value != nil {
			clear(value)
			t.Fatalf("invalid dependencies result = %v, %v", value, err)
		}
	}
}

type stubEnrollmentPasswordFile struct {
	info     fs.FileInfo
	statErr  error
	reader   io.Reader
	readErr  error
	closeErr error
}

func (file *stubEnrollmentPasswordFile) Stat() (fs.FileInfo, error) {
	return file.info, file.statErr
}

func (file *stubEnrollmentPasswordFile) Read(buffer []byte) (int, error) {
	if file.readErr != nil {
		return 0, file.readErr
	}
	if file.reader == nil {
		return 0, io.EOF
	}
	return file.reader.Read(buffer)
}

func (file *stubEnrollmentPasswordFile) Close() error { return file.closeErr }

func writePasswordFixture(t *testing.T, value []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, value, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
