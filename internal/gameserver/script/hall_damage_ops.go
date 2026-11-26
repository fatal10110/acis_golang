package script

// The per-clan damage tally the clan-hall boss scripts keep through a
// siege, and the clan it names the hall's winner.

// ClanDamage is, per clan, the damage it dealt a hall's boss this siege,
// each sum a counter the attacked hook adds to. The zero value is empty.
type ClanDamage struct {
	sums IntMapCell[*IntCell]
}

// Add adds n to clanID's damage, starting the clan's tally at 0 when it
// has none.
func (d *ClanDamage) Add(clanID int32, n int) {
	counter, _ := d.sums.SetIfAbsent(clanID, &IntCell{})
	counter.Add(n)
}

// Top names the clan that dealt the most damage, the first in the map's
// walk order among equals, and forgets every clan's damage; it is 0 when
// no clan dealt any. The sums wrap as 32-bit ints.
func (d *ClanDamage) Top() int32 {
	entries := d.sums.Entries()
	if len(entries) == 0 {
		return 0
	}
	best := entries[0]
	for _, e := range entries[1:] {
		if int32(e.Value.Get()) > int32(best.Value.Get()) {
			best = e
		}
	}
	d.sums.Clear()
	return best.Key
}

// LiveClanID returns id when that clan still exists, else 0.
func (s *Script) LiveClanID(id int32) int32 {
	if clan := s.Clan(id); clan != nil {
		return clan.ID()
	}
	return 0
}
