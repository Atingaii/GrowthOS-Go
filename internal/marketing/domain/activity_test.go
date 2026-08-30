package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewActivityCreatesCanonicalGenerationZeroDraft(t *testing.T) {
	activity, err := NewActivity(41, "  夏日增长活动  ")
	if err != nil {
		t.Fatalf("NewActivity: %v", err)
	}
	if activity.ID() != 41 || activity.Name() != "夏日增长活动" {
		t.Fatalf("identity/name = %d/%q", activity.ID(), activity.Name())
	}
	if activity.Lifecycle() != ActivityLifecycleDraft ||
		activity.StateVersion() != 0 ||
		activity.ActivePublicationVersion() != 0 {
		t.Fatalf(
			"draft state = %q/%d/%d",
			activity.Lifecycle(),
			activity.StateVersion(),
			activity.ActivePublicationVersion(),
		)
	}
	if retiredAt, ok := activity.RetiredAt(); ok || !retiredAt.IsZero() {
		t.Fatalf("draft retired-at = %v/%v", retiredAt, ok)
	}
	if reference, ok := activity.RetirementReference(); ok || reference != "" {
		t.Fatalf("draft retirement reference = %q/%v", reference, ok)
	}
	if err := activity.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestNewActivityRejectsInvalidIdentityAndName(t *testing.T) {
	tests := []struct {
		name string
		id   ActivityID
		text string
		want error
	}{
		{name: "missing id", text: "campaign", want: ErrActivityIDInvalid},
		{name: "empty", id: 1, text: " \t ", want: ErrActivityNameInvalid},
		{name: "invalid utf8", id: 1, text: string([]byte{0xff}), want: ErrActivityNameInvalid},
		{name: "control", id: 1, text: "campaign\u0000", want: ErrActivityNameInvalid},
		{name: "too long", id: 1, text: strings.Repeat("界", MaxActivityNameRunes+1), want: ErrActivityNameInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			activity, err := NewActivity(test.id, test.text)
			if !errors.Is(err, test.want) || !errors.Is(err, ErrActivityInvalid) {
				t.Fatalf("NewActivity() error = %v, want %v and ErrActivityInvalid", err, test.want)
			}
			if activity != (Activity{}) {
				t.Fatalf("failure returned Activity %#v", activity)
			}
		})
	}

	boundary := strings.Repeat("界", MaxActivityNameRunes)
	activity, err := NewActivity(1, boundary)
	if err != nil || activity.Name().String() != boundary {
		t.Fatalf("boundary name = %q, %v", activity.Name(), err)
	}
}

func TestRestoreActivityAcceptsEveryLegalLifecycleShape(t *testing.T) {
	now := testMarketingInstant()
	retirement := mustTestEvidence(t, "retirement/ticket-8")
	tests := []struct {
		name       string
		lifecycle  ActivityLifecycle
		state      ActivityStateVersion
		active     ActivityPublicationVersion
		retiredAt  time.Time
		retirement EvidenceReference
	}{
		{name: "draft", lifecycle: ActivityLifecycleDraft},
		{name: "published", lifecycle: ActivityLifecyclePublished, state: 7, active: 7},
		{name: "retired", lifecycle: ActivityLifecycleRetired, state: 8, active: 7, retiredAt: now, retirement: retirement},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			activity, err := RestoreActivity(
				9,
				"Summer Growth",
				test.lifecycle,
				test.state,
				test.active,
				test.retiredAt,
				test.retirement,
			)
			if err != nil {
				t.Fatalf("RestoreActivity: %v", err)
			}
			if err := activity.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestRestoreActivityRejectsEveryMixedLifecycleShape(t *testing.T) {
	now := testMarketingInstant()
	retirement := mustTestEvidence(t, "retirement/ticket-8")
	tests := []struct {
		name       string
		id         ActivityID
		activity   ActivityName
		lifecycle  ActivityLifecycle
		state      ActivityStateVersion
		active     ActivityPublicationVersion
		retiredAt  time.Time
		retirement EvidenceReference
		want       error
	}{
		{name: "zero id", activity: "name", lifecycle: ActivityLifecycleDraft, want: ErrActivityIDInvalid},
		{name: "noncanonical name", id: 1, activity: " name ", lifecycle: ActivityLifecycleDraft, want: ErrActivityNameInvalid},
		{name: "unknown lifecycle", id: 1, activity: "name", lifecycle: "paused", want: ErrActivityLifecycleUnsupported},
		{name: "draft state", id: 1, activity: "name", lifecycle: ActivityLifecycleDraft, state: 1, want: ErrActivityStateVersionInvalid},
		{name: "draft active", id: 1, activity: "name", lifecycle: ActivityLifecycleDraft, active: 1, want: ErrActivityStateVersionInvalid},
		{name: "draft retired at", id: 1, activity: "name", lifecycle: ActivityLifecycleDraft, retiredAt: now, want: ErrActivityLifecycleUnsupported},
		{name: "draft retirement ref", id: 1, activity: "name", lifecycle: ActivityLifecycleDraft, retirement: retirement, want: ErrActivityLifecycleUnsupported},
		{name: "published no active", id: 1, activity: "name", lifecycle: ActivityLifecyclePublished, state: 1, want: ErrActivityLifecycleUnsupported},
		{name: "published generation mismatch", id: 1, activity: "name", lifecycle: ActivityLifecyclePublished, state: 2, active: 1, want: ErrActivityStateVersionInvalid},
		{name: "published retired at", id: 1, activity: "name", lifecycle: ActivityLifecyclePublished, state: 1, active: 1, retiredAt: now, want: ErrActivityLifecycleUnsupported},
		{name: "published retirement ref", id: 1, activity: "name", lifecycle: ActivityLifecyclePublished, state: 1, active: 1, retirement: retirement, want: ErrActivityLifecycleUnsupported},
		{name: "retired no active", id: 1, activity: "name", lifecycle: ActivityLifecycleRetired, state: 1, retiredAt: now, retirement: retirement, want: ErrActivityLifecycleUnsupported},
		{name: "retired generation mismatch", id: 1, activity: "name", lifecycle: ActivityLifecycleRetired, state: 4, active: 2, retiredAt: now, retirement: retirement, want: ErrActivityStateVersionInvalid},
		{name: "retired active overflow", id: 1, activity: "name", lifecycle: ActivityLifecycleRetired, state: ActivityStateVersion(^uint64(0)), active: maxActivityVersion, retiredAt: now, retirement: retirement, want: ErrActivityStateVersionInvalid},
		{name: "retired missing at", id: 1, activity: "name", lifecycle: ActivityLifecycleRetired, state: 2, active: 1, retirement: retirement, want: ErrActivityLifecycleUnsupported},
		{name: "retired missing ref", id: 1, activity: "name", lifecycle: ActivityLifecycleRetired, state: 2, active: 1, retiredAt: now, want: ErrActivityLifecycleUnsupported},
		{name: "retired nanos", id: 1, activity: "name", lifecycle: ActivityLifecycleRetired, state: 2, active: 1, retiredAt: now.Add(time.Nanosecond), retirement: retirement, want: ErrActivityLifecycleUnsupported},
		{name: "retired non UTC", id: 1, activity: "name", lifecycle: ActivityLifecycleRetired, state: 2, active: 1, retiredAt: now.In(time.FixedZone("CST", 8*60*60)), retirement: retirement, want: ErrActivityLifecycleUnsupported},
		{name: "retired invalid ref", id: 1, activity: "name", lifecycle: ActivityLifecycleRetired, state: 2, active: 1, retiredAt: now, retirement: "bad ref", want: ErrActivityLifecycleUnsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			activity, err := RestoreActivity(
				test.id,
				test.activity,
				test.lifecycle,
				test.state,
				test.active,
				test.retiredAt,
				test.retirement,
			)
			if !errors.Is(err, test.want) || !errors.Is(err, ErrActivityInvalid) {
				t.Fatalf("RestoreActivity() error = %v, want %v and ErrActivityInvalid", err, test.want)
			}
			if activity != (Activity{}) {
				t.Fatalf("failure returned Activity %#v", activity)
			}
		})
	}
}

func TestEvidenceReferenceUsesBoundedCanonicalASCIIGrammar(t *testing.T) {
	boundary := "a" + strings.Repeat("x", MaxEvidenceReferenceBytes-1)
	reference, err := NewEvidenceReference(boundary)
	if err != nil || reference.String() != boundary {
		t.Fatalf("boundary reference = %q, %v", reference, err)
	}
	valid := []string{"approval:42", "ticket/change-8", "A_b.c-d"}
	for _, value := range valid {
		if _, err := NewEvidenceReference(value); err != nil {
			t.Fatalf("NewEvidenceReference(%q): %v", value, err)
		}
	}
	invalid := []string{"", "/leading", ".leading", "contains space", "emoji-🙂", strings.Repeat("x", MaxEvidenceReferenceBytes+1)}
	for _, value := range invalid {
		reference, err := NewEvidenceReference(value)
		if !errors.Is(err, ErrEvidenceReferenceInvalid) || reference != "" {
			t.Fatalf("NewEvidenceReference(%q) = %q, %v", value, reference, err)
		}
	}
}
