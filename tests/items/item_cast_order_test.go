package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: PlayableCast.doCast (PlayableCast.java:46-62) destroys the item
// carrying a cast and calls addItemSkillTimeStamp (Playable.java:321-333:
// item reuse, ExUseSharedGroupItem) before CreatureCast.doCast
// (CreatureCast.java:100-150) rolls the skill mastery
// (SKILL_READY_TO_USE_AGAIN = 2015), installs the skill's reuse and charges
// the initial MP, whose StatusUpdate goes out before MagicSkillUse.

// masteryItemSkills is a skill table in which the Lesser Healing Potion's
// skill (2031) is cast through the AI cast path with an initial MP cost, and
// the two-skill scroll's second skill has one too.
func masteryItemSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: 248, Level: 3},
		{ID: 294, Level: 1},
		{
			ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DUMMY", StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 3000,
			MPInitialConsume: 2,
		},
		{
			ID: 2014, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", StaticHitTime: true, HitTime: 0, StaticReuse: true,
		},
		{
			ID: 2015, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 3000,
			MPInitialConsume: 2,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

// masteryPlayer is the world player's skill-mastery and reuse surface.
type masteryPlayer interface {
	AddStatFuncs([]effect.Mod)
	SetFloatRollSource(func(float64) float64)
	SkillDisabled(int32) bool
}

// forceSkillMastery makes every skill-mastery roll of objID's player proc.
func forceSkillMastery(t *testing.T, srv *gameservertest.Server, objID int32) masteryPlayer {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	p, ok := obj.(masteryPlayer)
	if !ok {
		t.Fatalf("world player %d = %T, want mastery-capable player", objID, obj)
	}
	p.AddStatFuncs([]effect.Mod{{Stat: stat.SkillMastery, Op: effect.OpSet, Value: 1000}})
	p.SetFloatRollSource(func(float64) float64 { return 99.5 })
	return p
}

// statusUpdateMP asserts frame is objID's StatusUpdate and returns its
// current-MP attribute.
func statusUpdateMP(t *testing.T, frame []byte, objID int32) int {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "StatusUpdate")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != objID {
		t.Fatalf("StatusUpdate object id = %d, want %d", id, objID)
	}
	for n := r.ReadInt32(); n > 0; n-- {
		typ, value := r.ReadInt32(), r.ReadInt32()
		if typ == int32(serverpackets.StatusCurrentMP) {
			return int(value)
		}
	}
	t.Fatal("StatusUpdate carries no current MP")
	return 0
}

// TestItemCastConsumesItemBeforeMasteryAndInitialMP: an item-carried cast
// with a mastery proc and an initial MP cost announces the item's shared
// reuse first, then the mastery proc, then the initial MP's StatusUpdate,
// then MagicSkillUse. The item unit is already gone by then, and the item's
// own reuse still holds the skill after the mastery proc skipped the
// skill's.
func TestItemCastConsumesItemBeforeMasteryAndInitialMP(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(masteryItemSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, 1060, 5)
	startInWorld(t, c)
	player := forceSkillMastery(t, srv, objID)
	mp := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeUseItem(potion, false))
	readExUseSharedGroupItem(t, c, 1060, 8, 10, 10)
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSkillReadyToUseAgain)
	if got := statusUpdateMP(t, c.Read(), objID); got != mp-2 {
		t.Fatalf("initial-MP StatusUpdate MP = %d, want %d", got, mp-2)
	}
	assertMagicSkillUseSelf(t, c.Read(), objID, 2031, 1, 500, 3000)

	if !player.SkillDisabled(actorcast.ReuseKey(modelskill.Definition{ID: 2031, Level: 1})) {
		t.Fatal("skill 2031 not on reuse after a mastery proc, want the item's 10 s reuse")
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	srv.FlushItems(t)
	if inst := mustFindItem(t, srv, objID, potion); inst.Count != 4 {
		t.Fatalf("persisted potion count = %d, want 4", inst.Count)
	}
}

// TestItemCastWithItemGoneChargesNothing: the second skill of a one-unit
// two-skill scroll waits for the first, which uses the unit up. When it
// runs, its item is gone: it answers NOT_ENOUGH_ITEMS and ActionFailed, and
// neither the mastery proc nor the initial MP is charged for it.
func TestItemCastWithItemGoneChargesNothing(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(masteryItemSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, gameservertest.TwoSkillScrollID, 1)
	startInWorld(t, c)
	player := forceSkillMastery(t, srv, objID)
	mp := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeUseItem(scroll, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSkillReadyToUseAgain)
	assertMagicSkillUseSelf(t, c.Read(), objID, 2014, 1, 0, 0)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued later item skill")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "first MagicSkillLaunched")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageNotEnoughItems)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "item-gone ActionFailed")
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("item-gone cast extra opcode = %#x, want none", frame[0])
	}

	if got := srv.PlayerCurrentMP(t, objID); got != mp {
		t.Fatalf("MP after the item-gone cast = %d, want %d unchanged", got, mp)
	}
	if player.SkillDisabled(actorcast.ReuseKey(modelskill.Definition{ID: 2015, Level: 1})) {
		t.Fatal("skill 2015 on reuse after its item was gone, want nothing charged")
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("still casting after the item-gone cast, want no cast")
	}
}
