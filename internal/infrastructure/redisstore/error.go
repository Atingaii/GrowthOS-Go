package redisstore

// Stage is a safe, low-cardinality Redis failure phase. Error rendering never
// includes credentials, addresses, keys, payloads, certificate paths, or raw
// client errors; trusted code can still inspect the cause with errors.Is/As.
type Stage string

const (
	StageConfigInvalid Stage = "redis_config_invalid"
	StageTLSRoots      Stage = "redis_tls_roots"
	StageTLSCA         Stage = "redis_tls_ca"
	StageGetRange      Stage = "redis_getrange"
	StageSet           Stage = "redis_set"
	StageDelete        Stage = "redis_delete"
	StageClose         Stage = "redis_close"
)

// Error retains a diagnostic cause without rendering it at ordinary logging,
// HTTP, or formatting boundaries.
type Error struct {
	stage Stage
	cause error
}

func newError(stage Stage, cause error) *Error {
	return &Error{stage: stage, cause: cause}
}

func (e *Error) Error() string {
	if e == nil || !knownStage(e.stage) {
		return string(StageConfigInvalid)
	}
	return string(e.stage)
}

// Unwrap keeps context cancellation, network, TLS, and Redis errors available
// to trusted policy code without exposing them in Error().
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Stage reports the stable operation phase.
func (e *Error) Stage() Stage {
	if e == nil || !knownStage(e.stage) {
		return StageConfigInvalid
	}
	return e.stage
}

func knownStage(stage Stage) bool {
	switch stage {
	case StageConfigInvalid, StageTLSRoots, StageTLSCA, StageGetRange, StageSet, StageDelete, StageClose:
		return true
	default:
		return false
	}
}
