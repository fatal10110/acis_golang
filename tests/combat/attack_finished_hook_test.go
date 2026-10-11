package combat

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAttackFinishedHookOncePerSwing (AttackableAttack.onFinishedAttack):
// each swing a monster bound to a behavior completes raises the behavior's
// attack-finished hook once, naming the NPC and the swing's target.
func TestAttackFinishedHookOncePerSwing(t *testing.T) {
	t.Parallel()
	const npcID = int32(20050)
	var (
		mu    sync.Mutex
		lines []string
	)
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{npcID}, Hooks: script.Hooks{
			OnAttackFinished: func(_ *script.Script, e script.AttackFinished) {
				mu.Lock()
				defer mu.Unlock()
				lines = append(lines, fmt.Sprintf("npc=%d target=%T:%d", e.NPC.ObjectID(), e.Target, e.Target.ObjectID()))
			},
		}}
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{npcID: script.KindHostile},
			[]script.Listing{{Path: "ai.Recorder"}}, script.Catalog{"ai.Recorder": behavior}),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID).(attackable.Combatant)
	tmpl := gameservertest.AttackingHostileTemplate()
	tmpl.ID, tmpl.TemplateID = int(npcID), int(npcID)
	attacker := srv.SpawnAttackingHostileNPCTemplate(t, tmpl, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	want := fmt.Sprintf("npc=%d target=*script.Player:%d", attacker.ObjectID(), objID)
	for swing := 1; swing <= 2; swing++ {
		attacker.DoAttack(t, victim)
		drainUntilQuiet(t, c)
		// Back to full HP, so the next swing does not kill the player.
		srv.AddPlayerHP(t, objID, 1000)
		mu.Lock()
		got := slices.Clone(lines)
		mu.Unlock()
		if wantLines := slices.Repeat([]string{want}, swing); !slices.Equal(got, wantLines) {
			t.Fatalf("after swing %d: attack-finished hooks = %q, want %q", swing, got, wantLines)
		}
	}
}
