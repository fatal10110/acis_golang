package combat

import (
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// summonKillerID is the fixture monster that kills the owner's servitor.
const summonKillerID = int32(20052)

// summonKillScene is a character, its servitor and a monster beside them
// that swings once, deterministically, at what it is told to.
type summonKillScene struct {
	srv      *gameservertest.Server
	owner    attackable.Combatant
	servitor *summon.Actor
	killer   *gameservertest.AttackingHostile
}

// bootSummonKillScene boots a character that has called up a servitor of hp,
// and a monster of tmpl bound to the script bind builds, dx east of the
// servitor.
func bootSummonKillScene(t *testing.T, hp int, bind func() script.Script, tmpl *npc.Template, dx int) *summonKillScene {
	t.Helper()
	servitorTmpl := pkServitorTemplate()
	servitorTmpl.HPMax, servitorTmpl.Skills = float64(hp), nil // no skill to answer with
	const path = "ai.SummonKiller"
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, pkServitorSkills())),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{servitorTmpl})),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{summonKillerID: script.KindHostile},
			[]script.Listing{{Path: path}}, script.Catalog{path: bind}),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, pkServitorSummonSkill, 1)
	startInWorld(t, c)
	c.Send(encodeRequestMagicSkillUse(pkServitorSummonSkill, false, false))
	var servitor *summon.Actor
	srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := srv.State.Summon(objID)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	drainUntilQuiet(t, c)

	sx, sy, sz := servitor.Position()
	tmpl.ID, tmpl.TemplateID = int(summonKillerID), int(summonKillerID)
	killer := srv.SpawnAttackingHostileNPCTemplate(t, tmpl, location.Location{X: sx + dx, Y: sy, Z: sz})
	drainUntilQuiet(t, c)
	return &summonKillScene{srv: srv, owner: livePlayer(t, srv, objID).(attackable.Combatant), servitor: servitor, killer: killer}
}

// summonKillerTemplate is a fixture monster hitting hard enough to kill a
// servitor of 500 HP in one swing, aware of players within aggroRange, that
// stands where it is spawned.
func summonKillerTemplate(ai npc.AIParams, aggroRange int) *npc.Template {
	tmpl := gameservertest.AttackingHostileTemplate()
	tmpl.PAtk = 100_000
	tmpl.AIParams, tmpl.AggroRange = ai, aggroRange
	return tmpl
}

// monsterAI binds MonsterAI's hooks to the killer.
func monsterAI() script.Script {
	s := monster.New()
	s.NPCs = []int32{summonKillerID}
	return s
}

// warriorBehavior binds WarriorBehavior's hooks to the killer.
func warriorBehavior() script.Script {
	s := monsterbehavior.WarriorBehavior()
	s.NPCs = []int32{summonKillerID}
	return s
}

// kill has the monster swing once at the servitor and waits for the swing to
// finish and the hooks that follow to run.
func (s *summonKillScene) kill(t *testing.T) {
	t.Helper()
	s.killer.DoAttack(t, s.servitor)
	s.srv.Settle(t)
}

// attackDesire returns the monster's queued attack desire on the owner.
func (s *summonKillScene) attackDesire(t *testing.T) (weight float64, held, ok bool) {
	t.Helper()
	for _, d := range s.killer.AI().Desires().Snapshot() {
		if d.Kind == actorai.IntentionAttack && d.FinalTarget != nil && d.FinalTarget.ObjectID() == s.owner.ObjectID() {
			return d.Weight, !d.MoveToTarget, true
		}
	}
	return 0, false, false
}

// TestMonsterAIAttackFinishedTurnsOnTheOwnerOfAKilledSummon ports MonsterAI's
// attack-finished hook: a monster whose swing killed a summon wants to attack
// the summon's owner with weight 500; a swing that leaves the summon alive
// wants nothing of its owner.
func TestMonsterAIAttackFinishedTurnsOnTheOwnerOfAKilledSummon(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		hp   int // the servitor's
		want float64
	}{{"the swing kills", 500, 500}, {"the swing leaves it alive", 100_000_000, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			s := bootSummonKillScene(t, row.hp, monsterAI, summonKillerTemplate(nil, 0), 30)
			s.kill(t)
			if dead := s.servitor.Dead(); dead != (row.want > 0) {
				t.Fatalf("setup: the servitor is dead = %v", dead)
			}
			if got, _, ok := s.attackDesire(t); got != row.want || ok != (row.want > 0) {
				t.Fatalf("attack desire on the owner = %v (%v), want %v", got, ok, row.want)
			}
		})
	}
}

// TestWarriorBehaviorAttackFinishedTurnsOnTheOwnerOfAKilledSummon ports the
// attack-finished hook of WarriorBehavior, for a swing that killed a summon.
// By default (MovingAttack 1) the monster wants to attack the summon's owner
// with weight 5000, moving to it; when it is rooted and its top desire
// target is more than 40 away it drops that target and wants the owner once
// more, so 10000. With MovingAttack 0 it wants the owner, held in place, with
// weight 5000 when it may attack the owner on its own, and nothing
// otherwise.
func TestWarriorBehaviorAttackFinishedTurnsOnTheOwnerOfAKilledSummon(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		ai       npc.AIParams
		rooted   bool
		top      bool // the servitor was the monster's top desire target
		dx       int  // the monster's offset from the servitor
		aggro    int
		want     float64
		wantHeld bool
		// topOwner makes the owner the top desire target instead, and seed is
		// an attack desire the monster holds on the owner beforehand.
		topOwner bool
		seed     float64
	}{
		{"moving attack", nil, false, true, 30, 1000, 5000, false, false, 0},
		{"moving attack, rooted and 41 from the top target", nil, true, true, 41, 1000, 10000, false, false, 0},
		{"moving attack, rooted and 40 from the top target", nil, true, true, 40, 1000, 5000, false, false, 0},
		{"moving attack, rooted with no top target", nil, true, false, 100, 1000, 5000, false, false, 0},
		{"held attack, the owner in aggro range", npc.AIParams{"MovingAttack": "0"}, false, true, 30, 1000, 5000, true, false, 0},
		{"held attack, the owner out of aggro range", npc.AIParams{"MovingAttack": "0"}, false, true, 30, 10, 0, false, false, 0},
		{"moving attack, rooted, the owner on top and not attackable", nil, true, false, 100, 10, 5000, false, true, 0},
		{"held attack, the owner on top and not attackable", npc.AIParams{"MovingAttack": "0"}, false, false, 30, 10, 0, false, true, 300},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			s := bootSummonKillScene(t, 500, warriorBehavior, summonKillerTemplate(row.ai, row.aggro), row.dx)
			if row.rooted {
				landEffect(t, s.killer, "Root")
			}
			if row.top {
				s.killer.AI().SetTopDesireTarget(s.servitor)
			}
			if row.topOwner {
				s.killer.AI().SetTopDesireTarget(s.owner)
			}
			if row.seed > 0 {
				s.killer.AI().AddAttackDesireHold(s.owner, row.seed)
			}
			s.kill(t)
			if !s.servitor.Dead() {
				t.Fatal("setup: the servitor is alive")
			}
			got, held, ok := s.attackDesire(t)
			if got != row.want || ok != (row.want > 0) || (ok && held != row.wantHeld) {
				t.Fatalf("attack desire on the owner = %v (held %v, %v), want %v (held %v)", got, held, ok, row.want, row.wantHeld)
			}
		})
	}
}

// TestWarriorBehaviorAttackFinishedJumpsToTheOwnerWhenStuck ports the
// teleport of WarriorBehavior's attack-finished hook: a monster that killed
// a summon while its top desire target was the owner, after more than 10 path
// failures and with HP below full, jumps to the owner.
func TestWarriorBehaviorAttackFinishedJumpsToTheOwnerWhenStuck(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		failures int
		hurt     bool
		topOwner bool
		jumps    bool
	}{
		{"11 failures, hurt, the owner on top", 11, true, true, true},
		{"10 failures", 10, true, true, false},
		{"full HP", 11, false, true, false},
		{"the summon on top", 11, true, false, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			s := bootSummonKillScene(t, 500, warriorBehavior, summonKillerTemplate(nil, 1000), 100)
			if row.hurt {
				s.killer.SetHP(0.5 * float64(s.killer.MaxHP()))
			}
			for range row.failures {
				s.killer.AddGeoPathFailCount()
			}
			if row.topOwner {
				s.killer.AI().SetTopDesireTarget(s.owner)
			} else {
				s.killer.AI().SetTopDesireTarget(s.servitor)
			}
			s.kill(t)
			if !s.servitor.Dead() {
				t.Fatal("setup: the servitor is alive")
			}
			ox, oy, oz := s.owner.Position()
			x, y, z := s.killer.Position()
			if got, owner := (location.Location{X: x, Y: y, Z: z}), (location.Location{X: ox, Y: oy, Z: oz}); (got == owner) != row.jumps {
				t.Fatalf("the monster stands at %v, the owner at %v: jumped %v, want %v", got, owner, got == owner, row.jumps)
			}
		})
	}
}

// TestWarriorBehaviorAttackFinishedLeavesTheOwnerAloneWhileTheSummonLives
// pins the other half of the hook above: a swing that leaves the summon alive
// makes the monster want nothing of the owner, with either MovingAttack.
func TestWarriorBehaviorAttackFinishedLeavesTheOwnerAloneWhileTheSummonLives(t *testing.T) {
	t.Parallel()
	for _, ai := range []npc.AIParams{nil, {"MovingAttack": "0"}} {
		t.Run(ai["MovingAttack"], func(t *testing.T) {
			t.Parallel()
			s := bootSummonKillScene(t, 100_000_000, warriorBehavior, summonKillerTemplate(ai, 1000), 30)
			s.kill(t)
			if s.servitor.Dead() {
				t.Fatal("setup: the servitor is dead")
			}
			if got, _, ok := s.attackDesire(t); ok {
				t.Fatalf("attack desire on the owner = %v, want none", got)
			}
		})
	}
}
