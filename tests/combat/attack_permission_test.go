package combat

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// arenaEverywhere is a zone index whose one arena covers every test spawn.
func arenaEverywhere(t *testing.T) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(-100_000, 100_000, -100_000, 100_000, -10_000, 10_000)
	if err != nil {
		t.Fatalf("arena form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewArena(1, form))
	return zones
}

// seedPlayer seeds a selectable character on account with the given level
// and karma, before its client dials.
func seedPlayer(t *testing.T, srv *gameservertest.Server, account, name string, level, karma int) int32 {
	t.Helper()
	id := srv.SeedCharacterFor(t, account, name, level, 0).ID
	ctx := context.Background()
	ch, err := srv.Chars.Get(ctx, id)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	ch.KarmaPoints = karma
	if err := srv.Chars.Save(ctx, ch.SaveState()); err != nil {
		t.Fatalf("save %s: %v", name, err)
	}
	return id
}

// frameIndex is the index of the first frame whose opcode is op and, for a
// system message, whose message id is msg; -1 when none.
func frameIndex(frames [][]byte, op byte, msg int32) int {
	for i, f := range frames {
		if f[0] != op {
			continue
		}
		if op == serverpackets.OpcodeSystemMessage && wireReader(f[1:]).ReadInt32() != msg {
			continue
		}
		return i
	}
	return -1
}

// TestKarmaPlayerCannotAttackBlessedLowLevelPlayer has a level-30 player
// with karma attack a level-10 player under Blessing of Protection. Outside
// a PvP zone the attack is refused with TARGET_IS_INCORRECT then
// ActionFailed, and no swing starts; inside an arena it proceeds.
func TestKarmaPlayerCannotAttackBlessedLowLevelPlayer(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		arena bool
	}{
		{"outside a PvP zone", false},
		{"inside an arena", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := []gameservertest.Option{
				gameservertest.WithCharacter("Blessed", 10, 0),
				gameservertest.WithWantChars(1),
			}
			if tt.arena {
				opts = append(opts, gameservertest.WithZones(arenaEverywhere(t)))
			}
			srv := gameservertest.Boot(t, opts...)
			c, victimID := srv.Client, srv.SoleObjectID(t)
			seedPlayer(t, srv, "pk", "Pk", 30, 500)
			pc := srv.DialClient(t, "pk", 1)
			startInWorld(t, c)
			startInWorld(t, pc)
			victim, ok := srv.State.Player(victimID)
			if !ok {
				t.Fatal("victim missing from world state")
			}
			landEffect(t, victim.(effectHolder), "ProtectionBlessing")
			drainUntilQuiet(t, c)
			drainUntilQuiet(t, pc)

			selectPlayerTarget(t, pc, victimID)
			full := srv.PlayerCurrentHP(t, victimID) + srv.PlayerCurrentCP(t, victimID)
			pc.Send(encodeAttackRequest(victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))

			if tt.arena {
				srv.AdvanceUntil(t, "the karma player's swing landing", func() bool {
					return srv.PlayerCurrentHP(t, victimID)+srv.PlayerCurrentCP(t, victimID) < full
				})
				if i := frameIndex(readQuiet(pc), serverpackets.OpcodeSystemMessage, int32(serverpackets.SystemMessageTargetIncorrect)); i >= 0 {
					t.Fatal("attack inside an arena answered TARGET_IS_INCORRECT")
				}
				return
			}

			frames := readQuiet(pc)
			refused := frameIndex(frames, serverpackets.OpcodeSystemMessage, int32(serverpackets.SystemMessageTargetIncorrect))
			failed := frameIndex(frames, serverpackets.OpcodeActionFailed, 0)
			if refused < 0 || failed < refused {
				t.Fatalf("refused attack frames = %v, want TARGET_IS_INCORRECT then ActionFailed", opcodes(frames))
			}
			if i := frameIndex(frames, serverpackets.OpcodeAttack, 0); i >= 0 {
				t.Fatalf("refused attack still swung: frames %v", opcodes(frames))
			}
			if srv.PlayerCurrentHP(t, victimID)+srv.PlayerCurrentCP(t, victimID) != full {
				t.Fatal("refused attack reached the blessed player")
			}
		})
	}
}

// TestRefusedAttackKeepsPickupInFlight has the karma player click a distant
// ground item and, mid-walk, send an attack at the blessed low-level player.
// The refusal replaces no intention: the walk goes on and the item is still
// picked up on arrival.
func TestRefusedAttackKeepsPickupInFlight(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Blessed", 10, 0), gameservertest.WithWantChars(1))
	c, victimID := srv.Client, srv.SoleObjectID(t)
	pkID := seedPlayer(t, srv, "pk", "Pk", 30, 500)
	adena := srv.GiveItem(t, pkID, item.AdenaID, 100)
	pc := srv.DialClient(t, "pk", 1)
	startInWorld(t, c)
	startInWorld(t, pc)
	victim, ok := srv.State.Player(victimID)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	landEffect(t, victim.(effectHolder), "ProtectionBlessing")
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, pc)
	selectPlayerTarget(t, pc, victimID)
	drainUntilQuiet(t, pc)

	x, y, z := int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z)
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDropItem)
	w.WriteInt32(adena)
	w.WriteInt32(40)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	pc.Send(w.Bytes())
	frame := mustRead(t, pc, "DropItem")
	assertFrameOpcode(t, frame, serverpackets.OpcodeDropItem, "DropItem")
	r := wireReader(frame[1:])
	r.ReadInt32() // dropper id
	groundID := r.ReadInt32()

	pc.Send(encodeMoveBackwardToLocation(x+200, y, z))
	srv.AdvanceUntil(t, "walk away completed", func() bool {
		px, _, _ := srv.PlayerPosition(t, pkID)
		return px == int(x)+200
	})
	drainUntilQuiet(t, pc)

	pc.Send(encodeAction(groundID, x, y, z, false))
	assertFrameOpcode(t, mustRead(t, pc, "pickup ActionFailed"), serverpackets.OpcodeActionFailed, "pickup pending-action release")
	pc.Send(encodeAttackRequest(victimID, x, y, z, false))

	refused, getItem := false, false
	deadline := pc.Now().Add(15 * time.Second)
	for !getItem && pc.Now().Before(deadline) {
		f := pc.ReadWithTimeout(500 * time.Millisecond)
		if f == nil {
			continue
		}
		switch {
		case f[0] == serverpackets.OpcodeSystemMessage && wireReader(f[1:]).ReadInt32() == int32(serverpackets.SystemMessageTargetIncorrect):
			refused = true
		case f[0] == serverpackets.OpcodeGetItem:
			if !refused {
				t.Fatal("pickup completed before the attack was refused; the refusal did not land mid-walk")
			}
			getItem = true
		}
	}
	if !refused {
		t.Fatal("attack on the blessed player was not refused with TARGET_IS_INCORRECT")
	}
	if !getItem {
		t.Fatal("refused attack dropped the pickup in flight: the item was never collected")
	}
}

// offensiveKillSkillDefs is killSkillDefs with skill 42 marked offensive, the
// way the skill loader marks a PDAM skill.
func offensiveKillSkillDefs() []modelskill.Definition {
	defs := killSkillDefs()
	defs[0].Offensive = true
	return defs
}

// TestOffensiveCastOnUnflaggedPlayerNeedsForceOutsidePvPZone casts an
// offensive skill without ctrl at an unflagged, karma-free player: outside a
// PvP zone the cast is refused as an invalid target, while with both players inside an arena it
// starts.
func TestOffensiveCastOnUnflaggedPlayerNeedsForceOutsidePvPZone(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		arena bool
	}{
		{"outside a PvP zone", false},
		{"inside an arena", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := []gameservertest.Option{
				gameservertest.WithCharacter("Caster", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(combatPersistence(t, offensiveKillSkillDefs())),
			}
			if tt.arena {
				opts = append(opts, gameservertest.WithZones(arenaEverywhere(t)))
			}
			srv := gameservertest.Boot(t, opts...)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, 42, 1)
			victim := srv.SeedCharacterFor(t, "victim", "Victim", 1, 0)
			vc := srv.DialClient(t, "victim", 1)
			startInWorld(t, vc)
			startInWorld(t, c)
			drainUntilQuiet(t, vc)
			drainUntilQuiet(t, c)

			selectPlayerTarget(t, c, victim.ID)
			if tt.arena {
				castKillSkill(t, srv, c, objID, victim.ID, false)
				return
			}
			c.Send(encodeRequestMagicSkillUse(42, false, false))
			frames := readQuiet(c)
			if frameIndex(frames, serverpackets.OpcodeSystemMessage, int32(serverpackets.SystemMessageInvalidTarget)) < 0 || frameIndex(frames, serverpackets.OpcodeMagicSkillUse, 0) >= 0 {
				t.Fatalf("unforced cast on an unflagged player outside a PvP zone = %v, want INVALID_TARGET and no cast", opcodes(frames))
			}
		})
	}
}
