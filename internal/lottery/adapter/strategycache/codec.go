package strategycache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/Atingaii/GrowthOS-Go/internal/lottery/domain"
)

const (
	projectionSchema        = "growthos.lottery.strategy.projection"
	projectionSchemaVersion = 1
	maximumJSONDepth        = 16
)

var ErrProjectionInvalid = errors.New("lottery strategy cache: projection is invalid")

type projectionV1 struct {
	Schema        string               `json:"schema"`
	SchemaVersion int                  `json:"schema_version"`
	Strategy      projectionStrategyV1 `json:"strategy"`
}

type projectionStrategyV1 struct {
	ID     string              `json:"id"`
	Name   string              `json:"name"`
	Awards []projectionAwardV1 `json:"awards"`
}

type projectionAwardV1 struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Weight  string `json:"weight"`
	Outcome string `json:"outcome"`
}

func encodeProjection(strategy domain.Strategy) ([]byte, error) {
	validated, err := domain.RestoreStrategy(strategy.ID(), strategy.Name(), strategy.Awards())
	if err != nil {
		return nil, ErrProjectionInvalid
	}

	awards := validated.Awards()
	projectionAwards := make([]projectionAwardV1, 0, len(awards))
	for _, award := range awards {
		projectionAwards = append(projectionAwards, projectionAwardV1{
			ID:      strconv.FormatUint(uint64(award.ID()), 10),
			Name:    award.Name(),
			Weight:  strconv.FormatUint(uint64(award.Weight()), 10),
			Outcome: string(award.Outcome()),
		})
	}

	value, err := json.Marshal(projectionV1{
		Schema:        projectionSchema,
		SchemaVersion: projectionSchemaVersion,
		Strategy: projectionStrategyV1{
			ID:     strconv.FormatUint(uint64(validated.ID()), 10),
			Name:   validated.Name(),
			Awards: projectionAwards,
		},
	})
	if err != nil || len(value) == 0 || int64(len(value)) > MaximumProjectionBytes {
		return nil, ErrProjectionInvalid
	}
	return value, nil
}

func decodeProjection(value []byte) (domain.Strategy, error) {
	if len(value) == 0 || int64(len(value)) > MaximumProjectionBytes {
		return domain.Strategy{}, ErrProjectionInvalid
	}
	if err := rejectDuplicateJSONNames(value); err != nil {
		return domain.Strategy{}, ErrProjectionInvalid
	}
	if err := validateProjectionJSONShape(value); err != nil {
		return domain.Strategy{}, ErrProjectionInvalid
	}

	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var projection projectionV1
	if err := decoder.Decode(&projection); err != nil {
		return domain.Strategy{}, ErrProjectionInvalid
	}
	if err := requireJSONEOF(decoder); err != nil {
		return domain.Strategy{}, ErrProjectionInvalid
	}
	if projection.Schema != projectionSchema || projection.SchemaVersion != projectionSchemaVersion {
		return domain.Strategy{}, ErrProjectionInvalid
	}

	strategyID, err := parseCanonicalUint64(projection.Strategy.ID)
	if err != nil || strategyID == 0 {
		return domain.Strategy{}, ErrProjectionInvalid
	}
	if len(projection.Strategy.Awards) == 0 || len(projection.Strategy.Awards) > domain.MaxAwardsPerStrategy {
		return domain.Strategy{}, ErrProjectionInvalid
	}

	awards := make([]domain.Award, 0, len(projection.Strategy.Awards))
	for _, cachedAward := range projection.Strategy.Awards {
		awardID, idErr := parseCanonicalUint64(cachedAward.ID)
		weight, weightErr := parseCanonicalUint64(cachedAward.Weight)
		if idErr != nil || weightErr != nil || awardID == 0 || weight == 0 {
			return domain.Strategy{}, ErrProjectionInvalid
		}
		award, restoreErr := domain.RestoreAward(
			domain.AwardID(awardID),
			cachedAward.Name,
			domain.Weight(weight),
			domain.AwardOutcome(cachedAward.Outcome),
		)
		if restoreErr != nil {
			return domain.Strategy{}, ErrProjectionInvalid
		}
		awards = append(awards, award)
	}

	strategy, err := domain.RestoreStrategy(
		domain.StrategyID(strategyID),
		projection.Strategy.Name,
		awards,
	)
	if err != nil {
		return domain.Strategy{}, ErrProjectionInvalid
	}
	return strategy, nil
}

// validateProjectionJSONShape makes field names case-sensitive and requires
// every v1 field exactly once. encoding/json intentionally matches struct
// fields case-insensitively, which is useful for ordinary APIs but too loose
// for a versioned cache format shared across rolling releases.
func validateProjectionJSONShape(value []byte) error {
	root, err := exactJSONObject(value, "schema", "schema_version", "strategy")
	if err != nil {
		return err
	}
	strategy, err := exactJSONObject(root["strategy"], "id", "name", "awards")
	if err != nil {
		return err
	}

	var awards []json.RawMessage
	if err := json.Unmarshal(strategy["awards"], &awards); err != nil {
		return ErrProjectionInvalid
	}
	for _, award := range awards {
		if _, err := exactJSONObject(award, "id", "name", "weight", "outcome"); err != nil {
			return err
		}
	}
	return nil
}

func exactJSONObject(value []byte, expectedNames ...string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil || len(object) != len(expectedNames) {
		return nil, ErrProjectionInvalid
	}
	for _, name := range expectedNames {
		if _, found := object[name]; !found {
			return nil, ErrProjectionInvalid
		}
	}
	return object, nil
}

func parseCanonicalUint64(value string) (uint64, error) {
	if value == "" || len(value) > 20 {
		return 0, ErrProjectionInvalid
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || strconv.FormatUint(parsed, 10) != value {
		return 0, ErrProjectionInvalid
	}
	return parsed, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrProjectionInvalid
	}
	return nil
}

func rejectDuplicateJSONNames(value []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := consumeJSONValue(decoder, first, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return ErrProjectionInvalid
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder, first json.Token, depth int) error {
	if depth > maximumJSONDepth {
		return ErrProjectionInvalid
	}
	delimiter, isDelimiter := first.(json.Delim)
	if !isDelimiter {
		return nil
	}

	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := nameToken.(string)
			if !ok {
				return ErrProjectionInvalid
			}
			if _, duplicate := seen[name]; duplicate {
				return fmt.Errorf("%w: duplicate object name", ErrProjectionInvalid)
			}
			seen[name] = struct{}{}

			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := consumeJSONValue(decoder, valueToken, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return ErrProjectionInvalid
		}
		return nil
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := consumeJSONValue(decoder, valueToken, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return ErrProjectionInvalid
		}
		return nil
	default:
		return ErrProjectionInvalid
	}
}
