package strategycache

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

func TestProjectionRoundTripPreservesCanonicalUint64ValuesAndOrder(t *testing.T) {
	strategy := mustStrategy(t, domain.StrategyID(math.MaxUint64), "最大策略", []awardInput{
		{id: 9, name: "未中奖", weight: 1, outcome: domain.AwardOutcomeNoReward},
		{id: domain.AwardID(math.MaxUint64), name: "最大奖励", weight: domain.Weight(math.MaxUint64 - 1), outcome: domain.AwardOutcomeReward},
	})

	encoded, err := encodeProjection(strategy)
	if err != nil {
		t.Fatalf("encodeProjection() error = %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"id":"18446744073709551615"`)) ||
		!bytes.Contains(encoded, []byte(`"weight":"18446744073709551614"`)) {
		t.Fatalf("encoded projection did not preserve uint64 decimal strings: %s", encoded)
	}
	firstAwardMarker := []byte(`"awards":[{"id":"9"`)
	firstAward := bytes.Index(encoded, firstAwardMarker)
	secondAward := -1
	if firstAward >= 0 {
		relative := bytes.Index(encoded[firstAward+len(firstAwardMarker):], []byte(`"id":"18446744073709551615"`))
		if relative >= 0 {
			secondAward = firstAward + len(firstAwardMarker) + relative
		}
	}
	if firstAward < 0 || secondAward < 0 || firstAward >= secondAward {
		t.Fatalf("encoded awards are not in canonical AwardID order: %s", encoded)
	}

	decoded, err := decodeProjection(encoded)
	if err != nil {
		t.Fatalf("decodeProjection() error = %v", err)
	}
	assertSameStrategy(t, decoded, strategy)

	reencoded, err := encodeProjection(decoded)
	if err != nil {
		t.Fatalf("encodeProjection(decoded) error = %v", err)
	}
	if !bytes.Equal(reencoded, encoded) {
		t.Fatalf("projection encoding is not deterministic\nfirst:  %s\nsecond: %s", encoded, reencoded)
	}
}

func TestDecodeProjectionRejectsNonCanonicalOrUntrustedJSON(t *testing.T) {
	valid := `{"schema":"growthos.lottery.strategy.projection","schema_version":1,"strategy":{"id":"42","name":"Strategy","awards":[{"id":"7","name":"Award","weight":"3","outcome":"reward"}]}}`
	tests := map[string]string{
		"unknown root field":         strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"extra":true`, 1),
		"unknown nested field":       strings.Replace(valid, `"name":"Strategy"`, `"name":"Strategy","extra":true`, 1),
		"duplicate root field":       strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
		"duplicate award field":      strings.Replace(valid, `"weight":"3"`, `"weight":"3","weight":"3"`, 1),
		"case alias root field":      strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"Schema":"growthos.lottery.strategy.projection"`, 1),
		"case alias nested field":    strings.Replace(valid, `"name":"Strategy"`, `"name":"Strategy","Name":"Strategy"`, 1),
		"case alias award field":     strings.Replace(valid, `"outcome":"reward"`, `"outcome":"reward","Outcome":"reward"`, 1),
		"missing root field":         strings.Replace(valid, `"schema_version":1,`, ``, 1),
		"missing award field":        strings.Replace(valid, `,"outcome":"reward"`, ``, 1),
		"trailing document":          valid + `{}`,
		"wrong schema":               strings.Replace(valid, projectionSchema, "other", 1),
		"wrong schema version":       strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1),
		"numeric strategy id":        strings.Replace(valid, `"id":"42"`, `"id":42`, 1),
		"leading-zero strategy id":   strings.Replace(valid, `"id":"42"`, `"id":"042"`, 1),
		"zero strategy id":           strings.Replace(valid, `"id":"42"`, `"id":"0"`, 1),
		"uint64 overflow":            strings.Replace(valid, `"id":"42"`, `"id":"18446744073709551616"`, 1),
		"zero award id":              strings.Replace(valid, `"id":"7"`, `"id":"0"`, 1),
		"zero weight":                strings.Replace(valid, `"weight":"3"`, `"weight":"0"`, 1),
		"invalid outcome":            strings.Replace(valid, `"outcome":"reward"`, `"outcome":"fallback"`, 1),
		"noncanonical strategy name": strings.Replace(valid, `"name":"Strategy"`, `"name":" Strategy"`, 1),
		"empty awards":               strings.Replace(valid, `[{"id":"7","name":"Award","weight":"3","outcome":"reward"}]`, `[]`, 1),
		"null awards":                strings.Replace(valid, `[{"id":"7","name":"Award","weight":"3","outcome":"reward"}]`, `null`, 1),
		"non-object root":            `[]`,
		"malformed":                  `{`,
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			strategy, err := decodeProjection([]byte(value))
			if err == nil {
				t.Fatalf("decodeProjection() strategy = %#v, error = nil", strategy)
			}
			if strategy.ID() != 0 || strategy.Name() != "" || len(strategy.Awards()) != 0 || strategy.TotalWeight() != 0 {
				t.Fatalf("decodeProjection() returned nonzero Strategy on failure: %#v", strategy)
			}
		})
	}
}

func TestDecodeProjectionRejectsDuplicateAwardsOverflowAndExcessCount(t *testing.T) {
	tests := map[string]projectionV1{
		"duplicate award identity": projectionForTest("42", []projectionAwardV1{
			{ID: "1", Name: "One", Weight: "1", Outcome: "reward"},
			{ID: "1", Name: "Again", Weight: "1", Outcome: "no_reward"},
		}),
		"total weight overflow": projectionForTest("42", []projectionAwardV1{
			{ID: "1", Name: "One", Weight: strconv.FormatUint(math.MaxUint64, 10), Outcome: "reward"},
			{ID: "2", Name: "Two", Weight: "1", Outcome: "no_reward"},
		}),
	}

	tooMany := make([]projectionAwardV1, 0, domain.MaxAwardsPerStrategy+1)
	for index := 1; index <= domain.MaxAwardsPerStrategy+1; index++ {
		tooMany = append(tooMany, projectionAwardV1{
			ID:      strconv.Itoa(index),
			Name:    "Award",
			Weight:  "1",
			Outcome: "reward",
		})
	}
	tests["too many awards"] = projectionForTest("42", tooMany)

	for name, projection := range tests {
		t.Run(name, func(t *testing.T) {
			value, err := json.Marshal(projection)
			if err != nil {
				t.Fatalf("marshal test projection: %v", err)
			}
			if _, err := decodeProjection(value); err == nil {
				t.Fatal("decodeProjection() error = nil")
			}
		})
	}
}

func TestProjectionCodecEnforcesMaximumBytes(t *testing.T) {
	oversized := bytes.Repeat([]byte{' '}, int(MaximumProjectionBytes)+1)
	if _, err := decodeProjection(oversized); err == nil {
		t.Fatal("decodeProjection(oversized) error = nil")
	}
	if encoded, err := encodeProjection(domain.Strategy{}); err == nil || encoded != nil {
		t.Fatalf("encodeProjection(zero) = (%q, %v), want nil error result", encoded, err)
	}
}

func TestProjectionCodecAcceptsMaximumAwardCount(t *testing.T) {
	inputs := make([]awardInput, 0, domain.MaxAwardsPerStrategy)
	for index := 1; index <= domain.MaxAwardsPerStrategy; index++ {
		inputs = append(inputs, awardInput{
			id:      domain.AwardID(index),
			name:    "Award " + strconv.Itoa(index),
			weight:  1,
			outcome: domain.AwardOutcomeReward,
		})
	}
	strategy := mustStrategy(t, 42, "Maximum award count", inputs)
	value, err := encodeProjection(strategy)
	if err != nil {
		t.Fatalf("encodeProjection(max awards) error = %v", err)
	}
	if int64(len(value)) > MaximumProjectionBytes {
		t.Fatalf("encoded value bytes = %d, limit = %d", len(value), MaximumProjectionBytes)
	}
	decoded, err := decodeProjection(value)
	if err != nil {
		t.Fatalf("decodeProjection(max awards) error = %v", err)
	}
	assertSameStrategy(t, decoded, strategy)
}

func projectionForTest(strategyID string, awards []projectionAwardV1) projectionV1 {
	return projectionV1{
		Schema:        projectionSchema,
		SchemaVersion: projectionSchemaVersion,
		Strategy: projectionStrategyV1{
			ID:     strategyID,
			Name:   "Strategy",
			Awards: awards,
		},
	}
}
