package player

import "testing"

// TestSummonKillBetweenClansAtWar: killing the summon of a karma-free,
// unflagged player gives the killer karma at the summon rate, unless the
// two belong to clans at war with each other and neither is in its
// academy; a summon kill never gives a PvP point.
func TestSummonKillBetweenClansAtWar(t *testing.T) {
	mutual := map[[2]int32]bool{{10, 20}: true, {20, 10}: true}
	for _, tt := range []struct {
		name      string
		wars      map[[2]int32]bool
		academy   map[int32]bool
		wantKarma bool
	}{
		{name: "no war", wantKarma: true},
		{name: "mutual war", wars: mutual},
		{name: "one-sided war", wars: map[[2]int32]bool{{10, 20}: true}, wantKarma: true},
		{name: "academy owner", wars: mutual, academy: map[int32]bool{1: true}, wantKarma: true},
		{name: "academy killer", wars: mutual, academy: map[int32]bool{2: true}, wantKarma: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			owner, killer := policyPair(t, policyGraph{wars: tt.wars, academy: tt.academy}, policySide{clan: 10}, policySide{clan: 20})
			owner.AwardSummonKillKarma(killer)
			if got := killer.Karma() != 0; got != tt.wantKarma {
				t.Fatalf("killer gained karma = %v, want %v", got, tt.wantKarma)
			}
			if killer.PKKills != 0 || killer.PvPKills != 0 {
				t.Fatalf("summon kill counted PK %d, PvP %d; want neither", killer.PKKills, killer.PvPKills)
			}
		})
	}
}
