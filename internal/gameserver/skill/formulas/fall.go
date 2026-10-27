package formulas

// FallDamageBase returns the fall damage before the fall-damage stat
// applies: the drop height times max HP, multiplied as 32-bit integers
// (so a large product wraps), over 1000.
func FallDamageBase(height, maxHP int) float64 {
	return float64(int32(height)*int32(maxHP)) / 1000
}
