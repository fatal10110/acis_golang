package ai

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// shotSkillsOf returns the skill id of each MagicSkillUse frame id cast.
func shotSkillsOf(frames [][]byte, id int32) []int32 {
	var out []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, id) {
		// opcode, caster, target, skill id.
		out = append(out, int32(binary.LittleEndian.Uint32(f[9:13])))
	}
	return out
}

// TestFolkRunsTheWarriorAggressiveChain binds the WarriorAggressive chain
// to a civilian NPC whose template gives it one soulshot and a 100 percent
// soulshot rate, each hook of the chain wrapped to log once the chain's own
// hook returned. Seen, the NPC attacks the character. Its swing ends, and
// no attack-finished hook runs: a civilian NPC's attack reports no end.
// Hit, it runs the chain's attacked hook to its end, which charges its one
// soulshot, shown to the character as the 2154 self-cast; the next of its
// swings to land spends it. A second hit charges none, the spawn's
// soulshots spent.
func TestFolkRunsTheWarriorAggressiveChain(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	aggressive := func() script.Script {
		s := warrior.Aggressive()
		s.NPCs = []int32{folkPatrolID}
		base := s.Hooks
		s.Hooks = base.With(script.Hooks{
			OnSeeCreature: func(sc *script.Script, e script.SeeCreature) {
				base.SeeCreature(sc, e)
				log.add("SEE_CREATURE %d", e.Creature.ObjectID())
			},
			OnAttacked: func(sc *script.Script, e script.Attacked) {
				base.Attacked(sc, e)
				log.add("ATTACKED damage=%d", e.Damage)
			},
			OnAttackFinished: func(sc *script.Script, e script.AttackFinished) {
				log.add("ATTACK_FINISHED")
				base.AttackFinished(sc, e)
			},
		})
		return s
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkWarriorAggressive": aggressive},
		gameservertest.WithScriptRand(func(int) int { return 0 }))
	player := characterOf(t, srv)
	tmpl := folkFighterTemplate(folkPatrolID)
	tmpl.AIParams = npc.AIParams{"SoulShot": "1", "SoulShotRate": "100"}
	f := srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
	id := f.ObjectID()
	srv.ReadQueued(t, srv.Client)

	swings := framesOf(cycle(t, srv), serverpackets.OpcodeAttack, id)
	if got, want := log.take(), []string{fmt.Sprintf("SEE_CREATURE %d", player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	if len(swings) != 1 || attackTarget(swings[0]) != player.ObjectID() || swings[0][13]&serverpackets.AttackHitSoulshot != 0 {
		t.Fatalf("swings = %v, want one at the character, with no soulshot", swings)
	}
	if got := f.CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("intention = %v, want attack", got)
	}

	srv.Advance(t, 3*time.Second)
	srv.ReadQueued(t, srv.Client)
	if got := log.take(); len(got) != 0 {
		t.Fatalf("hooks = %q after the swings ended, want none", got)
	}

	hitFolk(t, srv, f, player, 10)
	if got, want := log.take(), []string{"ATTACKED damage=10"}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	frames := srv.ReadQueued(t, srv.Client)
	if got := shotSkillsOf(frames, id); !slices.Equal(got, []int32{2154}) {
		t.Fatalf("self-casts = %v, want the soulshot charge 2154", got)
	}
	if !f.SoulshotCharged() {
		t.Fatal("no soulshot charged after the attacked hook")
	}

	swung := 0
	for step := 0; f.SoulshotCharged(); step++ {
		if step == 30 {
			t.Fatalf("the soulshot is still charged after %d swings in 30s", swung)
		}
		srv.Advance(t, time.Second)
		swung += len(framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeAttack, id))
	}

	hitFolk(t, srv, f, player, 10)
	if got, want := log.take(), []string{"ATTACKED damage=10"}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	if got := shotSkillsOf(srv.ReadQueued(t, srv.Client), id); len(got) != 0 {
		t.Fatalf("self-casts = %v with the spawn's soulshots spent, want none", got)
	}
}

// TestFolkBehaviorFightOps drives the fighting operations of the script
// handle on a civilian NPC from its bound behavior, each hit's damage
// choosing the step, each logging only once its operations returned. The
// first hit reads the NPC unstunned, unparalyzed, with no failed path
// finding and able to attack the character, then queues an attack on it. The second drops every attack desire and zeroes the hate,
// keeping the character listed. The third aborts what the NPC does and
// kills it, the character taking the kill.
func TestFolkBehaviorFightOps(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	fighter := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
			OnAttacked: func(_ *script.Script, e script.Attacked) {
				n := e.NPC
				switch e.Damage {
				case 10:
					log.add("stunned=%v paralyzed=%v geofail=%d canattack=%v", n.Stunned(), n.Paralyzed(), n.GeoPathFailCount(), n.CanAttack(e.Attacker))
					n.AddAttackDesire(e.Attacker, 2000)
				case 20:
					n.RemoveAllAttackDesire()
					log.add("removed hated=%v", n.MostHated() != nil)
				case 30:
					n.AbortAll()
					n.DoDie(e.Attacker)
					log.add("died=%v", n.Dead())
				}
			},
			OnMyDying: func(_ *script.Script, e script.MyDying) {
				log.add("MY_DYING killer=%d", e.Killer.ObjectID())
			},
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkFighter": fighter})
	player := characterOf(t, srv)
	f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
	id := f.ObjectID()
	srv.ReadQueued(t, srv.Client)
	onPlayer := &actorai.Desire{Kind: actorai.IntentionAttack, FinalTarget: player}

	hitFolk(t, srv, f, player, 10)
	if got, want := log.take(), []string{"stunned=false paralyzed=false geofail=0 canattack=true"}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	if !f.Desires().Has(onPlayer) {
		t.Fatal("no attack desire on the character after the first hit")
	}
	srv.ReadQueued(t, srv.Client)

	hitFolk(t, srv, f, player, 20)
	if got, want := log.take(), []string{"removed hated=false"}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	if f.Desires().HasKind(actorai.IntentionAttack) {
		t.Fatal("an attack desire is left after RemoveAllAttackDesire")
	}
	if _, ok := f.Aggro().Get(player); !ok {
		t.Fatal("the character left the aggro list, want it kept with no hate")
	}
	srv.ReadQueued(t, srv.Client)

	hitFolk(t, srv, f, player, 30)
	srv.Advance(t, 4*time.Second)
	if got, want := log.take(), []string{"died=true", fmt.Sprintf("MY_DYING killer=%d", player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	if got := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeDie, id); len(got) != 1 {
		t.Fatalf("Die frames = %d, want 1", len(got))
	}
}
