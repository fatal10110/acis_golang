package script

// The per-clan damage tally the clan-hall boss scripts keep through a
// siege, and the clan it names the hall's winner.

// AddClanDamage adds n to clanID's damage in damage, starting the clan's
// tally at 0 when it has none.
func AddClanDamage(damage *IntMapCell[*IntCell], clanID int32, n int) {
	counter, _ := damage.SetIfAbsent(clanID, &IntCell{})
	counter.Add(n)
}

// TopDamageClan names the clan that dealt the most damage, the first in
// the map's walk order among equals, and forgets every clan's damage; it
// is 0 when no clan dealt any, or when that clan is gone. The sums wrap as
// 32-bit ints.
func (s *Script) TopDamageClan(damage *IntMapCell[*IntCell]) int32 {
	entries := damage.Entries()
	if len(entries) == 0 {
		return 0
	}
	best := entries[0]
	for _, e := range entries[1:] {
		if int32(e.Value.Get()) > int32(best.Value.Get()) {
			best = e
		}
	}
	damage.Clear()
	if clan := s.Clan(best.Key); clan != nil {
		return clan.ID()
	}
	return 0
}
