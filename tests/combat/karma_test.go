package combat

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// recallItemTemplate is the test potion whose attached skill (2031) is a
// RECALL-type teleport.
const recallItemTemplate int32 = 1060

// recallSkillDefs defines skill 2031 as a RECALL so both karma-teleport
// guards resolve it.
func recallSkillDefs() []modelskill.Definition {
	return append(killSkillDefs(), modelskill.Definition{
		ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "RECALL", Potion: true, HitTime: 500, ReuseDelay: 1200, StaticHitTime: true, StaticReuse: true,
	})
}

// seedKarmaCharacter creates the account's single selectable character with
// its karma preset before the client dials, so karma-teleport guard tests
// never race a live connection. Create's INSERT omits karma, so Save writes
// it explicitly.
func seedKarmaCharacter(karma int) func(*gamesql.CharacterStore, *gamesql.ItemStore) {
	return func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		ch, err := player.NewCharacter(4242, gameservertest.ClassTemplate(), "player1", "Karma", 1, 0, 0, player.SexMale)
		if err != nil {
			panic(err)
		}
		ch.KarmaPoints = karma
		ctx := context.Background()
		if err := chars.Create(ctx, ch); err != nil {
			panic(err)
		}
		if err := chars.Save(ctx, ch); err != nil {
			panic(err)
		}
	}
}

// TestKarmaBlocksRecallTeleportItemUse pins the karma'd item-use guard: with
// KarmaPlayerCanTeleport off, using an item whose attached skill is RECALL is
// silently rejected — no packet, stack untouched — mirroring the reference's
// bare return.
func TestKarmaBlocksRecallTeleportItemUse(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithKarmaTeleport(false),
		gameservertest.WithSeed(seedKarmaCharacter(240)),
		gameservertest.WithSkills(combatPersistence(t, recallSkillDefs())),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	itemObjectID := srv.GiveItem(t, objID, recallItemTemplate, 5)
	startInWorld(t, c)

	c.Send(encodeUseItem(itemObjectID, false))
	if reply := c.ReadWithTimeout(500 * time.Millisecond); reply != nil {
		t.Fatalf("karma-blocked item use reply = opcode %#x, want no reply (silent reject)", reply[0])
	}

	instances, err := srv.Items.ListByOwner(context.Background(), objID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	for _, inst := range instances {
		if inst.ObjectID == itemObjectID && inst.Count != 5 {
			t.Fatalf("stack count after blocked karma-teleport use = %d, want 5 (unchanged)", inst.Count)
		}
	}
}

// TestKarmaBlocksDirectRecallCast pins the same gate on the direct-cast path:
// a karma'd player casting the RECALL skill gets ActionFailed and no cast.
func TestKarmaBlocksDirectRecallCast(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithKarmaTeleport(false),
		gameservertest.WithSeed(seedKarmaCharacter(240)),
		gameservertest.WithSkills(combatPersistence(t, recallSkillDefs())),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 2031, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(2031, false, false))
	frame := mustRead(t, c, "blocked cast reply")
	assertFrameOpcode(t, frame, serverpackets.OpcodeActionFailed, "blocked recall cast")
	if reply := c.ReadWithTimeout(readQuietWindow); reply != nil {
		t.Fatalf("unexpected extra reply after blocked recall cast = opcode %#x", reply[0])
	}
}

// TestPlayerKillGrantsPKKarma walks the PK flow end to end: killing a
// karma-free, unflagged victim credits the killer with a PK kill and karma,
// announces the new karma total (SystemMessage before StatusUpdate), and
// persists both counters.
func TestPlayerKillGrantsPKKarma(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Killer", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	victim := srv.SeedCharacterFor(t, "victim", "Victim", 1, 0)
	vc := srv.DialClient(t, "victim", 1)
	startInWorld(t, vc)
	startInWorld(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	_ = vc

	selectPlayerTarget(t, c, victim.ID)
	// An innocent victim is only attackable with force (ctrl), matching the
	// reference's isAttackableWithoutForce gate.
	c.Send(encodeRequestMagicSkillUse(42, true, false))
	readCastStartFrames(t, c, objID, 42, 1, 500, 60_000, victim.ID)

	// The killer's own client learns the new karma in the reference's order:
	// SystemMessage(YOUR_KARMA_HAS_BEEN_CHANGED_TO_S1) first — the first PK
	// kill awards 240 — then StatusUpdate(KARMA).
	assertKarmaChangeFrames(t, c, objID, 240)

	c.Send(encodeLogout())
	waitFor(t, "persisted PK counters", func() bool {
		ch, err := srv.Chars.Get(context.Background(), objID)
		return err == nil && ch.PKKills == 1 && ch.KarmaPoints == 240
	})
}

// selectPlayerTarget clicks another player and consumes the click's reply
// sequence: ValidateLocation, MyTargetSelected, and the target's StatusUpdate
// snapshot.
func selectPlayerTarget(t *testing.T, c *scriptedClient, objectID int32) {
	t.Helper()
	c.Send(encodeAction(objectID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "click ValidateLocation"), serverpackets.OpcodeValidateLocation, "click ValidateLocation")
	frame := mustRead(t, c, "MyTargetSelected")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	if got := wireReader(frame[1:]).ReadInt32(); got != objectID {
		t.Fatalf("MyTargetSelected object id = %d, want %d", got, objectID)
	}
	frame = mustRead(t, c, "target StatusUpdate")
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "target StatusUpdate")
	if got := wireReader(frame[1:]).ReadInt32(); got != objectID {
		t.Fatalf("StatusUpdate object id = %d, want %d", got, objectID)
	}
}

// assertKarmaChangeFrames scans frames until the karma-change announcement
// arrives and asserts its payload and the immediately following
// StatusUpdate(KARMA).
func assertKarmaChangeFrames(t *testing.T, c *scriptedClient, objectID, wantKarma int32) {
	t.Helper()
	for i := 0; i < 50; i++ {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			t.Fatalf("karma change message for %d never arrived", wantKarma)
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		if id := r.ReadInt32(); id != int32(serverpackets.SystemMessageYourKarmaHasBeenChangedToS1) {
			continue
		}
		if params := r.ReadInt32(); params != 1 {
			t.Fatalf("karma message params = %d, want 1", params)
		}
		if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
			t.Fatalf("karma message param type = %d, want number", typ)
		}
		if got := r.ReadInt32(); got != wantKarma {
			t.Fatalf("karma message value = %d, want %d", got, wantKarma)
		}

		status := mustRead(t, c, "karma StatusUpdate")
		assertFrameOpcode(t, status, serverpackets.OpcodeStatusUpdate, "karma StatusUpdate")
		r = wireReader(status[1:])
		if got := r.ReadInt32(); got != objectID {
			t.Fatalf("karma StatusUpdate object id = %d", got)
		}
		found := false
		count := r.ReadInt32()
		for i := int32(0); i < count; i++ {
			typ, val := r.ReadInt32(), r.ReadInt32()
			if typ == int32(serverpackets.StatusKarma) {
				found = true
				if val != wantKarma {
					t.Fatalf("StatusUpdate KARMA = %d, want %d", val, wantKarma)
				}
			}
		}
		if !found {
			t.Fatal("karma StatusUpdate carries no KARMA attribute")
		}
		return
	}
	t.Fatal("karma SystemMessage not found within 50 frames")
}
