package ai

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/queenant"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Queen Ant, her larva and her nurse ant.
const (
	queenAntID   = int32(29001)
	queenLarvaID = int32(29002)
	queenNurseID = int32(29003)
)

// The skills the Queen Ant's family casts: her strikes and self cast, the
// two heals of her nurses, the second a minute long to cast.
const (
	queenSelfCast  = modelskill.ID(4017)
	queenStrike    = modelskill.ID(4018)
	queenFarStrike = modelskill.ID(4019)
	queenHeal      = modelskill.ID(4020)
	queenHeal2     = modelskill.ID(4024)
)

// queenAntSkills are fixture definitions of the skills her family casts.
func queenAntSkills() *modelskill.Table {
	heal := func(id modelskill.ID, hitTime int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "HEAL", CastRange: 600, HitTime: hitTime, StaticHitTime: true, StaticReuse: true, Power: 1,
		}
	}
	return modelskill.NewTable([]modelskill.Definition{
		npcSkill(queenSelfCast, 1, true), npcSkill(queenStrike, 1, true), npcSkill(queenFarStrike, 1, true),
		heal(queenHeal, 500), heal(queenHeal2, 60000),
	})
}

// bootQueenAnt boots the Queen Ant's family beside a level 20 character:
// the queen leads two nurse ants of weight point 1, every script draw
// being draw's answer. The scenarios of testdata/scenarios cover her fight,
// her territory and her death; these tests cover what the scenario format
// cannot place or show: her privates and larva, the teleport of the players
// on her ground, and the nurses' and larva's own hooks.
func bootQueenAnt(t *testing.T, draw func(n int) int) *warriorWorld {
	t.Helper()
	queen := warriorTemplate(queenAntID, nil)
	queen.Type, queen.Name = "GrandBoss", "Queen Ant"
	queen.Privates = []npc.PrivateEntry{
		{NpcID: int(queenNurseID), Weight: 1},
		{NpcID: int(queenNurseID), Weight: 1},
	}
	larva := warriorTemplate(queenLarvaID, nil)
	larva.CanMove = false
	nurse := warriorTemplate(queenNurseID, nil)
	kinds := map[int32]script.NPCKind{queenAntID: script.KindHostile, queenLarvaID: script.KindHostile, queenNurseID: script.KindHostile}
	list := []script.Listing{
		{Path: "script.ai.individual.DefaultNpc"},
		{Path: "script.ai.boss.queenant.QueenAnt"},
		{Path: "script.ai.boss.queenant.QueenAntPrivateAntLarva"},
		{Path: "script.ai.boss.queenant.QueenAntPrivateNurseAnt"},
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":                 defaultnpc.New,
		"script.ai.boss.queenant.QueenAnt":                queenant.QueenAnt,
		"script.ai.boss.queenant.QueenAntPrivateAntLarva": queenant.Larva,
		"script.ai.boss.queenant.QueenAntPrivateNurseAnt": queenant.Nurse,
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{queen, larva, nurse})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNpcSpawnSkills(queenAntSkills()),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(draw),
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

// lowestDraw and highestDraw answer every script draw with the lowest and
// the highest value its bound allows.
func lowestDraw(int) int    { return 0 }
func highestDraw(n int) int { return n - 1 }

// queenFamily returns the Queen Ant's nurses and her larva, each sorted by
// object id.
func queenFamily(t *testing.T, queen *npc.Hostile) (nurses, larvae []*npc.Hostile) {
	t.Helper()
	for _, p := range privatesOf(t, queen, 3) {
		switch int32(p.NpcID()) {
		case queenNurseID:
			nurses = append(nurses, p)
		case queenLarvaID:
			larvae = append(larvae, p)
		default:
			t.Fatalf("the queen leads npc %d", p.NpcID())
		}
	}
	if len(nurses) != 2 || len(larvae) != 1 {
		t.Fatalf("the queen leads %d nurses and %d larvae, want 2 and 1", len(nurses), len(larvae))
	}
	return nurses, larvae
}

// TestQueenAntSendsPlayersOnHerGroundAway covers the teleport of her
// created hook: the queen appearing 100 from the character, inside the 200
// of her territory, sends it up to 200 away on each axis from one of three
// places: the first on a draw under 33, the second on a draw of 33 or more
// then one under 50, the third otherwise.
func TestQueenAntSendsPlayersOnHerGroundAway(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		draw func(int) int
		to   location.Location
	}{
		{"draw 0", lowestDraw, location.Location{X: -19480, Y: 187344, Z: -5600}},
		{"draw 40", func(n int) int { return min(40, n-1) }, location.Location{X: -17928, Y: 180912, Z: -5520}},
		{"draw 99", highestDraw, location.Location{X: -23808, Y: 182368, Z: -5600}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootQueenAnt(t, c.draw)
			w.spawn(t, queenAntID, 100, 0)
			x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
			if abs(x-c.to.X) > 200 || abs(y-c.to.Y) > 200 {
				t.Fatalf("the character stands at %d,%d,%d, want within 200 of %v", x, y, z, c.to)
			}
		})
	}
}

// TestQueenAntAppearsWithHerFamily covers the rest of her created hook:
// appearing 400 from the character, out of her territory, she leaves it
// where it stands, plays BS01_A, takes weight point 10, leads the two
// nurses her template declares and her larva, placed at -21600, 179482,
// -5846 facing a draw of 360, here the highest (359).
func TestQueenAntAppearsWithHerFamily(t *testing.T) {
	t.Parallel()
	w := bootQueenAnt(t, highestDraw)
	live, err := w.srv.NpcSpawns.AddSpawn(queenAntID, w.at.X+400, w.at.Y, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	queen := live.(*npc.Hostile)

	qx, qy, qz := queen.Position()
	wantBaiumFrames(t, "appear", frames, map[int32]string{queen.ObjectID(): "queen"},
		fmt.Sprintf("S PlaySound type=1 file=BS01_A bind=1 obj=queen loc=%d,%d,%d delay=0", qx, qy, qz))
	if x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID()); (location.Location{X: x, Y: y, Z: z}) != w.at {
		t.Fatalf("the character stands at %d,%d,%d, want %v where it entered", x, y, z, w.at)
	}
	if got := queen.Scratch().Int(npc.IntWeightPoint); got != 10 {
		t.Fatalf("queen weight point = %d, want 10", got)
	}
	nurses, larvae := queenFamily(t, queen)
	for _, n := range nurses {
		if got := n.Scratch().Int(npc.IntWeightPoint); got != 1 {
			t.Fatalf("nurse %d weight point = %d, want 1", n.ObjectID(), got)
		}
	}
	if got, want := placeOf(larvae[0]), "-21600,179482,-5846 heading 359"; got != want {
		t.Fatalf("larva at %s, want %s", got, want)
	}
}

// TestQueenAntWandersSocially covers her 10 s timer: while she wanders, a
// draw under 30 queues social action 3 (a draw of 0 on the coin) or 4 for
// 1666 at weight 30; while she does not, nothing.
func TestQueenAntWandersSocially(t *testing.T) {
	t.Parallel()
	w := bootQueenAnt(t, lowestDraw)
	queen := w.spawn(t, queenAntID, 400, 0)

	w.srv.Advance(t, 10*time.Second)
	if d, ok := desire(queen, actorai.IntentionSocial); ok {
		t.Fatalf("idle queen social desire = %+v, want none", d)
	}

	w.onQueue(t, queen, func() { script.NPCOf(queen).AddWanderDesire(10, 1) })
	w.srv.AdvanceUntil(t, "the queen's wander", func() bool {
		cycle(t, w.srv)
		return queen.AI().CurrentIntention() == actorai.IntentionWander
	})
	w.srv.Advance(t, 10*time.Second)
	d, ok := desire(queen, actorai.IntentionSocial)
	if !ok || d.Weight != 30 || d.ItemObjectID != 3 || d.Timer != 1666 {
		t.Fatalf("wandering queen social desire = %+v, %v, want action 3 for 1666 at weight 30", d, ok)
	}
}

// TestQueenAntBringsHerPrivatesBack covers her party-died hook: a dying
// nurse is asked back after 10 s, the dying larva after 280 s plus a draw
// of 40, here the highest (39): 319 s.
func TestQueenAntBringsHerPrivatesBack(t *testing.T) {
	t.Parallel()
	w := bootQueenAnt(t, highestDraw)
	queen := w.spawn(t, queenAntID, 400, 0)
	nurses, larvae := queenFamily(t, queen)

	armed := map[string]bool{}
	for _, c := range []struct {
		name  string
		npc   *npc.Hostile
		delay time.Duration
	}{{"nurse", nurses[0], 10 * time.Second}, {"larva", larvae[0], 319 * time.Second}} {
		before := time.Now()
		w.kill(t, c.npc)
		after := time.Now()
		var fresh []string
		for i := 1; i <= 64; i++ {
			if key := fmt.Sprintf("private#%d", i); w.srv.NpcRespawns.Tracked(key) && !armed[key] {
				fresh = append(fresh, key)
			}
		}
		if len(fresh) != 1 {
			t.Fatalf("%s: respawns newly armed = %q, want one", c.name, fresh)
		}
		armed[fresh[0]] = true
		deadline, ok := w.srv.NpcRespawns.Deadline(fresh[0])
		if !ok {
			t.Fatalf("%s: no deadline for the armed respawn %s", c.name, fresh[0])
		}
		if deadline.Before(before.Add(c.delay)) || deadline.After(after.Add(c.delay)) {
			t.Fatalf("%s respawn due %s after the death, want %s", c.name, deadline.Sub(before), c.delay)
		}
	}
}

// TestQueenAntPrivatesLeaveWithoutLiveMaster covers the larva's and the
// nurse's 5 s timer: each stays while its master lives, and leaves the
// world at the next tick once it has none; a larva or nurse that appears
// with no master leaves 5 s after it appeared.
func TestQueenAntPrivatesLeaveWithoutLiveMaster(t *testing.T) {
	t.Parallel()
	w := bootQueenAnt(t, highestDraw)
	queen := w.spawn(t, queenAntID, 400, 0)
	nurses, larvae := queenFamily(t, queen)
	family := append(nurses, larvae...)

	w.srv.Advance(t, 5*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	for _, p := range family {
		if !w.inWorld(p) {
			t.Fatalf("npc %d left the world while the queen lives", p.NpcID())
		}
	}
	w.kill(t, queen)
	w.srv.Advance(t, 4*time.Second)
	for _, p := range family {
		if !w.inWorld(p) {
			t.Fatalf("npc %d left the world before its 5 s tick", p.NpcID())
		}
	}
	w.srv.Advance(t, time.Second)
	for _, p := range family {
		if w.inWorld(p) {
			t.Fatalf("npc %d stayed in the world without a live master", p.NpcID())
		}
	}

	for _, id := range []int32{queenLarvaID, queenNurseID} {
		alone := w.spawn(t, id, 300, 0)
		w.srv.Advance(t, 4*time.Second)
		if !w.inWorld(alone) {
			t.Fatalf("lone npc %d left the world before its 5 s tick", id)
		}
		w.srv.Advance(t, time.Second)
		if w.inWorld(alone) {
			t.Fatalf("lone npc %d stayed in the world", id)
		}
	}
}

// TestQueenAntNursesFollowOrWander covers the nurse's no-desire hook: with
// nothing to do it follows its live queen at weight 20; once she is dead it
// wanders for 40 at weight 20.
func TestQueenAntNursesFollowOrWander(t *testing.T) {
	t.Parallel()
	w := bootQueenAnt(t, highestDraw)
	queen := w.spawn(t, queenAntID, 400, 0)
	nurses, _ := queenFamily(t, queen)

	w.ticks(t, 2)
	for _, n := range nurses {
		d, ok := desire(n, actorai.IntentionFollow)
		if !ok || d.Weight != 20 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != queen.ObjectID() {
			t.Fatalf("nurse %d follow desire = %+v, %v, want the queen at weight 20", n.ObjectID(), d, ok)
		}
	}

	w.kill(t, queen)
	for _, n := range nurses {
		w.onQueue(t, n, func() { n.AI().Desires().Clear() })
	}
	w.ticks(t, 2)
	for _, n := range nurses {
		if d, ok := desire(n, actorai.IntentionWander); !ok || d.Weight != 20 || d.Timer != 40 {
			t.Fatalf("nurse %d wander desire = %+v, %v, want timer 40 at weight 20", n.ObjectID(), d, ok)
		}
	}
}

// TestQueenAntNursesHealOnPartyCalls covers the nurse's party-attacked
// hook: a hit on the queen makes each nurse want her heal 4020 on her at
// 1000000, walking to her; a hit on the larva, 4020 and 4024 on it at 100.
// A hit on a nurse wants nothing of either nurse. With the queen over 2500
// away, a nurse casting with the larva as its top desire target ignores a
// hit on her; an idle one still wants her heal.
func TestQueenAntNursesHealOnPartyCalls(t *testing.T) {
	t.Parallel()
	w := bootQueenAnt(t, highestDraw)
	queen := w.spawn(t, queenAntID, 400, 0)
	nurses, larvae := queenFamily(t, queen)
	larva := larvae[0]
	clear := func() {
		for _, n := range nurses {
			w.onQueue(t, n, func() { n.AI().Desires().Clear() })
		}
	}

	w.hit(t, queen, 1)
	for _, n := range nurses {
		wantCast(t, "nurse", n, queenHeal, queen, 1000000, true)
	}
	clear()

	w.hit(t, larva, 1)
	for _, n := range nurses {
		wantCast(t, "nurse", n, queenHeal, larva, 100, true)
		wantCast(t, "nurse", n, queenHeal2, larva, 100, true)
	}
	clear()

	w.hit(t, nurses[0], 1)
	for _, n := range nurses {
		noCast(t, "nurse", n, queenHeal)
		noCast(t, "nurse", n, queenHeal2)
	}
	clear()

	casting := nurses[1]
	w.onQueue(t, casting, func() {
		script.NPCOf(casting).AddCastDesire(script.PlayerOf(w.player), modelskill.Ref{ID: queenHeal2, Level: 1}, 100)
	})
	w.srv.AdvanceUntil(t, "the nurse's cast", func() bool {
		cycle(t, w.srv)
		return casting.AI().CurrentIntention() == actorai.IntentionCast
	})
	w.onQueue(t, casting, func() { script.NPCOf(casting).SetTopDesireTarget(script.NPCOf(larva)) })
	w.teleport(t, queen, 3000, 0)
	w.hit(t, queen, 1)
	wantCast(t, "idle nurse", nurses[0], queenHeal, queen, 1000000, true)
	noCast(t, "casting nurse", casting, queenHeal)
}

// TestQueenAntAnswersAPartyCall covers her party-attacked hook: the level
// 20 character hitting a nurse 400 from her, on draws of 0, makes her want
// strike 4018 on it from where she stands (over 150 away, a draw under 5
// then one under 80) and to attack it from where she stands by the damage
// over her hit points, over 0.05, times the damage, the nurse's weight
// point 1 and 1000.
func TestQueenAntAnswersAPartyCall(t *testing.T) {
	t.Parallel()
	w := bootQueenAnt(t, lowestDraw)
	queen := w.spawn(t, queenAntID, 400, 0)
	nurses, _ := queenFamily(t, queen)

	w.hit(t, nurses[0], siegeTwentieth)
	w.wantHeldCast(t, "queen", queen, queenStrike, 1000000)
	maxHP := float64(script.NPCOf(queen).MaxHP())
	w.holdsAttack(t, "queen", queen, float64(int32(float64(siegeTwentieth)/maxHP/0.05*siegeTwentieth*1*1000)))
}

// TestQueenAntGoesHomeOutOfTerritory covers her out-of-territory hook: a
// walk taking her out of the 200 of her territory ends with her arrival
// outside it, and she goes back to her nest at -21610, 181594, -5734,
// forgetting her targets. The scenario format moves an NPC by a jump,
// which is no arrival.
func TestQueenAntGoesHomeOutOfTerritory(t *testing.T) {
	t.Parallel()
	w := bootQueenAnt(t, highestDraw)
	queen := w.spawn(t, queenAntID, 400, 0)
	w.onQueue(t, queen, func() { script.NPCOf(queen).AddAttackDesire(script.PlayerOf(w.player), 1) })
	if _, ok := attackDesireOn(queen, w.player.ObjectID()); !ok {
		t.Fatal("setup: the queen holds no attack desire on the character")
	}

	(&ootRoamWorld{srv: w.srv}).walkTo(t, queen, location.Location{X: w.at.X + 700, Y: w.at.Y, Z: w.at.Z})
	w.srv.Settle(t)
	if x, y, z := queen.Position(); x != -21610 || y != 181594 || z != -5734 {
		t.Fatalf("queen at %d,%d,%d, want her nest -21610,181594,-5734", x, y, z)
	}
	if d, ok := attackDesireOn(queen, w.player.ObjectID()); ok {
		t.Fatalf("queen attack desire at her nest = %+v, want none", d)
	}
}
