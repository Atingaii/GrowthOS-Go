// Package mysqlstore owns the application's MySQL connection boundary.
package mysqlstore

// Stage is a stable, non-sensitive failure identifier.
type Stage string

const (
	StageConfigInvalid Stage = "mysql_config_invalid"
	StageTLSRoots      Stage = "mysql_tls_roots"
	StageTLSCA         Stage = "mysql_tls_ca"
	StageConnector     Stage = "mysql_connector"
	StagePing          Stage = "mysql_ping"
	StageClose         Stage = "mysql_close"
)

// Error deliberately renders only a stable stage code. The wrapped cause is
// retained for errors.Is/errors.As; callers must not serialize it to clients or
// log it without a separate privacy review.
type Error struct {
	stage Stage
	cause error
}

func newError(stage Stage, cause error) *Error {
	return &Error{stage: stage, cause: cause}
}

func (e *Error) Error() string {
	if e == nil || e.stage == "" {
		return string(StageConfigInvalid)
	}
	return string(e.stage)
}

// Stage returns the safe failure stage.
func (e *Error) Stage() Stage {
	if e == nil || e.stage == "" {
		return StageConfigInvalid
	}
	return e.stage
}

// Unwrap preserves programmatic cause inspection without including it in
// Error().
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
