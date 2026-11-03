package npc

// FestivalMonster reports whether h is one of the Festival of Darkness's
// monsters.
func (h *Hostile) FestivalMonster() bool {
	return hostileKind(h.Instance) == "FestivalMonster"
}

// OfferingBonus is how many blood offerings a kill of h is worth, as a
// festival monster: 1 unless SetOfferingBonus set another.
func (h *Hostile) OfferingBonus() int {
	return 1 + int(h.offeringBonus.Load())
}

// SetOfferingBonus sets how many blood offerings a kill of h is worth.
func (h *Hostile) SetOfferingBonus(n int) {
	h.offeringBonus.Store(int32(n - 1))
}
