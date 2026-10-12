package castlemanor

// SubmitNextSeedProduction makes list castle castleID's seed list for the
// next period, while the manor is modifiable, keeping only each seed the
// manor holds, for a start amount within its SeedsLimit, at a price within
// its seed price bounds. A seed of another castle's manor passes. It
// reports false, setting nothing, when the manor is not modifiable.
func (m *Manager) SubmitNextSeedProduction(castleID int, list []*Production) bool {
	if !m.Enabled() {
		return false
	}
	kept := make([]*Production, 0, len(list))
	for _, sp := range list {
		s, ok := m.seeds.Seed(sp.ID)
		if ok && sp.StartAmount <= m.SeedsLimit(s) && sp.Price >= s.SeedMinPrice() && sp.Price <= s.SeedMaxPrice() {
			kept = append(kept, sp)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status != StatusModifiable {
		return false
	}
	if p, ok := m.byCastle[castleID]; ok {
		p.productionNext = kept
	}
	return true
}

// SubmitNextCropProcure makes list castle castleID's crop list for the
// next period, while the manor is modifiable, keeping only each crop of a
// seed of that castle's manor, for a start amount within its CropsLimit,
// at a price within its crop price bounds. It reports false, setting
// nothing, when the manor is not modifiable.
func (m *Manager) SubmitNextCropProcure(castleID int, list []*Procure) bool {
	if !m.Enabled() {
		return false
	}
	kept := make([]*Procure, 0, len(list))
	for _, cp := range list {
		s, ok := m.seeds.SeedByCropForCastle(int(cp.ID), castleID)
		if ok && cp.StartAmount <= m.CropsLimit(s) && cp.Price >= s.CropMinPrice() && cp.Price <= s.CropMaxPrice() {
			kept = append(kept, cp)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status != StatusModifiable {
		return false
	}
	if p, ok := m.byCastle[castleID]; ok {
		p.procureNext = kept
	}
	return true
}
