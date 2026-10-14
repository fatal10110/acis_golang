package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRaidCurseLandsOnRegisteredPlayer drives a level 20 player's swing
// into a level 1 raid-related monster, more than 8 levels below it: the
// raid petrification curse lands on the player, and the effect names, as
// its effected creature, the very object the world registers the player
// under.
func TestRaidCurseLandsOnRegisteredPlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, []modelskill.Definition{{
			ID: modelskill.RaidCurse2SkillID, Level: 1,
			Activation: modelskill.ActivationActive, Debuff: true, Offensive: true,
			SkillType: "PARALYZE", EffectRange: 2000,
			Effects: []modelskill.EffectTemplate{{
				Name: "Petrification", Count: 1, Time: 120, Icon: true,
				StackType: "turn_stone", StackOrder: 99, EffectPower: -1,
			}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
	onHostileQueue(t, hostile, func() { hostile.SetRaidRelated(true) })
	drainUntilQuiet(t, c)
	registered, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	// The swing's hit task runs the curse check, which then stops the
	// attack: no later swing follows.
	srv.Advance(t, 2*time.Second)
	srv.Settle(t)
	frames := readQuiet(c)
	if indexOf(frames, 0, serverpackets.OpcodeMagicSkillUse, hostile.ObjectID()) < 0 {
		t.Fatal("the raid curse's MagicSkillUse never reached the player")
	}

	var cursed *effect.Effect
	for _, e := range registered.(interface{ EffectList() *effect.List }).EffectList().All() {
		if e.Skill.ID == modelskill.RaidCurse2SkillID {
			cursed = e
		}
	}
	if cursed == nil {
		t.Fatal("the raid curse landed no effect on the player")
	}
	if any(cursed.Effected) != any(registered) {
		t.Fatalf("curse effected = %T %p, want the registered player %T %p", cursed.Effected, cursed.Effected, registered, registered)
	}
	if any(cursed.Effector) != any(hostile) {
		t.Fatalf("curse effector = %T %p, want the raid-related monster %p", cursed.Effector, cursed.Effector, hostile)
	}
}
