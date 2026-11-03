package ai

import (
	"context"
	"database/sql"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The character's clan, which owns Aden Castle in the castle world below.
const (
	wardenClanID = 0x70000041
	adenCastleID = 5
)

// adenOwnerBoot is siegeSkillBoot with the shipped castles and their
// sieges, the character leading a clan that owns Aden Castle: every guard
// here is one of Aden's (castles.xml lists it among Aden's NPCs), so the
// character stands as its castle's owner (getPledgeCastleState 2).
func adenOwnerBoot(t *testing.T) raidBoot {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	boot := siegeSkillBoot(t)
	boot.opts = append(boot.opts,
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Hunter'", []any{wardenClanID}},
				{`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, hasCastle)
					SELECT ?, 'Wardens', 5, obj_Id, ? FROM characters WHERE char_name = 'Hunter'`, []any{wardenClanID, adenCastleID}},
			} {
				if _, err := db.ExecContext(context.Background(), q.q, q.args...); err != nil {
					t.Fatalf("seed the clan: %s: %v", q.q, err)
				}
			}
		}),
	)
	return boot
}

// TestSiegeGuardsSpareTheirCastleOwner ports the castle-state test of the
// siege guards' hooks (getPledgeCastleState(...) != 2) on Aden's guards and
// the character whose clan owns Aden Castle: the standing wizard that sees
// it casts nothing at it, a knight it hits wants nothing of it (attacked
// and the hit's clan call alike), and its Seal of Ruler landing on a
// knight or Duke Lewin Waldner leaves them alone, while a court guard,
// whose spelled hook has no such test, attacks it with weight 50000.
func TestSiegeGuardsSpareTheirCastleOwner(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, adenOwnerBoot(t),
		siegeCasterTemplate(adenWizardStandID, npc.SkillTypeDDMagic, siegeDDMagicID),
		siegeGuardTemplate(adenKnightID), siegeGuardTemplate(adenDukeID), siegeGuardTemplate(adenCourtGuardID))
	wizard := w.spawn(t, adenWizardStandID, 150, 0)
	w.srv.InstallNPCCastSeam(t, wizard, siegeNPCSkills())
	w.ticks(t, 2)
	if d, ok := desire(wizard, actorai.IntentionCast); ok {
		t.Fatalf("standing wizard wants %+v on its castle's owner", d)
	}

	knight := w.spawn(t, adenKnightID, 100, -400)
	w.hit(t, knight, siegeTwentieth)
	if got := knight.AggroHate(w.player); got != 0 {
		t.Fatalf("knight hate of its castle's owner after a hit = %v, want none", got)
	}

	for _, c := range []struct {
		name string
		id   int32
		dy   int
		want float64
	}{
		{"knight", adenKnightID, -400, 0},
		{"Duke Lewin Waldner", adenDukeID, 400, 0},
		{"court guard", adenCourtGuardID, 800, 50000},
	} {
		h := knight
		if c.id != adenKnightID {
			h = w.spawn(t, c.id, 100, c.dy)
		}
		w.selectTarget(t, h)
		w.cast(t, sealOfRulerID)
		if got := h.AggroHate(w.player); got != c.want {
			t.Fatalf("%s hate after its castle owner's Seal of Ruler = %v, want %v", c.name, got, c.want)
		}
	}
}
