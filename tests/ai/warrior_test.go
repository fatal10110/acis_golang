package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Warrior chain's NPC ids the scenarios spawn, each a fixture monster
// of level 1, and an id no behavior is bound to.
const (
	warriorID       = int32(20001) // Warrior
	warriorTwoID    = int32(20003) // Warrior
	warriorFarID    = int32(20004) // Warrior
	aggressiveID    = int32(20012) // WarriorAggressive
	aggressiveTwoID = int32(20080) // WarriorAggressive
	unboundID       = int32(29990) // no behavior
)

// warriorScripts are the behavior chain down to the Warrior behaviors and
// their children, in scripts.xml order, as the server registers them.
func warriorScripts() ([]script.Listing, script.Catalog) {
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":                                                                defaultnpc.New,
		"script.ai.individual.Monster.MonsterAI":                                                         monster.New,
		"script.ai.individual.Monster.WarriorBase.WarriorBase":                                           warrior.Base,
		"script.ai.individual.Monster.WarriorBase.Warrior.Warrior":                                       warrior.Warrior,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive":           warrior.Aggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHold.WarriorHold":                       warrior.Hold,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorFishingBlock":    warrior.FishingBlock,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecial": warrior.PhysicalSpecial,
	}
	var list []script.Listing
	for _, path := range []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.WarriorBase.WarriorBase",
		"script.ai.individual.Monster.WarriorBase.Warrior.Warrior",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHold.WarriorHold",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorFishingBlock",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecial",
	} {
		list = append(list, script.Listing{Path: path})
	}
	return list, catalog
}

// warriorTemplate is a level 1 fixture monster of id with ai as its AI
// parameters.
func warriorTemplate(id int32, ai npc.AIParams) *npc.Template {
	return &npc.Template{
		ID: int(id), TemplateID: int(id), Type: "Monster", Name: "Warrior", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CollisionRadius: 10, CollisionHeight: 20,
		AIParams: ai,
	}
}

// warriorWorld is one character in the world beside the Warrior chain's
// fixture NPCs, every script draw 0.
type warriorWorld struct {
	srv    *gameservertest.Server
	at     location.Location
	player attackable.Combatant
}

func bootWarriors(t *testing.T, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	list, catalog := warriorScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	)
	c := srv.Client
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.ReadQueued(t, c)
	id := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, id)
	obj, ok := srv.State.Object(id)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	return &warriorWorld{srv: srv, at: location.Location{X: x, Y: y, Z: z}, player: obj.(attackable.Combatant)}
}

// spawn places an NPC of id dx, dy from the character, through the live
// population, so its created hooks run.
func (w *warriorWorld) spawn(t *testing.T, id int32, dx, dy int) *npc.Hostile {
	t.Helper()
	n, err := w.srv.NpcSpawns.AddSpawn(id, w.at.X+dx, w.at.Y+dy, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", id, err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	return n.(*npc.Hostile)
}

// hit lands a hit of damage on h from the character, on its queue.
func (w *warriorWorld) hit(t *testing.T, h *npc.Hostile, damage int) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		h.TakeDamage(damage, w.player)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// ticks runs n AI task cycles and returns the frames they sent.
func (w *warriorWorld) ticks(t *testing.T, n int) [][]byte {
	t.Helper()
	var frames [][]byte
	for range n {
		frames = append(frames, cycle(t, w.srv)...)
	}
	return frames
}

// TestWarriorAggressiveAttacksOnSight runs the first stand-in cut-over: a
// WarriorAggressive NPC, with no aggressive time, wants to attack the
// character it sees with weight 200, while an unbound monster beside it
// ignores the character. A hit of 10 on a level 1 Warrior NPC adds the
// Warrior formula's hate, damage / (level + 7) * 100, once: the built-in
// attacked hate stays off for it, and still adds the same amount for the
// unbound monster.
func TestWarriorAggressiveAttacksOnSight(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(aggressiveID, nil), warriorTemplate(warriorID, nil), warriorTemplate(unboundID, nil))
	aggressive := w.spawn(t, aggressiveID, 150, 0)
	plain := w.spawn(t, warriorID, 0, 150)
	unbound := w.spawn(t, unboundID, -150, 0)

	w.ticks(t, 2)
	if got := aggressive.AggroHate(w.player); got != 200 {
		t.Fatalf("WarriorAggressive hate of the character it sees = %v, want 200", got)
	}
	if got := aggressive.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("WarriorAggressive intention = %v, want attack", got)
	}
	for _, h := range []*npc.Hostile{plain, unbound} {
		if got := h.AggroHate(w.player); got != 0 {
			t.Fatalf("npc %d hate of the character it sees = %v, want none", h.NpcID(), got)
		}
	}

	w.hit(t, plain, 10)
	w.hit(t, unbound, 10)
	for _, h := range []*npc.Hostile{plain, unbound} {
		if got := h.AggroHate(w.player); got != 125 {
			t.Errorf("npc %d hate after a hit of 10 = %v, want 125", h.NpcID(), got)
		}
	}
}

// TestWarriorAssistsItsClanOnceSettled shows the Warrior clan call, for
// Warrior and WarriorAggressive ids alike: a hit on one NPC makes a clan
// member in range want to attack the attacker, with the damage formula's
// hate, once the member has lived more than 7 AI cycles; before that it
// does not react. The clan call reaches the victim itself first, so a
// settled victim adds the formula's hate twice: once from its attacked
// hook, once from its own clan call. The NPCs stand beyond the see range
// of the character, so the aggressive ones do not turn on it on sight.
func TestWarriorAssistsItsClanOnceSettled(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name             string
		victimID, member int32
	}{
		{"Warrior", warriorID, warriorTwoID},
		{"WarriorAggressive", aggressiveID, aggressiveTwoID},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			clan := func(id int32) *npc.Template {
				tmpl := warriorTemplate(id, nil)
				tmpl.Clans, tmpl.ClanRange = []string{"gremlins"}, 500
				return tmpl
			}
			w := bootWarriors(t, clan(row.victimID), clan(row.member))
			victim := w.spawn(t, row.victimID, 700, 0)
			member := w.spawn(t, row.member, 700, 100)

			w.ticks(t, 2)
			for _, h := range []*npc.Hostile{victim, member} {
				if got := h.AggroHate(w.player); got != 0 {
					t.Fatalf("npc %d hate before any hit = %v, want none", h.NpcID(), got)
				}
			}
			w.hit(t, victim, 10)
			if got := victim.AggroHate(w.player); got != 125 {
				t.Fatalf("unsettled victim hate after a hit of 10 = %v, want 125", got)
			}
			if got := member.AggroHate(w.player); got != 0 {
				t.Fatalf("clan member hate after a hit at life time %d = %v, want none", member.LifeTime(), got)
			}

			w.ticks(t, 8)
			if member.LifeTime() <= 7 {
				t.Fatalf("clan member life time = %d, want past 7", member.LifeTime())
			}
			before := victim.AggroHate(w.player)
			w.hit(t, victim, 10)
			if got := member.AggroHate(w.player); got != 125 {
				t.Fatalf("clan member hate after a hit of 10 = %v, want 125", got)
			}
			if got := victim.AggroHate(w.player) - before; got < 249.99 || got > 250.01 {
				t.Fatalf("settled victim hate gained from a hit of 10 = %v, want 250", got)
			}
		})
	}
}

// TestWarriorCreatedTimerPlaysSocial shows the timer a Warrior or
// WarriorAggressive NPC arms when it is created with a MoveAroundSocial
// parameter: every 10 s, while it is idle or wandering, it plays its
// social animation (the draw under 20 wins with every draw 0), and an NPC
// without the parameter arms nothing. The NPCs stand beyond the see range
// of the character, so the aggressive ones stay idle.
func TestWarriorCreatedTimerPlaysSocial(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name            string
		socialID, still int32
	}{
		{"Warrior", warriorID, warriorTwoID},
		{"WarriorAggressive", aggressiveID, aggressiveTwoID},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootWarriors(t, warriorTemplate(row.socialID, npc.AIParams{"MoveAroundSocial": "42"}), warriorTemplate(row.still, nil))
			social := w.spawn(t, row.socialID, 700, 0)
			still := w.spawn(t, row.still, 700, 150)

			if frames := w.ticks(t, 2); len(framesOf(frames, serverpackets.OpcodeSocialAction, social.ObjectID())) != 0 {
				t.Fatal("the warrior played a social animation before its timer fired")
			}
			w.srv.Advance(t, 10*time.Second)
			frames := append(w.srv.ReadQueued(t, w.srv.Client), w.ticks(t, 2)...)
			if got := framesOf(frames, serverpackets.OpcodeSocialAction, social.ObjectID()); len(got) != 1 {
				t.Fatalf("social animations of the warrior after its timer = %d, want 1", len(got))
			}
			if got := framesOf(frames, serverpackets.OpcodeSocialAction, still.ObjectID()); len(got) != 0 {
				t.Fatalf("social animations of the warrior without the parameter = %d, want none", len(got))
			}
			if got := social.AggroHate(w.player); got != 0 {
				t.Fatalf("social warrior hate of the character = %v, want none", got)
			}
		})
	}
}

// TestWarriorShoutTargetReachesOnlyItsRadius shows the script event the
// Warrior base broadcasts on attacked: a Warrior with ShoutTarget whose
// most hated creature is its attacker sends event 10016 to the NPCs within
// 300 (the draw under 5 wins). A bound NPC within it answers by wanting to
// attack the attacker with weight 100 (the draw under 50 wins); one 600
// away hears nothing, and the sender does not hear itself.
func TestWarriorShoutTargetReachesOnlyItsRadius(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t,
		warriorTemplate(warriorID, npc.AIParams{"ShoutTarget": "1"}),
		warriorTemplate(warriorTwoID, nil),
		warriorTemplate(warriorFarID, nil),
	)
	shouter := w.spawn(t, warriorID, 150, 0)
	near := w.spawn(t, warriorTwoID, 150, 200)
	far := w.spawn(t, warriorFarID, 150, 600)

	w.hit(t, shouter, 10)
	if got := shouter.AggroHate(w.player); got != 125 {
		t.Fatalf("shouter hate = %v, want 125 from its own hit only", got)
	}
	if got := near.AggroHate(w.player); got != 100 {
		t.Fatalf("NPC within 300 hate = %v, want 100 from the shout", got)
	}
	if got := far.AggroHate(w.player); got != 0 {
		t.Fatalf("NPC 600 away hate = %v, want none", got)
	}
}
