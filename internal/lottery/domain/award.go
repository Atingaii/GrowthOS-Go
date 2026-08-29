package domain

// AwardID identifies one selectable outcome inside a lottery strategy.
type AwardID uint64

// Weight is a positive, relative selection weight. It is not a percentage:
// 1:3 and 100:300 express the same distribution.
type Weight uint64

// AwardOutcome distinguishes a deliverable reward from an intentional miss.
// It does not describe the reward's concrete Benefit type or delivery state.
type AwardOutcome string

const (
	// AwardOutcomeReward means selecting the award creates a reward description
	// for a later Benefit delivery flow.
	AwardOutcomeReward AwardOutcome = "reward"
	// AwardOutcomeNoReward means the draw completed successfully without a
	// reward. A miss is data, not an error or an absent result.
	AwardOutcomeNoReward AwardOutcome = "no_reward"
)

// Award is one immutable, weighted candidate in a Strategy.
type Award struct {
	id      AwardID
	name    string
	weight  Weight
	outcome AwardOutcome
}

// NewAward constructs an Award only when all local invariants hold.
func NewAward(id AwardID, name string, weight Weight, outcome AwardOutcome) (Award, error) {
	if id == 0 {
		return Award{}, ErrAwardIDRequired
	}

	name, err := normalizeName(
		name,
		MaxAwardNameRunes,
		ErrAwardNameRequired,
		ErrAwardNameInvalid,
		ErrAwardNameTooLong,
	)
	if err != nil {
		return Award{}, err
	}

	award := Award{
		id:      id,
		name:    name,
		weight:  weight,
		outcome: outcome,
	}
	if err := award.validate(); err != nil {
		return Award{}, err
	}
	return award, nil
}

// ID returns the award's durable identity.
func (a Award) ID() AwardID { return a.id }

// Name returns the user-facing outcome name.
func (a Award) Name() string { return a.name }

// Weight returns the award's relative selection weight.
func (a Award) Weight() Weight { return a.weight }

// Outcome returns whether the selection describes a reward or an intentional miss.
func (a Award) Outcome() AwardOutcome { return a.outcome }

// HasReward reports whether a later Benefit flow has a reward to deliver.
func (a Award) HasReward() bool { return a.outcome == AwardOutcomeReward }

func (o AwardOutcome) valid() bool {
	return o == AwardOutcomeReward || o == AwardOutcomeNoReward
}

func (a Award) validate() error {
	if a.id == 0 {
		return ErrAwardIDRequired
	}
	if err := validateCanonicalName(
		a.name,
		MaxAwardNameRunes,
		ErrAwardNameRequired,
		ErrAwardNameInvalid,
		ErrAwardNameTooLong,
	); err != nil {
		return err
	}
	if a.weight == 0 {
		return ErrAwardWeightRequired
	}
	if !a.outcome.valid() {
		return ErrAwardOutcomeInvalid
	}
	return nil
}
