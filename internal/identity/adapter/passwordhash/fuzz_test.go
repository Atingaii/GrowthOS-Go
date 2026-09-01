package passwordhash

import (
	"context"
	"errors"
	"testing"
	"time"
	"unicode/utf8"
)

func FuzzParseEnvelope(f *testing.F) {
	salt := phcBase64.EncodeToString(make([]byte, SaltBytes))
	output := phcBase64.EncodeToString(make([]byte, OutputBytes))
	f.Add("$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$" + output)
	f.Add("")
	f.Add("$argon2id$v=19$m=999999999,t=2,p=1$bad$bad")

	f.Fuzz(func(t *testing.T, encoded string) {
		envelope, err := ParseEnvelope(encoded)
		if err != nil {
			if envelope != (Envelope{}) || !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("ParseEnvelope(%q) = (%v, %v)", encoded, envelope, err)
			}
			return
		}
		if envelope.Encoded() != encoded {
			t.Fatalf("ParseEnvelope() did not preserve canonical input")
		}
		if _, err := parseEnvelope(envelope.Encoded()); err != nil {
			t.Fatalf("accepted envelope did not round trip: %v", err)
		}
	})
}

func FuzzPasswordBounds(f *testing.F) {
	f.Add([]byte("password123"), uint8(MinimumEnrollmentCodePoints))
	f.Add([]byte("密"), uint8(MinimumLoginCodePoints))
	f.Add([]byte{0xff}, uint8(MinimumLoginCodePoints))

	f.Fuzz(func(t *testing.T, password []byte, minimumSeed uint8) {
		minimum := MinimumLoginCodePoints
		if minimumSeed%2 == 0 {
			minimum = MinimumEnrollmentCodePoints
		}
		err := validatePassword(password, minimum)
		count := utf8.RuneCount(password)
		wantValid := utf8.Valid(password) &&
			len(password) <= MaximumPasswordBytes &&
			count >= minimum && count <= MaximumPasswordCodePoints
		if wantValid && err != nil {
			t.Fatalf("validatePassword(valid) error = %v", err)
		}
		if !wantValid && !errors.Is(err, ErrPasswordRejected) {
			t.Fatalf("validatePassword(invalid) error = %v", err)
		}
	})
}

func FuzzWorkGateCapacityAndCancellation(f *testing.F) {
	f.Add(uint8(1), uint8(1), true)
	f.Add(uint8(4), uint8(2), false)

	f.Fuzz(func(t *testing.T, capacitySeed, occupiedSeed uint8, cancelAttempt bool) {
		capacity := int(capacitySeed%MaximumMaxConcurrent) + 1
		occupied := int(occupiedSeed % uint8(capacity+1))
		gate := newWorkGate(capacity)
		for index := 0; index < occupied; index++ {
			if err := gate.acquire(context.Background(), time.Millisecond); err != nil {
				t.Fatalf("seed acquire %d/%d: %v", index, occupied, err)
			}
		}

		ctx := context.Background()
		cancel := func() {}
		if cancelAttempt {
			cancelable, cancelFunc := context.WithCancel(ctx)
			cancelFunc()
			ctx = cancelable
			cancel = cancelFunc
		}
		wait := time.Second
		if occupied == capacity {
			wait = MinimumAcquireTimeout
		}
		err := gate.acquire(ctx, wait)
		cancel()
		if cancelAttempt {
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled acquire error = %v", err)
			}
		} else if occupied == capacity {
			if !errors.Is(err, ErrHashingUnavailable) {
				t.Fatalf("full acquire error = %v", err)
			}
		} else if err != nil {
			t.Fatalf("available acquire error = %v", err)
		} else {
			gate.release()
		}
		for index := 0; index < occupied; index++ {
			gate.release()
		}
	})
}
