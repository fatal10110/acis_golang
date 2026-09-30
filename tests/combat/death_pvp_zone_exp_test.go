package combat

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Player.applyDeathPenalty's PvP-zone early returns
// (Player.java:2879-2895) and its caller in Player.doDie
// (Player.java:2615-2651), which clears the exp snapshot on every death
// with a killer and passes killedByPlayable = pk != null. Charm of Courage
// is skill 5041 (stayAfterDeath, CharmOfCourage effect); its onExit
// broadcasts EtcStatusUpdate, whose charm field is
// isAffected(CHARM_OF_COURAGE) (EtcStatusUpdate.java:25).

const charmOfCourageSkillID = 5041

// charmOfCourageSkills is the kill skill plus a Charm of Courage self-buff
// that survives death, as the datapack's skill 5041 does.
func charmOfCourageSkills() []modelskill.Definition {
	return append(killSkillDefs(), modelskill.Definition{
		ID: charmOfCourageSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 0, StaticHitTime: true, SkillType: "BUFF", StayAfterDeath: true,
		Effects: []modelskill.EffectTemplate{{Name: "CharmOfCourage", Time: 1200}},
	})
}

// activeSiegeEverywhere is a zone index whose one running siege battlefield
// covers every test spawn, so everyone in it is in both a PvP and a siege
// zone.
func activeSiegeEverywhere(t *testing.T) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(-100_000, 100_000, -100_000, 100_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("siege form: %v", err)
	}
	siege, err := zone.NewSiege(1, form, commons.NewStatSet())
	if err != nil {
		t.Fatalf("siege zone: %v", err)
	}
	siege.SetActive(true)
	zones := zone.NewIndex()
	zones.Add(siege)
	return zones
}

// seedCharacterWithDeathSnapshot is seedExperiencedCharacter with an exp
// snapshot left over from an earlier death.
func seedCharacterWithDeathSnapshot(exp, before int64) func(*gamesql.CharacterStore, *gamesql.ItemStore) {
	return func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		ch, err := player.NewCharacter(4242, gameservertest.ClassTemplate(), "player1", "Victim", 1, 0, 0, player.SexMale)
		if err != nil {
			panic(err)
		}
		ch.CharLevel = 5
		ch.Exp = exp
		ch.ExpBeforeDeath = before
		ctx := context.Background()
		if err := chars.Create(ctx, ch); err != nil {
			panic(err)
		}
		if err := chars.Save(ctx, ch.SaveState()); err != nil {
			panic(err)
		}
	}
}

// etcStatusCharm returns the charm field of every EtcStatusUpdate among
// frames, in order.
func etcStatusCharm(t *testing.T, frames [][]byte) []int32 {
	t.Helper()
	var out []int32
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeEtcStatusUpdate {
			continue
		}
		r := wireReader(f[1:])
		for range 5 {
			r.ReadInt32()
		}
		charm := r.ReadInt32()
		r.ReadInt32()
		if err := r.Err(); err != nil {
			t.Fatalf("read EtcStatusUpdate: %v", err)
		}
		out = append(out, charm)
	}
	return out
}

// pvpZoneDeathScene boots the victim (exp 1500 at level 5, so a full loss
// is 200) and a killer knowing the one-shot PDAM, both inside zones.
func pvpZoneDeathScene(t *testing.T, zones *zone.Index, seed func(*gamesql.CharacterStore, *gamesql.ItemStore)) (srv *gameservertest.Server, killer *scriptedClient, killerID, victimID int32) {
	t.Helper()
	srv = gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(seed),
		gameservertest.WithSkills(combatPersistence(t, charmOfCourageSkills())),
		gameservertest.WithLevels(deathLossTable(t)),
		gameservertest.WithAllowDelevel(true),
		gameservertest.WithZones(zones),
	)
	victimID = srv.SoleObjectID(t)
	seedKnownSkill(t, srv, victimID, charmOfCourageSkillID, 1)
	startInWorld(t, srv.Client)

	killerChar := srv.SeedCharacterFor(t, "killer", "Killer", 5, 0)
	seedKnownSkill(t, srv, killerChar.ID, 42, 1)
	killer = srv.DialClient(t, "killer", 1)
	startInWorld(t, killer)
	drainUntilQuiet(t, killer)
	drainUntilQuiet(t, srv.Client)
	return srv, killer, killerChar.ID, victimID
}

// TestSiegeCharmOfCourageSparesDeathExp: a player killed by another player
// inside a running siege battlefield while holding Charm of Courage loses
// no exp and uses the charm up, which the dying client learns from an
// EtcStatusUpdate with the charm cleared; the death still clears the
// earlier death's exp snapshot. Without the charm, the siege battlefield is
// no arena: the same kill costs a quarter of the normal loss (50 of 200).
func TestSiegeCharmOfCourageSparesDeathExp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		charm      bool
		wantExp    int64
		wantBefore int64
	}{
		{name: "charm", charm: true, wantExp: 1500, wantBefore: 0},
		{name: "no charm", charm: false, wantExp: 1450, wantBefore: 1500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, killer, killerID, victimID := pvpZoneDeathScene(t, activeSiegeEverywhere(t), seedCharacterWithDeathSnapshot(1500, 1700))
			c := srv.Client
			victim := reviveVictimOf(t, srv, victimID)
			if tc.charm {
				c.Send(encodeRequestMagicSkillUse(charmOfCourageSkillID, false, false))
				srv.Settle(t)
				if got := etcStatusCharm(t, readQuiet(c)); len(got) == 0 || got[len(got)-1] != 1 {
					t.Fatalf("EtcStatusUpdate charm fields after the charm = %v, want the last one 1", got)
				}
				if !hasEffectID(victim.EffectList().All(), charmOfCourageSkillID) {
					t.Fatal("Charm of Courage did not land")
				}
				drainUntilQuiet(t, killer)
			}

			killPrimaryClient(t, srv, killer, killerID, victimID)
			frames := readQuiet(c)
			die := indexOf(frames, 0, serverpackets.OpcodeDie, victimID)
			if die < 0 {
				t.Fatal("victim never received its own Die")
			}
			if tc.charm {
				got := etcStatusCharm(t, frames[die+1:])
				if len(got) == 0 || got[len(got)-1] != 0 {
					t.Fatalf("EtcStatusUpdate charm fields after Die = %v, want the last one 0", got)
				}
			}
			if hasEffectID(victim.EffectList().All(), charmOfCourageSkillID) {
				t.Fatal("Charm of Courage survived the siege death")
			}
			if exp, before := persistedExp(t, srv, c, victimID); exp != tc.wantExp || before != tc.wantBefore {
				t.Fatalf("persisted exp/before-death = %d/%d, want %d/%d", exp, before, tc.wantExp, tc.wantBefore)
			}
		})
	}
}

// TestArenaDeathToPlayerCostsNoExp: inside a PvP zone that is not a siege
// battlefield, a death to another player costs no exp (and clears the
// earlier death's snapshot), while a death to a monster costs the full loss.
func TestArenaDeathToPlayerCostsNoExp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		byPlayer   bool
		wantExp    int64
		wantBefore int64
	}{
		{name: "player", byPlayer: true, wantExp: 1500, wantBefore: 0},
		{name: "monster", byPlayer: false, wantExp: 1300, wantBefore: 1500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, killer, killerID, victimID := pvpZoneDeathScene(t, arenaEverywhere(t), seedCharacterWithDeathSnapshot(1500, 1700))
			c := srv.Client
			if tc.byPlayer {
				killPrimaryClient(t, srv, killer, killerID, victimID)
			} else {
				killByMonster(t, srv, reviveVictimOf(t, srv, victimID))
			}
			if exp, before := persistedExp(t, srv, c, victimID); exp != tc.wantExp || before != tc.wantBefore {
				t.Fatalf("persisted exp/before-death = %d/%d, want %d/%d", exp, before, tc.wantExp, tc.wantBefore)
			}
		})
	}
}
