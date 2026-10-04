package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// tunicID is the shared catalog's droppable chest armor.
const tunicID int32 = 40

// TestClanWarDeathDrop has the Knights founder, holding potions and a
// tunic, killed by a Rivals member while the clans are at war, every death
// set to drop everything. A karma-free founder drops nothing; a founder
// who is a player killer past the PK threshold drops both, the war
// notwithstanding.
func TestClanWarDeathDrop(t *testing.T) {
	t.Parallel()
	every := player.DeathDropRates{Chance: 100, Equip: 100, EquipWeapon: 100, Item: 100, Limit: 10}
	rules := player.DeathDropRules{Karma: every, Monster: every, KarmaPKLimit: 5}
	for _, tt := range []struct {
		name     string
		pk       bool
		wantDrop bool
	}{
		{name: "karma-free victim"},
		{name: "player killer victim", pk: true, wantDrop: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stmts := []string{
				warStmt(knightsClanID, rivalsClanID), warStmt(rivalsClanID, knightsClanID),
			}
			if tt.pk {
				stmts = append(stmts, `UPDATE characters SET karma = 240, pkkills = 5 WHERE char_name = 'Founder'`)
			}
			w := bootAllianceCast(t, nil, stmts, gameservertest.WithDeathDrop(rules))
			founderChar := onlineCharacter(t, w.srv, w.leaderID)
			onPlayerQueue(t, w.srv, w.leaderID, func() {
				founderChar.AddRewardItem(potionID, 5, 0x7f200001)
				founderChar.AddRewardItem(tunicID, 1, 0x7f200002)
			})
			drainFrames(t, w.leader)

			killPlayer(t, w, w.leaderID, w.memberID)
			founder := drainFrames(t, w.leader)

			dropped := slices.Contains(messages(t, founder), serverpackets.SystemMessageYouDroppedS1)
			if dropped != tt.wantDrop {
				t.Fatalf("founder told of a drop = %v, want %v", dropped, tt.wantDrop)
			}
			want := 0
			if tt.wantDrop {
				want = 2
			}
			if n := w.srv.GroundItems.Len(); n != want {
				t.Fatalf("ground items = %d, want %d", n, want)
			}
		})
	}
}
