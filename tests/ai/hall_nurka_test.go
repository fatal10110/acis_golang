package ai

import (
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Bloody Lord Nurka, the Partisan Hideaway's standing guards, and Nurka's
// second form with his escort's healer, swordsman and archer.
const (
	nurkaID        = int32(35368) // BloodyLordNurka1
	standGuardID   = int32(35370) // Agit01Stand1
	standGuard2ID  = int32(35374) // Agit01Stand1
	nurka2ID       = int32(35375) // no behavior in this boot: the escort's master
	escortHealerID = int32(35376) // Agit01PartisanHealer2
	escortSwordID  = int32(35377) // Agit01Stand2
	escortBowID    = int32(35378) // Agit01Stand2
)

// Nurka's debuff, which the shipped skills do not hold, and magic strike.
const (
	hallNurkaDebuff = modelskill.ID(5456)
	hallNurkaStrike = modelskill.ID(4042)
)

// nurkaLanding is where Nurka sends the players in his territory as he
// appears; each lands up to 200 away on each axis.
var nurkaLanding = location.Location{X: 51952, Y: 111060, Z: -1970}

// TestNurkaSendsPlayersInHisTerritoryAway ports BloodyLordNurka1's created
// hook: Nurka appearing 100 from the character, inside the 200 of his
// territory, sends it to 51952, 111060, -1970, up to 200 away on each
// axis; appearing 500 away, he leaves it where it stands.
func TestNurkaSendsPlayersInHisTerritoryAway(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		dx    int
		moved bool
	}{{"inside", 100, true}, {"outside", 500, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootHalls(t, 0, 0, location.Location{}, raidBoot{}, agitTemplate(nurkaID))
			w.spawn(t, nurkaID, c.dx, 0)
			w.srv.Settle(t)
			x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
			if !c.moved {
				if at := (location.Location{X: x, Y: y, Z: z}); at != w.at {
					t.Fatalf("the character stands at %v, want %v where it entered", at, w.at)
				}
				return
			}
			if abs(x-nurkaLanding.X) > 200 || abs(y-nurkaLanding.Y) > 200 {
				t.Fatalf("the character stands at %d,%d,%d, want within 200 of %v", x, y, z, nurkaLanding)
			}
		})
	}
}

// wantHeldCast fails unless h wants to cast skill id on the character from
// where it stands, with weight.
func (w *warriorWorld) wantHeldCast(t *testing.T, name string, h *npc.Hostile, id modelskill.ID, weight float64) {
	t.Helper()
	wantCast(t, name, h, id, w.player, weight, false)
}

// noCast fails when h wants to cast skill id.
func noCast(t *testing.T, name string, h *npc.Hostile, id modelskill.ID) {
	t.Helper()
	if d, ok := castDesireOf(h, id); ok {
		t.Fatalf("%s cast desire of %d = %+v, want none", name, id, d)
	}
}

// TestNurkaAnswersAHit ports BloodyLordNurka1's attacked and clan-attacked
// hooks. The level 20 character hits a level 1 Nurka, more than 8 levels
// below it, for a twentieth of his hit points. He wants his debuff 5456 on
// it, which the skill table does not hold (no desire), and stops hating
// it. Then, on a draw under 10, he wants his strike 4042 at weight
// 1000000, and to attack it from where he stands by 10000 for the hit;
// his own clan call adds 5000, and on a draw under 2 his strike once more.
// A second hit makes him stop hating it again before he adds the 15000, so
// he hates it by 15000; a level 20 Nurka, not 8 levels below, keeps his
// hate, 30000.
func TestNurkaAnswersAHit(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		level  int
		draw   int
		strike float64
		hate   float64
	}{
		{"draw 0", 1, 0, 2000000, 15000},
		{"draw 2", 1, 2, 1000000, 15000},
		{"draw 10", 1, 10, 0, 15000},
		{"level 20", 20, 10, 0, 30000},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tmpl := agitTemplate(nurkaID)
			tmpl.Level = c.level
			w := bootHalls(t, 0, c.draw, location.Location{}, raidBoot{}, tmpl)
			nurka := w.spawn(t, nurkaID, 300, 0)

			w.hit(t, nurka, siegeTwentieth)
			noCast(t, "Nurka", nurka, hallNurkaDebuff)
			if c.strike > 0 {
				w.wantHeldCast(t, "Nurka", nurka, hallNurkaStrike, c.strike)
			} else {
				noCast(t, "Nurka", nurka, hallNurkaStrike)
			}
			w.holdsAttack(t, "Nurka", nurka, 15000)

			w.hit(t, nurka, siegeTwentieth)
			w.wantHate(t, "Nurka", nurka, c.hate)
		})
	}
}

// TestNurkaAnswersAClanCall ports BloodyLordNurka1's clan-attacked hook on
// a hit of a clan mate: Nurka wants to attack the character from where he
// stands by 5000 for a hit of a twentieth of his hit points, and on a
// draw under 2 to cast his strike 4042 at it, weight 1000000.
func TestNurkaAnswersAClanCall(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		draw   int
		strike bool
	}{{"draw 1", 1, true}, {"draw 2", 2, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootHalls(t, 0, c.draw, location.Location{}, raidBoot{}, agitTemplate(nurkaID), agitTemplate(siegeDummyID))
			mate := w.spawn(t, siegeDummyID, 300, 0)
			nurka := w.spawn(t, nurkaID, 300, 150)

			w.hit(t, mate, siegeTwentieth)
			if c.strike {
				w.wantHeldCast(t, "Nurka", nurka, hallNurkaStrike, 1000000)
			} else {
				noCast(t, "Nurka", nurka, hallNurkaStrike)
			}
			w.holdsAttack(t, "Nurka", nurka, 5000)
		})
	}
}

// TestNurkaStrikesWhoSpells ports BloodyLordNurka1's see-spell hook: the
// character's spell with aggro points on a monster beside him makes Nurka
// want to cast his strike 4042 at it from where he stands, weight 1000000,
// on a draw under 15, and nothing on a draw of 15.
func TestNurkaStrikesWhoSpells(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		draw   int
		strike bool
	}{{"draw 14", 14, true}, {"draw 15", 15, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			extra := siegeSkillBoot(t)
			w := bootHalls(t, 0, c.draw, location.Location{}, extra, agitTemplate(nurkaID), warriorTemplate(unboundID, nil))
			dummy := w.spawn(t, unboundID, 150, 0)
			nurka := w.spawn(t, nurkaID, 0, 300)

			w.selectTarget(t, dummy)
			w.cast(t, siegeBoltID)
			if c.strike {
				w.wantHeldCast(t, "Nurka", nurka, hallNurkaStrike, 1000000)
			} else {
				noCast(t, "Nurka", nurka, hallNurkaStrike)
			}
		})
	}
}

// TestPartisanStandingGuardsHoldTheirGround ports Agit01Stand1's hooks:
// each guard fights from where it stands. The character's hit of a
// twentieth of a guard's hit points makes it want to attack by 100, and
// the hit's clan calls, the guard's own first, add 100 to each guard. A
// guard that sees the character wants to attack it by 200, and its spell
// with 22 aggro points on a monster beside the guard by 22 / 440 / 0.05 *
// 150 = 150 more.
func TestPartisanStandingGuardsHoldTheirGround(t *testing.T) {
	t.Parallel()
	t.Run("hit", func(t *testing.T) {
		t.Parallel()
		w := bootHalls(t, 0, 0, location.Location{}, raidBoot{}, agitTemplate(standGuardID), agitTemplate(standGuard2ID))
		guard := w.spawn(t, standGuardID, 1000, 0)
		mate := w.spawn(t, standGuard2ID, 1000, 150)

		w.hit(t, guard, siegeTwentieth)
		w.holdsAttack(t, "the hit guard", guard, 200)
		w.holdsAttack(t, "its clan mate", mate, 100)
	})
	t.Run("sight and spell", func(t *testing.T) {
		t.Parallel()
		w := bootHalls(t, 0, 0, location.Location{}, siegeSkillBoot(t), agitTemplate(standGuardID), warriorTemplate(unboundID, nil))
		dummy := w.spawn(t, unboundID, 150, 0)
		guard := w.spawn(t, standGuardID, 0, 150)
		w.ticks(t, 1)
		w.holdsAttack(t, "the guard", guard, 200)

		w.selectTarget(t, dummy)
		w.cast(t, siegeBoltID)
		w.holdsAttack(t, "the guard", guard, 350)
	})
}

// escortWorld boots the character beside Nurka's second form, unbound
// here, whose template lists his escort: two healers, a swordsman and an
// archer. It raises him 300 from the character and his escort with him,
// and returns the world, Nurka and his escort by template.
func escortWorld(t *testing.T) (*warriorWorld, *npc.Hostile, map[int32][]*npc.Hostile) {
	t.Helper()
	master := agitTemplate(nurka2ID)
	for _, id := range []int32{escortHealerID, escortHealerID, escortSwordID, escortBowID} {
		master.Privates = append(master.Privates, npc.PrivateEntry{NpcID: int(id), Weight: 1, RespawnDelay: time.Hour})
	}
	w := bootHalls(t, 0, 0, location.Location{}, raidBoot{}, master, agitTemplate(escortHealerID), agitTemplate(escortSwordID), agitTemplate(escortBowID))
	nurka := w.spawn(t, nurka2ID, 300, 0)
	return w, nurka, w.raiseEscort(t, nurka)
}

// raiseEscort places the escort master's template lists and returns them
// by template, each template's in the order they appeared.
func (w *warriorWorld) raiseEscort(t *testing.T, master *npc.Hostile) map[int32][]*npc.Hostile {
	t.Helper()
	w.onQueue(t, master, func() {
		if err := w.srv.NpcSpawns.CreatePrivates(master); err != nil {
			t.Errorf("create privates: %v", err)
		}
	})
	escort := map[int32][]*npc.Hostile{}
	for _, h := range privatesOf(t, master, 4) {
		escort[int32(h.NpcID())] = append(escort[int32(h.NpcID())], h)
	}
	return escort
}

// setFlag sets the master's flag, as his own script would.
func (w *warriorWorld) setFlag(t *testing.T, master *npc.Hostile, flag int32) {
	t.Helper()
	w.onQueue(t, master, func() { script.NPCOf(master).SetInt(script.SlotFlag, flag) })
}

// wantWalk fails unless h has a desire to walk to at, weight 100000000,
// among its queued walks: one not reached yet stays queued.
func wantWalk(t *testing.T, name string, h *npc.Hostile, at location.Location) {
	t.Helper()
	var walks []actorai.Desire
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind != actorai.IntentionMoveTo {
			continue
		}
		if d.Weight == 100000000 && d.Location.X == at.X && d.Location.Y == at.Y && d.Location.Z == at.Z {
			return
		}
		walks = append(walks, d)
	}
	t.Fatalf("%s move desires = %+v, want one of weight 100000000 to %v", name, walks, at)
}

// TestEscortHealersFollowTheFlag ports Agit01PartisanHealer2's created,
// timer and dying hooks. Every 10 seconds a healer with a master walks
// once to the place of the master's flag: 20 to the side of the master
// while the flag is 0, to 45822 or 45782, 109981, -1970 when it is 1, to
// 44545 or 44505, 108867, -2020 for any other value. The first healer to
// appear takes the larger x of each pair (+20 beside the master), the
// other the smaller. Its dying lets the first of a fresh escort, raised
// once the dying hooks have run, take the larger x again.
func TestEscortHealersFollowTheFlag(t *testing.T) {
	t.Parallel()
	w, nurka, escort := escortWorld(t)
	first, second := escort[escortHealerID][0], escort[escortHealerID][1]
	if first.ObjectID() > second.ObjectID() {
		first, second = second, first
	}
	at := positionOf(nurka)

	w.srv.Advance(t, 10000*time.Millisecond)
	wantWalk(t, "the first healer", first, location.Location{X: at.X + 20, Y: at.Y, Z: at.Z})
	wantWalk(t, "the second healer", second, location.Location{X: at.X - 20, Y: at.Y, Z: at.Z})

	w.setFlag(t, nurka, 1)
	w.srv.Advance(t, 10000*time.Millisecond)
	wantWalk(t, "the first healer", first, location.Location{X: 45822, Y: 109981, Z: -1970})
	wantWalk(t, "the second healer", second, location.Location{X: 45782, Y: 109981, Z: -1970})

	w.setFlag(t, nurka, 2)
	w.srv.Advance(t, 10000*time.Millisecond)
	wantWalk(t, "the first healer", first, location.Location{X: 44545, Y: 108867, Z: -2020})
	wantWalk(t, "the second healer", second, location.Location{X: 44505, Y: 108867, Z: -2020})
	for _, h := range []*npc.Hostile{first, second} {
		if got := script.NPCOf(h).Int(script.SlotIntAI0); got != 3 {
			t.Fatalf("healer %d step = %d, want 3", h.ObjectID(), got)
		}
	}

	w.onQueue(t, first, func() { first.Kill(w.player) })
	w.srv.Advance(t, 3000*time.Millisecond) // the dying hooks run 3 seconds after the kill
	fresh := w.raiseEscort(t, nurka)[escortHealerID]
	if fresh[0].ObjectID() > fresh[1].ObjectID() {
		fresh[0], fresh[1] = fresh[1], fresh[0]
	}
	w.srv.Advance(t, 10000*time.Millisecond)
	wantWalk(t, "the fresh first healer", fresh[0], location.Location{X: 44545, Y: 108867, Z: -2020})
	wantWalk(t, "the fresh second healer", fresh[1], location.Location{X: 44505, Y: 108867, Z: -2020})
}

// TestEscortGuardsStayPut ports Agit01Stand2's created, timer and dying
// hooks. Each guard takes a post as it appears, but its 10 second check
// looks it up among the other kind's posts, finds none and stops there:
// whatever the master's flag, neither the swordsman nor the archer ever
// walks, and its step stays 0.
func TestEscortGuardsStayPut(t *testing.T) {
	t.Parallel()
	w, nurka, escort := escortWorld(t)
	guards := []*npc.Hostile{escort[escortSwordID][0], escort[escortBowID][0]}
	for _, flag := range []int32{0, 1, 2} {
		w.setFlag(t, nurka, flag)
		w.srv.Advance(t, 10000*time.Millisecond)
		for _, h := range guards {
			if d, ok := desire(h, actorai.IntentionMoveTo); ok {
				t.Fatalf("flag %d: guard %d move desire = %+v, want none", flag, h.NpcID(), d)
			}
			if got := script.NPCOf(h).Int(script.SlotIntAI0); got != 0 {
				t.Fatalf("flag %d: guard %d step = %d, want 0", flag, h.NpcID(), got)
			}
		}
	}
}
