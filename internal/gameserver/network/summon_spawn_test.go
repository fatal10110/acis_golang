package network

import (
	"testing"

	"github.com/rs/zerolog"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
)

// TestWireSummonAIForwardsManaFieldsToOwner pins the second half of issue
// #2350's scope: a summon's OnHitResult copies ManaDamageMissed and
// ManaDrains through to the owner alongside the existing
// AttackFailed/Lethals/MagicResists fields, and still drops
// OpponentMPReduced (Manadam.java:72 gates it `creature instanceof Player`,
// and a Summon isn't one).
func TestWireSummonAIForwardsManaFieldsToOwner(t *testing.T) {
	ownerFrames := &testsupport.FrameCapture{}
	targetFrames := &testsupport.FrameCapture{}
	owner := newTestLivePlayer(t, 100, ownerFrames)
	target := newTestLivePlayer(t, 200, targetFrames)

	state := world.New()
	state.AddPlayer(owner)
	state.AddPlayer(target)

	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID:       300,
		Owner:          owner,
		NPCID:          1,
		Name:           "Servitor",
		OwnerInventory: owner.Inventory(),
		Stats:          summon.CombatStats{MaxHP: 100, MaxMP: 100},
	})
	if err != nil {
		t.Fatalf("NewServitor() error: %v", err)
	}

	l := &GameClientLink{world: state, log: zerolog.Nop()}
	aiController := l.wireSummonAI(servitor)

	aiController.OnHitResult(actorcast.EffectResult{
		ManaDamageMissed:  1,
		ManaDrains:        []handlerskill.ManaDrain{{TargetID: 200, CasterName: "Servitor", MP: 15}},
		OpponentMPReduced: []int32{999},
	})

	ownerGot := ownerFrames.Frames()
	if len(ownerGot) != 1 {
		t.Fatalf("owner frame count = %d, want 1 (MISSED_TARGET only, OpponentMPReduced must not forward)", len(ownerGot))
	}
	assertStaticSystemMessageFrame(t, ownerGot[0], serverpackets.SystemMessageMissedTarget)

	targetGot := targetFrames.Frames()
	if len(targetGot) != 1 {
		t.Fatalf("target frame count = %d, want 1 (ManaDrain)", len(targetGot))
	}
	assertSystemMessageStringNumberFrame(t, targetGot[0], serverpackets.SystemMessageS2MPHasBeenDrainedByS1, "Servitor", 15)
}
