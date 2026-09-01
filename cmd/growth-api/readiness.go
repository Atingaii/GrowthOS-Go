package main

import (
	"context"
	"errors"
	"reflect"
	"time"

	sharedhttp "github.com/Atingaii/GrowthOS-Go/internal/infrastructure/httpapi"
)

var (
	errReadinessConfiguration = errors.New("dual mysql readiness is not configured")
	errReadinessUnavailable   = errors.New("dual mysql readiness is unavailable")
)

// dualMySQLReadiness probes both authoritative stores without owning either
// pool. Per-pool deadlines prevent one dependency from consuming the other's
// budget; the HTTP adapter owns the outer max-duration deadline.
type dualMySQLReadiness struct {
	business        sharedhttp.ReadinessChecker
	identity        sharedhttp.ReadinessChecker
	businessTimeout time.Duration
	identityTimeout time.Duration
}

func newDualMySQLReadiness(
	business sharedhttp.ReadinessChecker,
	identity sharedhttp.ReadinessChecker,
	businessTimeout time.Duration,
	identityTimeout time.Duration,
) (*dualMySQLReadiness, error) {
	readiness := &dualMySQLReadiness{
		business:        business,
		identity:        identity,
		businessTimeout: businessTimeout,
		identityTimeout: identityTimeout,
	}
	if readiness.Validate() != nil {
		return nil, errReadinessConfiguration
	}
	return readiness, nil
}

func (readiness *dualMySQLReadiness) Validate() error {
	if readiness == nil || nilReadinessDependency(readiness.business) ||
		nilReadinessDependency(readiness.identity) ||
		readiness.businessTimeout <= 0 || readiness.identityTimeout <= 0 {
		return errReadinessConfiguration
	}
	return nil
}

func (readiness *dualMySQLReadiness) PingContext(ctx context.Context) error {
	if readiness.Validate() != nil || ctx == nil || ctx.Err() != nil {
		return errReadinessUnavailable
	}
	probeContext, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan bool, 2)
	probe := func(checker sharedhttp.ReadinessChecker, timeout time.Duration) {
		dependencyContext, dependencyCancel := context.WithTimeout(probeContext, timeout)
		err := checker.PingContext(dependencyContext)
		failed := err != nil || dependencyContext.Err() != nil
		dependencyCancel()
		results <- failed
	}
	go probe(readiness.business, readiness.businessTimeout)
	go probe(readiness.identity, readiness.identityTimeout)

	failed := false
	for completed := 0; completed < 2; completed++ {
		if <-results {
			failed = true
			// Stop useful work in the sibling, but still wait for it to return so
			// this method never leaves an unowned probe goroutine behind.
			cancel()
		}
	}
	if failed || ctx.Err() != nil {
		return errReadinessUnavailable
	}
	return nil
}

func dualMySQLReadinessTimeout(business, identity time.Duration) time.Duration {
	if identity > business {
		return identity
	}
	return business
}

func nilReadinessDependency(checker sharedhttp.ReadinessChecker) bool {
	if checker == nil {
		return true
	}
	value := reflect.ValueOf(checker)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
