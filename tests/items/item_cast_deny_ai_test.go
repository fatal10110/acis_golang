package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference for the tests below (#3069): an ItemSkills cast that is not a
// potion goes through PlayableAI.tryToCast (ItemSkills.java:79-80), whose
// first check refuses with ActionFailed alone while denyAiAction() holds
// (PlayableAI.java:297-302). For a player that is stunned, immobile until
// attacked, asleep, paralyzed, teleporting, dead or afraid, or in store or
// observer mode (Creature.java:636-639, Player.java:627-630). UseItem's own
// gate (UseItem.java:66) covers alike-dead, stun, sleep, paralysis and fear,
// so teleporting and ImmobileUntilAttacked are the states that reach
// tryToCast. (With any skill on reuse, ItemSkills' isSkillDisabled() already
// answers S1_PREPARED_FOR_REUSE under ImmobileUntilAttacked,
// Creature.java:1584-1590; the player below has none.) A cast already held as the next intention runs through
// PlayerAI.thinkCast when its wait ends; there the same denyAiAction()
// idles the AI and answers ActionFailed (PlayerAI.java:219-226).

// bootDenyAICaster boots a player with three escape scrolls in the world.
func bootDenyAICaster(t *testing.T) (*gameservertest.Server, int32, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(consumableSkills(t)),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1))
	objID := srv.SoleObjectID(t)
	scroll := srv.GiveItem(t, objID, escapeScrollID, 3)
	startInWorld(t, srv.Client)
	return srv, objID, scroll
}

// landImmobileUntilAttacked puts an ImmobileUntilAttacked effect on the
// player and returns it, so the test can lift it again.
func landImmobileUntilAttacked(t *testing.T, srv *gameservertest.Server, objID int32) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 4101, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "ImmobileUntilAttacked", Time: 60})
	if err != nil {
		t.Fatalf("effect.New(ImmobileUntilAttacked): %v", err)
	}
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
	var held bool
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { held = pc.DenyAIAction() })
	if !held {
		t.Fatal("ImmobileUntilAttacked landed but the player's AI actions are not denied")
	}
	drainUntilQuiet(t, srv.Client)
	return e
}

// assertScrollUsableAgain uses the scroll once the denying state is gone and
// requires its cast to start: the refused use left no reuse behind.
func assertScrollUsableAgain(t *testing.T, srv *gameservertest.Server, objID, scroll int32) {
	t.Helper()
	srv.Client.Send(encodeUseItem(scroll, false))
	assertMagicSkillUseSelf(t, srv.Client.Read(), objID, 2013, 1, 0, 5000)
}

// TestItemSkillCastWhileTeleportingAnswersActionFailed: an escape scroll used
// between a teleport and Appearing is answered with ActionFailed alone. No
// cast starts, the scroll is kept, and no reuse is started.
func TestItemSkillCastWhileTeleportingAnswersActionFailed(t *testing.T) {
	t.Parallel()
	srv, objID, scroll := bootDenyAICaster(t)
	c := srv.Client
	if !onlineLivePlayer(t, srv, objID).SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}

	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "item cast while teleporting")
	assertNoFrameFor(t, c, 300*time.Millisecond, "after the refused item cast")
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("item cast started while teleporting")
	}
	assertItemCount(t, srv, objID, scroll, 3)

	onlineLivePlayer(t, srv, objID).SetTeleporting(false)
	assertScrollUsableAgain(t, srv, objID, scroll)
}

// TestItemSkillCastWhileImmobileUntilAttackedAnswersActionFailed: the same
// refusal while an ImmobileUntilAttacked effect holds the player.
func TestItemSkillCastWhileImmobileUntilAttackedAnswersActionFailed(t *testing.T) {
	t.Parallel()
	srv, objID, scroll := bootDenyAICaster(t)
	c := srv.Client
	e := landImmobileUntilAttacked(t, srv, objID)

	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "item cast while immobile until attacked")
	assertNoFrameFor(t, c, 300*time.Millisecond, "after the refused item cast")
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("item cast started while immobile until attacked")
	}
	assertItemCount(t, srv, objID, scroll, 3)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.EffectList().Remove(e) })
	drainUntilQuiet(t, c)
	assertScrollUsableAgain(t, srv, objID, scroll)
}

// TestHeldItemSkillCastResumedWhileTeleportingGoesIdle: an escape scroll held
// behind a stand-up, whose owner is teleporting when the stand-up ends, is
// answered with ActionFailed alone. The held cast does not start and is not
// kept for later, and the scroll is kept.
func TestHeldItemSkillCastResumedWhileTeleportingGoesIdle(t *testing.T) {
	t.Parallel()
	srv, objID, scroll := bootDenyAICaster(t)
	if !srv.DrivesClock() {
		t.Skip("holding a stand-up open needs the driven clock")
	}
	c := srv.Client
	sitAndSettle(t, srv)
	standUp(t, c)
	c.Send(encodeUseItem(scroll, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "held item cast")

	live := onlineLivePlayer(t, srv, objID)
	if !live.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}
	srv.Advance(t, sitStandDelay)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "held item cast resumed while teleporting")
	assertNoFrameFor(t, c, 300*time.Millisecond, "after the refused held item cast")
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("held item cast started while teleporting")
	}

	// The held cast went idle: clearing the teleport does not revive it.
	live.SetTeleporting(false)
	assertNoFrameFor(t, c, 300*time.Millisecond, "after the teleport cleared")
	assertItemCount(t, srv, objID, scroll, 3)
}
