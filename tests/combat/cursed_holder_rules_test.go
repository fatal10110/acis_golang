package combat

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: CreatureAttack.onHitTimer (CreatureAttack.java:136-141): a
// connecting swing of a player holding a cursed weapon, or of a hero at a
// holder, sets the struck player's CP to 0 (PlayerStatus.setCp, which
// reports the status) before the hit lands; an invulnerable target keeps
// its CP. Player.canBeHealed (Player.java:684-687): a holder is never
// healed (Heal.java:42).

// holderHealSkill is a plain single-target heal.
const holderHealSkill = 1011

// holderPair is a booted level-30 attacker, above the level a holder
// refuses to fight, and a level-40 victim with karma, both in
// the world with the cursed weapons running; every swing of the attacker
// lands without a critical.
type holderPair struct {
	srv                  *gameservertest.Server
	c, vc                *scriptedClient
	attackerID, victimID int32
}

// bootHolderPair boots the pair; hero makes the attacker an active hero.
func bootHolderPair(t *testing.T, hero bool) *holderPair {
	t.Helper()
	defs := append(cursedSkillDefs(), modelskill.Definition{
		ID: holderHealSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 600, HitTime: 500, StaticHitTime: true, StaticReuse: true, SkillType: "HEAL", Power: 100,
	})
	opts := []gameservertest.Option{
		gameservertest.WithCharacter("Attacker", 30, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithItemTemplates(cursedTemplates()),
		gameservertest.WithSkills(combatPersistence(t, defs)),
		gameservertest.WithCursedWeapons(cursedTable(t, 10)),
	}
	if hero {
		opts = append(opts, gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			if _, err := db.Exec(`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 0, 1, 1, 1 FROM characters WHERE char_name = 'Attacker'`); err != nil {
				t.Fatalf("seed hero: %v", err)
			}
		}))
	}
	srv := gameservertest.Boot(t, opts...)
	p := &holderPair{srv: srv, c: srv.Client, attackerID: srv.SoleObjectID(t)}
	seedKnownSkill(t, srv, p.attackerID, holderHealSkill, 1)
	p.victimID = srv.SeedCharacterFor(t, "victim", "Victim", 40, 0).ID
	ch, err := srv.Chars.Get(context.Background(), p.victimID)
	if err != nil {
		t.Fatalf("load victim: %v", err)
	}
	ch.KarmaPoints = 500
	if err := srv.Chars.Save(context.Background(), ch.SaveState()); err != nil {
		t.Fatalf("save victim karma: %v", err)
	}
	p.vc = srv.DialClient(t, "victim", 1)
	startInWorld(t, p.vc)
	startInWorld(t, p.c)
	drainUntilQuiet(t, p.vc)
	drainUntilQuiet(t, p.c)
	if got := victimCharacter(t, srv, p.attackerID).IsHero(); got != hero {
		t.Fatalf("attacker hero = %v, want %v", got, hero)
	}
	return p
}

// steady makes the attacker's swings land without a critical and fills
// the victim's HP and CP, then drains both clients.
func (p *holderPair) steady(t *testing.T) {
	t.Helper()
	attacker, victim := intakePlayerOf(t, p.srv, p.attackerID), intakePlayerOf(t, p.srv, p.victimID)
	onQueue(t, p.srv.PlayerQueue(t, p.attackerID), func() { attacker.SetRollSource(landNoCrit()) })
	maxHP, maxCP := p.srv.PlayerMaxHP(t, p.victimID), p.srv.PlayerMaxCP(t, p.victimID)
	onQueue(t, p.srv.PlayerQueue(t, p.victimID), func() {
		victim.SetHP(float64(maxHP))
		victim.SetCP(float64(maxCP))
	})
	drainUntilQuiet(t, p.vc)
	drainUntilQuiet(t, p.c)
}

// firstHit has the attacker auto-attack the victim until a first hit
// lands, and returns the victim's own status updates from then.
func (p *holderPair) firstHit(t *testing.T) [][]byte {
	t.Helper()
	before := p.srv.PlayerCurrentHP(t, p.victimID) + p.srv.PlayerCurrentCP(t, p.victimID)
	attackPlayer(t, p.c, p.victimID)
	p.srv.AdvanceUntil(t, "first hit on the victim", func() bool {
		return p.srv.PlayerCurrentHP(t, p.victimID)+p.srv.PlayerCurrentCP(t, p.victimID) < before
	})
	statuses, _ := statusFramesFor(t, p.vc, p.victimID)
	return statuses
}

// emptiedCPStatus is the victim's status as its CP drops to 0 at full HP.
func (p *holderPair) emptiedCPStatus(t *testing.T) []byte {
	t.Helper()
	return statusFixture(p.victimID,
		wantCurHP, int32(p.srv.PlayerMaxHP(t, p.victimID)),
		wantCurMP, int32(p.srv.PlayerCurrentMP(t, p.victimID)),
		wantCurCP, 0,
		wantMaxCP, int32(p.srv.PlayerMaxCP(t, p.victimID)),
	)
}

// TestCursedHolderHitEmptiesTargetCP: the holder's first connecting swing
// sets the struck player's CP to 0, reported at full HP, and the hit's
// damage then falls on HP.
func TestCursedHolderHitEmptiesTargetCP(t *testing.T) {
	t.Parallel()
	p := bootHolderPair(t, false)
	pickUpCursed(t, p.srv, p.c, p.attackerID)
	p.steady(t)

	statuses := p.firstHit(t)
	if len(statuses) < 2 {
		t.Fatalf("victim statuses = %x, want the emptied CP then the hit", statuses)
	}
	assertStatusFrames(t, "CP emptied", statuses[:1], p.emptiedCPStatus(t))
	if cp := p.srv.PlayerCurrentCP(t, p.victimID); cp != 0 {
		t.Fatalf("victim CP = %d after the hit, want 0", cp)
	}
	if hp := p.srv.PlayerCurrentHP(t, p.victimID); hp >= p.srv.PlayerMaxHP(t, p.victimID) {
		t.Fatalf("victim HP = %d after the hit, want the damage taken off it", hp)
	}
}

// TestHeroHitEmptiesHolderCP: a hero's connecting swing at a holder sets
// the holder's CP to 0; anyone else's hit at a holder takes only its
// damage off the CP.
func TestHeroHitEmptiesHolderCP(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		hero bool
	}{
		{"hero", true},
		{"not a hero", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := bootHolderPair(t, tc.hero)
			pickUpCursed(t, p.srv, p.vc, p.victimID)
			drainUntilQuiet(t, p.c)
			p.steady(t)

			statuses := p.firstHit(t)
			if len(statuses) == 0 {
				t.Fatal("no status for the holder")
			}
			if !tc.hero {
				// The holder's few CP go to the hit's damage, the rest to HP:
				// no status shows them emptied at full HP.
				if bytes.Equal(statuses[0], p.emptiedCPStatus(t)) {
					t.Fatalf("plain hit status = %x, want the hit's damage alone", statuses[0])
				}
				return
			}
			assertStatusFrames(t, "CP emptied", statuses[:1], p.emptiedCPStatus(t))
			if cp := p.srv.PlayerCurrentCP(t, p.victimID); cp != 0 {
				t.Fatalf("holder CP = %d after the hero's hit, want 0", cp)
			}
		})
	}
}

// TestCursedHolderCannotBeHealed: a heal cast on a holder restores
// nothing; the same heal on the victim before it holds the weapon does.
func TestCursedHolderCannotBeHealed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		holder bool
	}{
		{"holder", true},
		{"not a holder", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := bootHolderPair(t, false)
			if tc.holder {
				pickUpCursed(t, p.srv, p.vc, p.victimID)
				drainUntilQuiet(t, p.c)
			}
			victim := intakePlayerOf(t, p.srv, p.victimID)
			hurt := float64(p.srv.PlayerMaxHP(t, p.victimID) / 2)
			onQueue(t, p.srv.PlayerQueue(t, p.victimID), func() { victim.SetHP(hurt) })
			drainUntilQuiet(t, p.vc)
			selectPlayerTarget(t, p.c, p.victimID)
			drainUntilQuiet(t, p.c)

			// A friendly skill reaches a chaotic player only forced.
			p.c.Send(encodeRequestMagicSkillUse(holderHealSkill, true, false))
			readUntil(t, p.c, serverpackets.OpcodeMagicSkillUse, "heal MagicSkillUse")
			p.srv.Advance(t, time.Second)
			p.srv.Settle(t)
			readUntil(t, p.c, serverpackets.OpcodeMagicSkillLaunched, "heal MagicSkillLaunched")

			healed := p.srv.PlayerCurrentHP(t, p.victimID) > int(hurt)
			if healed == tc.holder {
				t.Fatalf("victim HP %d from %v after the heal: healed %v, want %v", p.srv.PlayerCurrentHP(t, p.victimID), hurt, healed, !tc.holder)
			}
		})
	}
}
