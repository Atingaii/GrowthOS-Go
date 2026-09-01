package passwordhash

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func TestHashEnrollmentAndVerifyLoginRoundTrip(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher(t, bytes.NewReader([]byte("0123456789abcdef")), 2, time.Second)
	password := []byte("  GrowthOS密码🙂!  ")
	callerCopy := bytes.Clone(password)

	envelope, err := hasher.HashEnrollment(context.Background(), password)
	if err != nil {
		t.Fatalf("HashEnrollment() error = %v", err)
	}
	if envelope.Encoded() == "" || len(envelope.Encoded()) > MaximumEnvelopeBytes {
		t.Fatalf("HashEnrollment() encoded length = %d", len(envelope.Encoded()))
	}
	if !strings.HasPrefix(envelope.Encoded(), "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("HashEnrollment() prefix is not the current profile")
	}
	parts := strings.Split(envelope.Encoded(), "$")
	if len(parts) != 6 || strings.Contains(parts[4], "=") || strings.Contains(parts[5], "=") {
		t.Fatalf("HashEnrollment() used a non-canonical base64 field")
	}

	verification, err := hasher.VerifyLogin(context.Background(), password, envelope.Encoded())
	if err != nil {
		t.Fatalf("VerifyLogin() error = %v", err)
	}
	if !verification.Matched() || verification.NeedsRehash() {
		t.Fatalf("VerifyLogin() = %+v, want matched current profile", verification)
	}

	mismatch, err := hasher.VerifyLogin(
		context.Background(),
		bytes.TrimSpace(password),
		envelope.Encoded(),
	)
	if err != nil {
		t.Fatalf("VerifyLogin(trimmed) error = %v", err)
	}
	if mismatch != (Verification{}) {
		t.Fatalf("VerifyLogin(trimmed) = %+v, want zero mismatch", mismatch)
	}
	if !bytes.Equal(password, callerCopy) {
		t.Fatalf("password adapter modified the caller-owned input")
	}

	parsed, err := ParseEnvelope(envelope.Encoded())
	if err != nil {
		t.Fatalf("ParseEnvelope() error = %v", err)
	}
	if parsed.Encoded() != envelope.Encoded() {
		t.Fatalf("ParseEnvelope().Encoded() did not preserve canonical envelope")
	}
}

func TestPasswordBounds(t *testing.T) {
	t.Parallel()

	invalidUTF8 := string([]byte{0xff})
	cases := []struct {
		name      string
		password  []byte
		minimum   int
		wantError bool
	}{
		{name: "login one", password: []byte("密"), minimum: MinimumLoginCodePoints},
		{name: "login empty", password: []byte{}, minimum: MinimumLoginCodePoints, wantError: true},
		{name: "enrollment twelve", password: []byte(strings.Repeat(" ", 12)), minimum: MinimumEnrollmentCodePoints},
		{name: "enrollment eleven", password: []byte(strings.Repeat("a", 11)), minimum: MinimumEnrollmentCodePoints, wantError: true},
		{name: "maximum code points and bytes", password: []byte(strings.Repeat("🙂", 128)), minimum: MinimumEnrollmentCodePoints},
		{name: "too many code points", password: []byte(strings.Repeat("a", 129)), minimum: MinimumLoginCodePoints, wantError: true},
		{name: "invalid utf8", password: []byte(invalidUTF8), minimum: MinimumLoginCodePoints, wantError: true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validatePassword(test.password, test.minimum)
			if test.wantError && !errors.Is(err, ErrPasswordRejected) {
				t.Fatalf("validatePassword() error = %v, want ErrPasswordRejected", err)
			}
			if !test.wantError && err != nil {
				t.Fatalf("validatePassword() error = %v", err)
			}
		})
	}

	maximum := []byte(strings.Repeat("🙂", MaximumPasswordCodePoints))
	if got := len(maximum); got != MaximumPasswordBytes {
		t.Fatalf("maximum UTF-8 fixture bytes = %d, want %d", got, MaximumPasswordBytes)
	}
	if got := utf8.RuneCount(maximum); got != MaximumPasswordCodePoints {
		t.Fatalf("maximum UTF-8 fixture code points = %d", got)
	}
}

func TestLegacyProfileMatchesAndRequiresRehash(t *testing.T) {
	t.Parallel()

	params := parameters{memory: MinimumLegacyMemoryKiB, iterations: 1, parallelism: 1}
	password := []byte("legacy login password")
	salt := []byte("legacy-salt-0001")
	output := derive(password, salt, params)
	encoded, err := encodeEnvelope(params, salt, output)
	zero(password)
	zero(output)
	if err != nil {
		t.Fatalf("encodeEnvelope() error = %v", err)
	}

	hasher := newTestHasher(t, bytes.NewReader(make([]byte, SaltBytes)), 2, time.Second)
	verification, err := hasher.VerifyLogin(context.Background(), []byte("legacy login password"), encoded)
	if err != nil {
		t.Fatalf("VerifyLogin() error = %v", err)
	}
	if !verification.Matched() || !verification.NeedsRehash() {
		t.Fatalf("VerifyLogin() = %+v, want matched legacy profile", verification)
	}
}

func TestMalformedEnvelopeIsRejectedBeforeArgonAdmission(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher(t, bytes.NewReader(make([]byte, SaltBytes)), 1, MinimumAcquireTimeout)
	if err := hasher.gate.acquire(context.Background(), time.Second); err != nil {
		t.Fatalf("occupy gate error = %v", err)
	}
	verification, err := hasher.VerifyLogin(
		context.Background(),
		[]byte("valid login password"),
		"$argon2id$v=19$m=999999999,t=2,p=1$bad$bad",
	)
	hasher.gate.release()
	if verification != (Verification{}) {
		t.Fatalf("VerifyLogin(malformed) returned non-zero verification")
	}
	if !errors.Is(err, ErrInvalidEnvelope) || errors.Is(err, ErrHashingUnavailable) {
		t.Fatalf("VerifyLogin(malformed) error = %v", err)
	}
}

func TestEnrollmentSerializesInjectedEntropyReader(t *testing.T) {
	reader := &observedEntropyReader{}
	hasher := newTestHasher(t, reader, 2, time.Second)
	start := make(chan struct{})
	errorsChannel := make(chan error, 2)
	for index := 0; index < 2; index++ {
		go func() {
			<-start
			_, err := hasher.HashEnrollment(context.Background(), []byte("concurrent password"))
			errorsChannel <- err
		}()
	}
	close(start)
	for index := 0; index < 2; index++ {
		if err := <-errorsChannel; err != nil {
			t.Fatalf("HashEnrollment(concurrent %d) error = %v", index, err)
		}
	}
	if got := reader.maximum.Load(); got != 1 {
		t.Fatalf("injected entropy concurrent reads = %d, want 1", got)
	}
}

func TestEnrollmentUsesFreshInjectedSalt(t *testing.T) {
	t.Parallel()

	entropy := append(
		[]byte("0123456789abcdef"),
		[]byte("fedcba9876543210")...,
	)
	hasher := newTestHasher(t, bytes.NewReader(entropy), 2, time.Second)
	password := []byte("same enrollment password")
	first, err := hasher.HashEnrollment(context.Background(), password)
	if err != nil {
		t.Fatalf("first HashEnrollment() error = %v", err)
	}
	second, err := hasher.HashEnrollment(context.Background(), password)
	if err != nil {
		t.Fatalf("second HashEnrollment() error = %v", err)
	}
	if first.Encoded() == second.Encoded() {
		t.Fatalf("two enrollment hashes reused a salt")
	}
}

func TestEnrollmentRejectsAllZeroEntropy(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher(t, bytes.NewReader(make([]byte, SaltBytes)), 1, time.Second)
	envelope, err := hasher.HashEnrollment(
		context.Background(),
		[]byte("valid enrollment password"),
	)
	if envelope != (Envelope{}) || !errors.Is(err, ErrEntropyUnavailable) {
		t.Fatalf("HashEnrollment(all-zero entropy) = %v, %v", envelope, err)
	}
}

func TestUnknownLoginUsesBoundedDummyPath(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher(t, bytes.NewReader(make([]byte, SaltBytes)), 1, 5*time.Millisecond)
	if !hasher.dummy.parameters.current() {
		t.Fatalf("dummy envelope parameters = %+v, want current", hasher.dummy.parameters)
	}

	if err := hasher.gate.acquire(context.Background(), time.Second); err != nil {
		t.Fatalf("occupy dummy gate: %v", err)
	}
	err := hasher.VerifyUnknownLogin(context.Background(), []byte("unknown account password"))
	hasher.gate.release()
	if !errors.Is(err, ErrHashingUnavailable) {
		t.Fatalf("VerifyUnknownLogin(blocked) error = %v, want unavailable", err)
	}

	if err := hasher.VerifyUnknownLogin(context.Background(), []byte("unknown account password")); err != nil {
		t.Fatalf("VerifyUnknownLogin() error = %v", err)
	}
	if err := hasher.VerifyUnknownLogin(
		context.Background(),
		[]byte("GrowthOS fixed dummy credential v1"),
	); err != nil {
		t.Fatalf("VerifyUnknownLogin(dummy match) error = %v", err)
	}
}

func TestCancellationAndEntropyFailuresAreSafe(t *testing.T) {
	t.Parallel()

	hasher := newTestHasher(t, errorReader{err: errors.New("entropy-reader-secret")}, 1, time.Second)
	_, err := hasher.HashEnrollment(context.Background(), []byte("valid password"))
	if !errors.Is(err, ErrEntropyUnavailable) {
		t.Fatalf("HashEnrollment() error = %v, want ErrEntropyUnavailable", err)
	}
	if strings.Contains(err.Error(), "entropy-reader-secret") {
		t.Fatalf("HashEnrollment() exposed the entropy reader error")
	}
	shortHasher := newTestHasher(t, bytes.NewReader(make([]byte, SaltBytes-1)), 1, time.Second)
	if _, err := shortHasher.HashEnrollment(context.Background(), []byte("valid password")); !errors.Is(err, ErrEntropyUnavailable) {
		t.Fatalf("HashEnrollment(short entropy) error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := hasher.HashEnrollment(ctx, []byte("valid password"))
	if result != (Envelope{}) {
		t.Fatalf("HashEnrollment(canceled) returned non-zero envelope")
	}
	if !errors.Is(err, ErrHashingUnavailable) || !errors.Is(err, context.Canceled) {
		t.Fatalf("HashEnrollment(canceled) error = %v", err)
	}
	if err.Error() != ErrHashingUnavailable.Error() {
		t.Fatalf("HashEnrollment(canceled) error text = %q", err.Error())
	}

	if _, err := hasher.HashEnrollment(nil, []byte("valid password")); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("HashEnrollment(nil context) error = %v", err)
	}
}

func TestConfigurationAndFormattingAreRedacted(t *testing.T) {
	defaultHasher, err := New(Config{})
	if err != nil {
		t.Fatalf("New(default) error = %v", err)
	}
	secondHasher, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New(DefaultConfig()) error = %v", err)
	}
	if defaultHasher.gate != secondHasher.gate {
		t.Fatalf("New() did not share the process gate")
	}

	for maximum := MinimumMaxConcurrent; maximum <= MaximumMaxConcurrent; maximum++ {
		if _, err := normalizeConfig(Config{MaxConcurrent: maximum}); err != nil {
			t.Fatalf("normalizeConfig(max=%d) error = %v", maximum, err)
		}
	}
	for _, maximum := range []int{-1, MaximumMaxConcurrent + 1} {
		if _, err := normalizeConfig(Config{MaxConcurrent: maximum}); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("normalizeConfig(max=%d) error = %v", maximum, err)
		}
	}
	for _, wait := range []time.Duration{
		-time.Nanosecond,
		MinimumAcquireTimeout - time.Nanosecond,
		MaximumAcquireTimeout + time.Nanosecond,
	} {
		if _, err := normalizeConfig(Config{AcquireTimeout: wait}); !errors.Is(err, ErrInvalidConfiguration) {
			t.Fatalf("normalizeConfig(wait=%s) error = %v", wait, err)
		}
	}
	for _, wait := range []time.Duration{0, MinimumAcquireTimeout, MaximumAcquireTimeout} {
		if _, err := normalizeConfig(Config{AcquireTimeout: wait}); err != nil {
			t.Fatalf("normalizeConfig(wait=%s) error = %v", wait, err)
		}
	}

	var typedNil *bytes.Reader
	if _, err := normalizeConfig(Config{Entropy: typedNil}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("normalizeConfig(typed nil) error = %v", err)
	}

	secret := "entropy-reader-secret"
	config := Config{
		MaxConcurrent:  DefaultMaxConcurrent,
		AcquireTimeout: DefaultAcquireTimeout,
		Entropy:        namedReader{name: secret},
	}
	if strings.Contains(fmt.Sprint(config), secret) || strings.Contains(fmt.Sprintf("%#v", config), secret) {
		t.Fatalf("Config formatting exposed entropy")
	}

	envelope := Envelope{encoded: "$argon2id$credential-secret"}
	verification := Verification{matched: true, needsRehash: true}
	if envelope.String() != "passwordhash.Envelope{[REDACTED]}" {
		t.Fatalf("Envelope.String() = %q", envelope.String())
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	logger.Info("redaction", "config", config, "envelope", envelope, "verification", verification, "hasher", defaultHasher)
	for _, sensitive := range []string{secret, envelope.encoded, "matched:true", "needsRehash:true"} {
		if strings.Contains(logs.String(), sensitive) {
			t.Fatalf("structured log exposed %q: %s", sensitive, logs.String())
		}
	}
}

func TestProcessGateRejectsConflictingCapacity(t *testing.T) {
	first, err := New(Config{})
	if err != nil {
		t.Fatalf("New(default) error = %v", err)
	}
	second, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New(same capacity) error = %v", err)
	}
	if first.gate != second.gate {
		t.Fatalf("same-capacity constructors did not share the process gate")
	}
	for index := 0; index < DefaultMaxConcurrent; index++ {
		if err := first.gate.acquire(context.Background(), time.Second); err != nil {
			t.Fatalf("occupy shared process gate %d: %v", index, err)
		}
	}
	second.acquireTimeout = MinimumAcquireTimeout
	if err := second.VerifyUnknownLogin(context.Background(), []byte("shared process budget")); !errors.Is(err, ErrHashingUnavailable) {
		t.Fatalf("second Hasher bypassed shared process capacity: %v", err)
	}
	for index := 0; index < DefaultMaxConcurrent; index++ {
		first.gate.release()
	}
	if _, err := New(Config{MaxConcurrent: MaximumMaxConcurrent}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("New(conflicting capacity) error = %v", err)
	}
}

func newTestHasher(
	t *testing.T,
	entropy io.Reader,
	capacity int,
	wait time.Duration,
) *Hasher {
	t.Helper()
	dummy, err := fixedDummyEnvelope()
	if err != nil {
		t.Fatalf("fixedDummyEnvelope() error = %v", err)
	}
	return &Hasher{
		gate:           newWorkGate(capacity),
		acquireTimeout: wait,
		entropy:        entropy,
		dummy:          dummy,
	}
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

type namedReader struct {
	name string
}

func (r namedReader) Read(buffer []byte) (int, error) { return len(buffer), nil }

func (r namedReader) String() string { return r.name }

type observedEntropyReader struct {
	active  atomic.Int32
	maximum atomic.Int32
	calls   atomic.Uint32
}

func (r *observedEntropyReader) Read(buffer []byte) (int, error) {
	active := r.active.Add(1)
	for {
		maximum := r.maximum.Load()
		if active <= maximum || r.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond)
	fill := byte(r.calls.Add(1))
	for index := range buffer {
		buffer[index] = fill
	}
	r.active.Add(-1)
	return len(buffer), nil
}
