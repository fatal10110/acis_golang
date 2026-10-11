package ai

import (
	"slices"
	"testing"
	"time"
	"unicode/utf16"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The Warrior children's NPC ids the scenarios spawn.
const (
	holdID        = int32(27102) // WarriorHold
	holdTwoID     = int32(27103) // WarriorHold
	physicalID    = int32(20084) // WarriorPhysicalSpecial
	physicalTwoID = int32(20199) // WarriorPhysicalSpecial
	caughtFrogID  = int32(18319) // WarriorFishingBlock: Caught Frog
	caughtUndine  = int32(18320) // WarriorFishingBlock: Caught Undine
)

// physicalSpecialProbe is the physical special skill the fixture
// WarriorPhysicalSpecial NPCs list: a one-target strike of reach 150.
const physicalSpecialProbe = modelskill.ID(9301)

// teleport puts h at dx, dy from where it stands, on its queue.
func (w *warriorWorld) teleport(t *testing.T, h *npc.Hostile, dx, dy int) {
	t.Helper()
	x, y, z := h.Position()
	done := make(chan struct{})
	if !h.Queue().Post(func() {
		defer close(done)
		script.NPCOf(h).InstantTeleportTo(script.Loc{X: x + dx, Y: y + dy, Z: z})
	}) {
		t.Fatal("the npc's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// onQueue runs fn on h's queue and waits for it.
func (w *warriorWorld) onQueue(t *testing.T, h *npc.Hostile, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !h.Queue().Post(func() {
		defer close(done)
		fn()
	}) {
		t.Fatal("the npc's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// desire returns h's queued desire of kind, and whether it has one.
func desire(h *npc.Hostile, kind actorai.Intention) (actorai.Desire, bool) {
	for _, d := range h.AI().Desires().Snapshot() {
		if d.Kind == kind {
			return d, true
		}
	}
	return actorai.Desire{}, false
}

// inWorld reports whether h is still in the world.
func (w *warriorWorld) inWorld(h *npc.Hostile) bool {
	_, ok := w.srv.State.Object(h.ObjectID())
	return ok
}

// says returns the lines NpcSay frames show from objectID.
func says(frames [][]byte, objectID int32) []string {
	var out []string
	for _, f := range framesOf(frames, serverpackets.OpcodeNpcSay, objectID) {
		// opcode, object id, say type, npc id, then the UTF-16LE text.
		body := f[13:]
		var units []uint16
		for i := 0; i+1 < len(body); i += 2 {
			u := uint16(body[i]) | uint16(body[i+1])<<8
			if u == 0 {
				break
			}
			units = append(units, u)
		}
		out = append(out, string(utf16.Decode(units)))
	}
	return out
}

// TestWarriorHoldWalksBackToItsSpawnThenStands ports WarriorHold's
// no-desire hook: with nothing to do, a WarriorHold NPC standing off its
// spawn point wants to walk back to it with weight 30, and one standing on
// it wants to do nothing for 40 with weight 30. Neither wanders, while a
// plain Warrior beside them does.
func TestWarriorHoldWalksBackToItsSpawnThenStands(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, warriorTemplate(holdID, nil), warriorTemplate(holdTwoID, nil), warriorTemplate(warriorID, nil))
	away := w.spawn(t, holdID, 700, 0)
	home := w.spawn(t, holdTwoID, 700, 300)
	plain := w.spawn(t, warriorID, 700, -300)
	spawnX, spawnY, _ := away.Position()
	w.teleport(t, away, 200, 0)

	// An NPC first idles on its second cycle; the cycle after acts on what
	// its hook queued.
	w.ticks(t, 2)
	if d, ok := desire(away, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location.X != spawnX || d.Location.Y != spawnY {
		t.Fatalf("WarriorHold off its spawn point move desire = %+v (%v), want weight 30 to %d,%d", d, ok, spawnX, spawnY)
	}
	if d, ok := desire(home, actorai.IntentionNothing); !ok || d.Weight != 30 || d.Timer != 40 {
		t.Fatalf("WarriorHold on its spawn point do-nothing desire = %+v (%v), want timer 40, weight 30", d, ok)
	}
	for _, h := range []*npc.Hostile{away, home} {
		if d, ok := desire(h, actorai.IntentionWander); ok {
			t.Fatalf("WarriorHold npc %d wants to wander: %+v", h.NpcID(), d)
		}
	}

	frames := w.ticks(t, 1)
	if got := away.AI().CurrentIntention(); got != actorai.IntentionMoveTo {
		t.Fatalf("WarriorHold off its spawn point intention = %v, want move to", got)
	}
	moves := framesOf(frames, serverpackets.OpcodeMoveToLocation, away.ObjectID())
	if len(moves) != 1 {
		t.Fatalf("WarriorHold off its spawn point MoveToLocation frames = %d, want 1", len(moves))
	}
	if to := moveDestination(moves[0]); to.X != spawnX || to.Y != spawnY {
		t.Fatalf("WarriorHold walks to %v, want its spawn point %d,%d", to, spawnX, spawnY)
	}
	if got := home.AI().CurrentIntention(); got != actorai.IntentionNothing {
		t.Fatalf("WarriorHold on its spawn point intention = %v, want do nothing", got)
	}
	if got := framesOf(frames, serverpackets.OpcodeMoveToLocation, home.ObjectID()); len(got) != 0 {
		t.Fatalf("WarriorHold on its spawn point MoveToLocation frames = %d, want none", len(got))
	}
	if got := plain.AI().CurrentIntention(); got != actorai.IntentionWander {
		t.Fatalf("plain Warrior intention = %v, want wander", got)
	}
}

// physicalSpecialTemplate is a fixture WarriorPhysicalSpecial NPC of id that
// lists physicalSpecialProbe as its physical special skill, in the clan the
// scenario's NPCs share.
func physicalSpecialTemplate(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{npc.SkillTypePhysicalSpecial: {ID: physicalSpecialProbe, Level: 1}}
	tmpl.Clans, tmpl.ClanRange = []string{"bugbears"}, 500
	return tmpl
}

// TestWarriorPhysicalSpecialStrikesItsMostHated ports WarriorPhysicalSpecial's
// attacked and clan attacked hooks, every draw 0: a hit wants the physical
// special skill cast on the attacker with weight 1000000 only once the
// attacker is the most hated creature, so the first hit, which makes it
// so, casts nothing and the second does. A settled clan member that is not
// attacking wants the same cast on the attacker at the first hit.
func TestWarriorPhysicalSpecialStrikesItsMostHated(t *testing.T) {
	t.Parallel()
	w := bootWarriors(t, physicalSpecialTemplate(physicalID), physicalSpecialTemplate(physicalTwoID))
	victim := w.spawn(t, physicalID, 100, 0)
	member := w.spawn(t, physicalTwoID, 100, 100)
	defs := modelskill.NewTable([]modelskill.Definition{{
		ID: physicalSpecialProbe, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, SkillType: "PDAM",
		CastRange: 150, HitTime: 500, StaticHitTime: true, StaticReuse: true, Power: 1,
	}})
	for _, h := range []*npc.Hostile{victim, member} {
		w.srv.InstallNPCCastSeam(t, h, defs)
	}
	w.ticks(t, 8)
	if member.LifeTime() <= 7 {
		t.Fatalf("clan member life time = %d, want past 7", member.LifeTime())
	}

	castOn := func(h *npc.Hostile) (actorai.Desire, bool) {
		d, ok := desire(h, actorai.IntentionCast)
		return d, ok && d.Skill.ID == physicalSpecialProbe && d.FinalTarget != nil && d.FinalTarget.ObjectID() == w.player.ObjectID()
	}
	w.hit(t, victim, 10)
	if d, ok := castOn(victim); ok {
		t.Fatalf("victim wants %+v after the hit that made the attacker its most hated, want no cast", d)
	}
	if d, ok := castOn(member); !ok || d.Weight != 1000000 {
		t.Fatalf("settled clan member cast desire = %+v (%v), want the physical special on the attacker, weight 1000000", d, ok)
	}

	w.hit(t, victim, 10)
	if d, ok := castOn(victim); !ok || d.Weight != 1000000 {
		t.Fatalf("victim cast desire after a hit by its most hated = %+v (%v), want the physical special, weight 1000000", d, ok)
	}
}

// fishingTemplate is the fixture caught monster of id.
func fishingTemplate(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.Name, tmpl.Level = "Caught", 20
	return tmpl
}

// TestFishingMonsterTurnsOnItsFisher ports WarriorFishingBlock, every draw
// 0. A caught monster spawned for the fisher, as the fishing catch spawns
// it, wants to attack the fisher with weight 2000 and taunts them by name
// with the first of its creation lines (Caught Undine: 1010409). A hit
// makes it say the first of its hit lines (1010412), and its death the
// first of its dying lines (1010415). One spawned for nobody leaves at
// once. Its 50 s check keeps it while it attacks and deletes it once it
// has stopped.
func TestFishingMonsterTurnsOnItsFisher(t *testing.T) {
	t.Parallel()
	undine, frog := fishingTemplate(caughtUndine), fishingTemplate(caughtFrogID)
	w := bootWarriors(t, undine, frog)

	stray := w.spawn(t, caughtFrogID, 100, 0)
	if w.inWorld(stray) || len(w.liveNPCs(caughtFrogID)) != 0 {
		t.Fatal("a caught monster spawned for nobody stays in the world")
	}

	if err := w.srv.NpcSpawns.SpawnSummoned(undine, w.at.X+50, w.at.Y, w.at.Z, 0, w.player); err != nil {
		t.Fatalf("spawn the caught monster: %v", err)
	}
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	monster := w.liveNPCs(caughtUndine)
	if len(monster) != 1 {
		t.Fatalf("caught monsters in the world = %d, want 1", len(monster))
	}
	m := monster[0]
	if got := says(frames, m.ObjectID()); len(got) != 1 || got[0] != "Kaak! Hunter, What are you doing now?" {
		t.Fatalf("caught monster lines at creation = %q, want the 1010409 taunt naming the fisher", got)
	}
	if got := m.AggroHate(w.player); got != 2000 {
		t.Fatalf("caught monster hate of its fisher = %v, want 2000", got)
	}
	if got := m.AI().CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("caught monster intention = %v, want attack", got)
	}

	w.srv.Advance(t, 50*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if !w.inWorld(m) {
		t.Fatal("the 50 s check deleted a caught monster that attacks")
	}

	w.onQueue(t, m, func() { script.NPCOf(m).RemoveAllAttackDesire() })
	w.ticks(t, 1)
	if got := m.AI().CurrentIntention(); got == actorai.IntentionAttack || got == actorai.IntentionCast {
		t.Fatalf("caught monster intention after dropping its attack = %v, want neither attack nor cast", got)
	}
	w.srv.Advance(t, 50*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if w.inWorld(m) || len(w.liveNPCs(caughtUndine)) != 0 {
		t.Fatal("the 50 s check kept a caught monster that no longer attacks, or it came back")
	}
}

// TestFishingMonsterLinesOnHitAndDeath shows the caught monster's hit and
// dying lines, every draw 0. Neither names anybody: the reference passes
// the attacker's or killer's name to lines that have no place for it.
func TestFishingMonsterLinesOnHitAndDeath(t *testing.T) {
	t.Parallel()
	undine := fishingTemplate(caughtUndine)
	w := bootWarriors(t, undine)
	if err := w.srv.NpcSpawns.SpawnSummoned(undine, w.at.X+50, w.at.Y, w.at.Z, 0, w.player); err != nil {
		t.Fatalf("spawn the caught monster: %v", err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	m := w.liveNPCs(caughtUndine)[0]

	w.hit(t, m, 10)
	if got := says(w.srv.ReadQueued(t, w.srv.Client), m.ObjectID()); len(got) != 1 || got[0] != "You made me angry!" {
		t.Fatalf("caught monster lines on a hit = %q, want the 1010412 line", got)
	}

	w.hit(t, m, 10000)
	w.srv.Advance(t, 3*time.Second)
	var frames [][]byte
	for range 3 {
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	}
	// The killing hit raises the attacked hook first, before the HP falls.
	want := []string{"You made me angry!", "I'll get you for this!"}
	if got := says(frames, m.ObjectID()); !slices.Equal(got, want) {
		t.Fatalf("caught monster lines on the killing hit and its death = %q, want %q (1010412, 1010415)", got, want)
	}
}
