package skills

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: CreatureAttack.onHitTimer calls Player.updatePvPStatus once per
// swing, for the swing's main target only; it flags the attacker for
// PVP_PVP_TIME when that target is a flagged karma-free player (checkIfPvP)
// and PVP_NORMAL_TIME otherwise, and a main target that is no player flags
// nothing. A pole swing's splash targets take no part in either.

// poleTarget is one creature a pole swing hits: a monster, or a level 40
// player that survives the swing.
type poleTarget struct {
	id      int32
	player  bool
	client  *testsupport.ScriptedClient
	monster interface{ CurrentHP() int }
}

func (p poleTarget) hp(srv *gameservertest.Server, t *testing.T) int {
	if p.player {
		return srv.PlayerCurrentHP(t, p.id)
	}
	return p.monster.CurrentHP()
}

// TestPoleSwingPvPFlagFollowsTheMainTarget swings a pole at a main target with
// a splash target behind it and reads the attacker's flag when the hit has
// landed, and 30 seconds later, past the flagged-target time and short of
// the normal one.
func TestPoleSwingPvPFlagFollowsTheMainTarget(t *testing.T) {
	t.Parallel()
	type kind int
	const (
		monster kind = iota
		innocent
		flagged
	)
	for _, tc := range []struct {
		name         string
		main, splash kind
		splashHit    bool
		now, later   task.PvPFlagState
	}{
		// A player is swept only behind a main target that is a player
		// too, and only when it may be attacked without force.
		{name: "monster main, flagged player splash", main: monster, splash: flagged, now: task.PvPFlagNone, later: task.PvPFlagNone},
		{name: "monster main, innocent player splash", main: monster, splash: innocent, now: task.PvPFlagNone, later: task.PvPFlagNone},
		{name: "innocent player main, monster splash", main: innocent, splash: monster, splashHit: true, now: task.PvPFlagOn, later: task.PvPFlagOn},
		{name: "flagged player main, monster splash", main: flagged, splash: monster, splashHit: true, now: task.PvPFlagOn, later: task.PvPFlagNone},
		{name: "innocent player main, flagged player splash", main: innocent, splash: flagged, splashHit: true, now: task.PvPFlagOn, later: task.PvPFlagOn},
		{name: "flagged player main, innocent player splash", main: flagged, splash: innocent, now: task.PvPFlagOn, later: task.PvPFlagNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var clock atomic.Int64
			start := time.UnixMilli(1_000)
			clock.Store(start.UnixNano())
			flags := task.NewPvPFlags(task.DefaultPvPFlagOptions(), func() time.Time { return time.Unix(0, clock.Load()) })
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{poleCount()})),
				gameservertest.WithItemTemplates(poleCatalog()),
				gameservertest.WithPvPFlags(flags),
			)
			if !srv.DrivesClock() {
				t.Skip("holding the swing between the flag ticks needs the driven clock")
			}
			c, objID := srv.Client, srv.SoleObjectID(t)
			pole := srv.GiveItem(t, objID, poleWeaponID, 1)
			seedKnownSkill(t, srv, objID, poleCountSkillID, 1)
			startInWorld(t, c)
			use := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
			use.WriteInt32(pole)
			use.WriteInt32(0)
			c.Send(use.Bytes())
			srv.InventoryUpdates.Tick()
			drainUntilQuiet(t, c)
			steadyHits(t, srv, objID)

			px, py, pz := srv.PlayerPosition(t, objID)
			accounts := 0
			spawn := func(k kind, dy int) poleTarget {
				at := location.Location{X: px, Y: py + dy, Z: pz}
				if k == monster {
					h := srv.SpawnHostileNPCAt(t, at)
					return poleTarget{id: h.ObjectID(), monster: h}
				}
				accounts++
				account, name := "poleacc"+string(rune('a'+accounts)), "Pole"+string(rune('A'+accounts))
				ch := srv.SeedCharacterForAt(t, account, name, 40, 0, at)
				vc := srv.DialClient(t, account, 1)
				startInWorldAmongPlayers(t, vc)
				if k == flagged {
					obj, ok := srv.State.Player(ch.ID)
					if !ok {
						t.Fatalf("player %d missing from world state", ch.ID)
					}
					done := make(chan struct{})
					if !srv.PlayerQueue(t, ch.ID).Post(func() {
						defer close(done)
						flags.AddNormal(obj.(task.PvPFlagActor))
					}) {
						t.Fatal("victim queue closed")
					}
					<-done
				}
				return poleTarget{id: ch.ID, player: true, client: vc}
			}
			// Players enter first: a monster standing in the world would put
			// its NpcInfo into their EnterWorld burst.
			var main, splash poleTarget
			if tc.main == monster {
				splash = spawn(tc.splash, 45)
				main = spawn(tc.main, 30)
			} else {
				main = spawn(tc.main, 30)
				splash = spawn(tc.splash, 45)
			}
			drainUntilQuiet(t, c)
			mainHP, splashHP := main.hp(srv, t), splash.hp(srv, t)

			c.Send(encodeAction(main.id, int32(px), int32(py), int32(pz), false))
			drainUntilQuiet(t, c)
			c.Send(encodeAttackRequestOn(main.id))
			srv.AdvanceUntil(t, "the swing landing on its main target", func() bool { return main.hp(srv, t) < mainHP })
			srv.Settle(t)
			if hit := splash.hp(srv, t) < splashHP; hit != tc.splashHit {
				t.Fatalf("splash target hit = %v, want %v", hit, tc.splashHit)
			}

			obj, ok := srv.State.Player(objID)
			if !ok {
				t.Fatal("attacker missing from world state")
			}
			attacker := obj.(interface{ PvPFlagState() task.PvPFlagState })
			if got := attacker.PvPFlagState(); got != tc.now {
				t.Fatalf("attacker flag after the swing = %v, want %v", got, tc.now)
			}
			clock.Add(int64(30 * time.Second))
			flags.Tick()
			srv.Settle(t)
			if got := attacker.PvPFlagState(); got != tc.later {
				t.Fatalf("attacker flag 30s after the swing = %v, want %v", got, tc.later)
			}
		})
	}
}
