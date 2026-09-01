package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"reflect"

	"github.com/Atingaii/GrowthOS-Go/internal/identity/adapter/passwordhash"
)

var errEnrollmentPasswordFile = errors.New("identity provision password file rejected")

const maximumEnrollmentPasswordFileBytes = passwordhash.MaximumPasswordBytes + 2

type enrollmentPasswordFile interface {
	io.Reader
	Stat() (fs.FileInfo, error)
	Close() error
}

type passwordFileDependencies struct {
	lstat func(string) (fs.FileInfo, error)
	open  func(string) (enrollmentPasswordFile, error)
}

func readEnrollmentPassword(path string) ([]byte, error) {
	return readEnrollmentPasswordWith(path, passwordFileDependencies{
		lstat: os.Lstat,
		open: func(path string) (enrollmentPasswordFile, error) {
			return os.Open(path)
		},
	})
}

// readEnrollmentPasswordWith returns one caller-owned byte slice. The caller
// must clear it immediately after HashEnrollment returns. Every rejected path
// clears bytes already read and exposes only a stable, path-free error.
func readEnrollmentPasswordWith(
	path string,
	dependencies passwordFileDependencies,
) ([]byte, error) {
	if path == "" || dependencies.lstat == nil || dependencies.open == nil {
		return nil, errEnrollmentPasswordFile
	}
	before, err := dependencies.lstat(path)
	if err != nil || before == nil || !before.Mode().IsRegular() {
		return nil, errEnrollmentPasswordFile
	}

	file, err := dependencies.open(path)
	if err != nil || nilEnrollmentPasswordFile(file) {
		return nil, errEnrollmentPasswordFile
	}
	after, statErr := file.Stat()
	if statErr != nil || after == nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, errEnrollmentPasswordFile
	}

	value, readErr := io.ReadAll(io.LimitReader(file, maximumEnrollmentPasswordFileBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(value) == 0 || len(value) > maximumEnrollmentPasswordFileBytes {
		clear(value)
		return nil, errEnrollmentPasswordFile
	}

	contentLength := len(value)
	if contentLength >= 2 && value[contentLength-2] == '\r' && value[contentLength-1] == '\n' {
		contentLength -= 2
	} else if value[contentLength-1] == '\n' {
		contentLength--
	}
	if contentLength == 0 || contentLength > passwordhash.MaximumPasswordBytes {
		clear(value)
		return nil, errEnrollmentPasswordFile
	}
	clear(value[contentLength:])
	return value[:contentLength:contentLength], nil
}

func nilEnrollmentPasswordFile(file enrollmentPasswordFile) bool {
	if file == nil {
		return true
	}
	value := reflect.ValueOf(file)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
