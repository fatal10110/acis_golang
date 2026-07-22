package formulas

// LethalOutcome is the lethal-strike tier that landed.
type LethalOutcome uint8

const (
	LethalNone LethalOutcome = iota
	LethalHalf
	LethalFull
)

// LethalInput is a lethal-strike roll's already-resolved state.
type LethalInput struct {
	Chance1 int
	Chance2 int

	MagicLevel    int
	AttackerLevel int
	TargetLevel   int
	LethalMul     float64
}

// LethalRate returns the per-mille lethal chance for baseChance.
func LethalRate(in LethalInput, baseChance int) float64 {
	if baseChance <= 0 {
		return 0
	}
	attackerLevel := positiveLevel(in.AttackerLevel)
	targetLevel := positiveLevel(in.TargetLevel)
	baseRate := float64(baseChance)

	var editedRate float64
	if in.MagicLevel > 0 {
		delta := ((in.MagicLevel + attackerLevel) / 2) - 1 - targetLevel
		switch {
		case delta >= -3:
			editedRate = baseRate * float64(attackerLevel) / float64(targetLevel)
		case delta >= -9:
			editedRate = baseRate / float64(delta) * -3
		default:
			editedRate = baseRate / 15
		}
	} else {
		editedRate = baseRate * float64(attackerLevel) / float64(targetLevel)
	}

	return editedRate * 10 * in.LethalMul
}

// LethalHit resolves a full/half lethal strike, rolling the full tier first.
func LethalHit(in LethalInput, roll func(int) int) LethalOutcome {
	if roll == nil {
		return LethalNone
	}
	if in.Chance2 > 0 && lethalSucceeds(in, in.Chance2, roll(1000)) {
		return LethalFull
	}
	if in.Chance1 > 0 && lethalSucceeds(in, in.Chance1, roll(1000)) {
		return LethalHalf
	}
	return LethalNone
}

func lethalSucceeds(in LethalInput, baseChance, roll int) bool {
	return LethalRate(in, baseChance) > float64(roll)
}

func positiveLevel(level int) int {
	if level <= 0 {
		return 1
	}
	return level
}
