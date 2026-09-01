package mysqlprovisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	drivermysql "github.com/go-sql-driver/mysql"
)

func TestErrorBoundariesExposeOnlyStableClasses(t *testing.T) {
	t.Parallel()
	const privateSentinel = "private-dsn-password-envelope-sentinel"
	privateCause := &drivermysql.MySQLError{
		Number:  1142,
		Message: "INSERT denied with " + privateSentinel,
	}
	provisionError := newError(ErrDependencyUnavailable, privateCause)

	if !errors.Is(provisionError, ErrDependencyUnavailable) {
		t.Fatal("error omitted its stable dependency class")
	}
	if errors.Is(provisionError, privateCause) || errors.Unwrap(provisionError) != nil {
		t.Fatal("private cause entered ordinary error traversal")
	}
	var typed *Error
	if !errors.As(provisionError, &typed) || typed.Cause() != privateCause {
		t.Fatal("explicit trusted cause inspection was not retained")
	}

	formatted := fmt.Sprintf("%v|%+v|%#v", provisionError, provisionError, provisionError)
	encoded, err := json.Marshal(provisionError)
	if err != nil {
		t.Fatal(err)
	}
	var logged bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logged, nil))
	logger.Error("provision failed", slog.Any("error", provisionError))

	for boundary, value := range map[string]string{
		"format": formatted,
		"json":   string(encoded),
		"slog":   logged.String(),
	} {
		if strings.Contains(value, privateSentinel) || strings.Contains(value, "INSERT denied") {
			t.Fatalf("%s leaked private cause: %s", boundary, value)
		}
		if !strings.Contains(value, ErrDependencyUnavailable.Error()) {
			t.Fatalf("%s omitted stable error class: %s", boundary, value)
		}
	}
}

func TestUnknownAndNilErrorsFailClosed(t *testing.T) {
	t.Parallel()
	const privateSentinel = "unknown-private-class"
	unknown := &Error{
		class: errors.New(privateSentinel),
		cause: errors.New("private cause"),
	}
	if got := fmt.Sprintf("%v %#v", unknown, unknown); got !=
		ErrDependencyUnavailable.Error()+" "+ErrDependencyUnavailable.Error() {
		t.Fatalf("unknown class formatting = %q", got)
	}
	if !errors.Is(unknown, ErrDependencyUnavailable) || errors.Is(unknown, unknown.class) {
		t.Fatal("unknown error class did not fail closed")
	}

	var nilError *Error
	if nilError.Error() != ErrDependencyUnavailable.Error() ||
		nilError.GoString() != ErrDependencyUnavailable.Error() ||
		nilError.Cause() != nil ||
		!errors.Is(nilError, ErrDependencyUnavailable) {
		t.Fatal("nil error receiver was not safe")
	}
}

func TestCancellationErrorsRetainStableAndContextClasses(t *testing.T) {
	t.Parallel()
	for _, check := range []struct {
		name         string
		cause        error
		contextClass error
	}{
		{name: "canceled", cause: context.Canceled, contextClass: context.Canceled},
		{name: "deadline", cause: context.DeadlineExceeded, contextClass: context.DeadlineExceeded},
		{name: "unclassified cause", cause: errors.New("private cancellation detail")},
	} {
		t.Run(check.name, func(t *testing.T) {
			t.Parallel()
			err := canceledError(check.cause)
			if !errors.Is(err, ErrOperationCanceled) {
				t.Fatal("cancellation omitted stable provisioner class")
			}
			if check.contextClass != nil && !errors.Is(err, check.contextClass) {
				t.Fatalf("cancellation omitted context class %v", check.contextClass)
			}
			if check.contextClass == nil &&
				(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
				t.Fatal("unclassified cause invented a context class")
			}
			if err.Error() != ErrOperationCanceled.Error() {
				t.Fatalf("rendered cancellation = %q", err)
			}
		})
	}
}

func TestProvisionerFormattingContainsNoDatabaseState(t *testing.T) {
	t.Parallel()
	var zero Provisioner
	formatted := fmt.Sprintf("%v|%+v|%#v", &zero, &zero, &zero)
	if formatted != redactedProvisioner+"|"+redactedProvisioner+"|"+redactedProvisioner {
		t.Fatalf("Provisioner formatting = %q", formatted)
	}
	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("adapter", slog.Any("value", &zero))
	if !strings.Contains(logged.String(), redactedProvisioner) {
		t.Fatalf("Provisioner structured log = %q", logged.String())
	}
}
