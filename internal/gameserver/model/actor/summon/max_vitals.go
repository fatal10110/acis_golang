package summon

// SetMaxHpMp fills a's MP, then its HP, to their maxima and republishes its
// vitals once. A dead summon keeps its values and republishes nothing.
func (a *Actor) SetMaxHpMp() {
	maxHP, maxMP := a.MaxHPValue(), a.MaxMPValue()
	a.vitals.mu.Lock()
	if a.dead {
		a.vitals.mu.Unlock()
		return
	}
	a.vitals.mp = maxMP
	a.vitals.hp = maxHP
	a.vitals.mu.Unlock()
	a.BroadcastStatus()
}
