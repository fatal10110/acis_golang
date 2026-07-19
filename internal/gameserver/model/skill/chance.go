package skill

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// TriggerType classifies the combat or cast event a chance-triggered skill
// reacts to.
type TriggerType uint8

const (
	TriggerOnAttacked TriggerType = iota
	TriggerOnAttackedHit
	TriggerOnCrit
	TriggerOnHit
	TriggerOnMagicGood
	TriggerOnMagicOffensive
)

var triggerTypeStrings = [...]string{
	"ON_ATTACKED", "ON_ATTACKED_HIT", "ON_CRIT", "ON_HIT", "ON_MAGIC_GOOD", "ON_MAGIC_OFFENSIVE",
}

var triggerTypeNames = commons.NameIndex[TriggerType](triggerTypeStrings[:])

// String returns t's canonical XML spelling.
func (t TriggerType) String() string {
	if int(t) < len(triggerTypeStrings) {
		return triggerTypeStrings[t]
	}
	return fmt.Sprintf("TriggerType(%d)", uint8(t))
}

// ChanceCondition is a chance-triggered skill's activation rule: it fires
// when its configured event occurs and, when Chance is set, wins a roll.
type ChanceCondition struct {
	Trigger TriggerType
	// Chance is the roll threshold out of 100; a negative value always wins
	// once Trigger matches.
	Chance int
}

// ParseChanceCondition resolves an effect template's chanceType and
// activationChance attributes into a ChanceCondition. An empty chanceType
// reports ok=false with a nil error, matching a skill that carries no
// chance-trigger condition at all; a non-empty value naming an unrecognized
// trigger type is an error rather than silently defaulting.
func ParseChanceCondition(chanceType string, chance int) (cond ChanceCondition, ok bool, err error) {
	if chanceType == "" {
		return ChanceCondition{}, false, nil
	}
	trigger, known := triggerTypeNames[chanceType]
	if !known {
		return ChanceCondition{}, false, fmt.Errorf("skill: unknown chance trigger type %q", chanceType)
	}
	return ChanceCondition{Trigger: trigger, Chance: chance}, true, nil
}

// Fires reports whether cond activates: trigger matches its configured
// event and, when Chance is set (non-negative), roll — expected in
// [0,100) — wins it.
func (cond ChanceCondition) Fires(trigger TriggerType, roll int) bool {
	return cond.Trigger == trigger && (cond.Chance < 0 || roll < cond.Chance)
}
