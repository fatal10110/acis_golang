package ai

import (
	"slices"
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
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// raidPrivateID is a RaidPrivateStandard NPC (Turek Mercenary Warrior). The
// scenarios give it a fixture template and make it the private of a party
// leader, which creates its privates as it spawns: the raid bosses that
// lead it in the reference are not ported yet.
const raidPrivateID = int32(25006)

// raidCurseID is the skill a raid private casts on a creature more than 8
// levels above it.
const raidCurseID = modelskill.ID(4515)

// raidScripts are partyScripts with RaidPrivateStandard, in scripts.xml
// order after MonsterAI.
func raidScripts() ([]script.Listing, script.Catalog) {
	list, catalog := partyScripts()
	const path = "script.ai.individual.Monster.RaidPrivate.RaidPrivateStandard"
	catalog[path] = raid.PrivateStandard
	at := slices.IndexFunc(list, func(l script.Listing) bool {
		return l.Path == "script.ai.individual.Monster.MonsterAI"
	}) + 1
	return slices.Insert(list, at, script.Listing{Path: path}), catalog
}

// raidParty boots the character (level 20) beside a party leader of
// leaderLevel leading one raid private of privateLevel, neither of which
// walks, the leader 400 away from the character. It returns the leader and
// its private.
func raidParty(t *testing.T, leaderLevel, privateLevel int) (*warriorWorld, *npc.Hostile, *npc.Hostile) {
	t.Helper()
	leaderTmpl := warriorTemplate(leaderID, nil)
	leaderTmpl.Level = leaderLevel
	leaderTmpl.Privates = []npc.PrivateEntry{{NpcID: int(raidPrivateID), Weight: 1, RespawnDelay: 240 * time.Second}}
	privateTmpl := warriorTemplate(raidPrivateID, nil)
	privateTmpl.Name = "Raid Private"
	privateTmpl.Level = privateLevel
	leaderTmpl.CanMove, privateTmpl.CanMove = false, false
	w := bootRaid(t, leaderTmpl, privateTmpl)
	leader := w.spawn(t, leaderID, 400, 0)
	return w, leader, privatesOf(t, leader, 1)[0]
}

// bootRaid is bootParty with RaidPrivateStandard registered and no
// npcmakers.
func bootRaid(t *testing.T, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	list, catalog := raidScripts()
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

// distance2D returns how far apart a and b stand on the ground plane.
func distance2D(a, b *npc.Hostile) float64 {
	return positionOf(a).Distance2D(positionOf(b))
}

// TestRaidPrivateFollowsItsLiveMaster ports RaidPrivateStandard's
// no-desire hook (#2162): with nothing to do, a raid private with a live
// master wants to follow it with weight 5 and does not wander. With no
// master, it wants to wander for 10 with weight 5 instead. A dead master
// never leaves it masterless (its death deletes the private), so the
// scenario drops the link by hand to reach that branch.
func TestRaidPrivateFollowsItsLiveMaster(t *testing.T) {
	t.Parallel()
	w, leader, private := raidParty(t, 1, 1)

	w.ticks(t, 2)
	d, ok := desire(private, actorai.IntentionFollow)
	if !ok || d.Weight != 5 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != leader.ObjectID() {
		t.Fatalf("raid private follow desire = %+v (%v), want its master with weight 5", d, ok)
	}
	if d, ok := desire(private, actorai.IntentionWander); ok {
		t.Fatalf("raid private with a live master wants to wander: %+v", d)
	}

	w.onQueue(t, private, func() {
		private.SetMaster(nil)
		private.AI().Desires().Clear()
	})
	w.ticks(t, 2)
	if d, ok := desire(private, actorai.IntentionWander); !ok || d.Timer != 10 || d.Weight != 5 {
		t.Fatalf("masterless raid private wander desire = %+v (%v), want timer 10, weight 5", d, ok)
	}
	if d, ok := desire(private, actorai.IntentionFollow); ok {
		t.Fatalf("masterless raid private wants to follow: %+v", d)
	}
}

// TestRaidPrivateLivesOnlyWithItsMaster ports the created and party-died
// hooks: a raid private spawned with no master leaves the world at once,
// and one whose master dies leaves it with its master's death, sending
// DeleteObject.
func TestRaidPrivateLivesOnlyWithItsMaster(t *testing.T) {
	t.Parallel()
	w, leader, private := raidParty(t, 1, 1)

	alone := w.spawn(t, raidPrivateID, -400, 0)
	if w.inWorld(alone) {
		t.Fatal("a raid private spawned with no master stayed in the world")
	}
	if !w.inWorld(private) {
		t.Fatal("the raid private of a live master left the world")
	}

	w.onQueue(t, leader, func() { script.NPCOf(leader).DoDie(nil) })
	frames := w.srv.ReadQueued(t, w.srv.Client)
	if w.inWorld(private) {
		t.Fatal("the raid private stayed in the world once its master died")
	}
	if got := framesOf(frames, serverpackets.OpcodeDeleteObject, private.ObjectID()); len(got) != 1 {
		t.Fatalf("DeleteObject frames for the raid private = %d, want 1", len(got))
	}
}

// TestRaidPrivateReturnsToItsMaster ports the 1001 timer: 2 s after it is
// created, then every 60 s, a raid private standing more than 500 from its
// live master, and not attacking, is teleported onto its master.
func TestRaidPrivateReturnsToItsMaster(t *testing.T) {
	t.Parallel()
	w, leader, private := raidParty(t, 1, 1)

	w.teleport(t, private, 700, 0)
	if got := distance2D(private, leader); got <= 500 {
		t.Fatalf("raid private stands %v from its master before the first check, want over 500", got)
	}
	w.srv.Advance(t, 2*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if got, want := positionOf(private), positionOf(leader); got != want {
		t.Fatalf("raid private at %v after its first check, want on its master at %v", got, want)
	}

	w.teleport(t, private, 700, 0)
	w.srv.Advance(t, 59*time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := distance2D(private, leader); got <= 500 {
		t.Fatalf("raid private moved back %v from its master before its next check", got)
	}
	w.srv.Advance(t, time.Second)
	w.srv.ReadQueued(t, w.srv.Client)
	if got, want := positionOf(private), positionOf(leader); got != want {
		t.Fatalf("raid private at %v after its 60 s check, want on its master at %v", got, want)
	}
}

// TestRaidPrivateHatesForItsParty ports the party-attacked hook's hate: a
// hit of 10 by a character at most 8 levels above the master makes the raid
// private want to attack it with damage / (level + 7) * 20000, truncated,
// whichever party member was hit.
func TestRaidPrivateHatesForItsParty(t *testing.T) {
	t.Parallel()
	w, leader, private := raidParty(t, 20, 20)

	w.hit(t, private, 10)
	// int(10 / 27 * 20000)
	if got := private.AggroHate(w.player); got != 7407 {
		t.Fatalf("raid private hate after a hit on itself = %v, want 7407", got)
	}
	w.hit(t, leader, 10)
	if got := private.AggroHate(w.player); got != 2*7407 {
		t.Fatalf("raid private hate after a hit on its master = %v, want 14814", got)
	}
}

// TestRaidPrivateCursesOverLevelledAttacker ports the party-attacked hook's
// raid curse: a level 1 raid private hit by the level 20 character, more
// than 8 levels above it, wants to cast its raid curse on the character
// with weight 1000000 and stops hating it, dropping its attack desire on
// it. With a level 20 master, the hit first adds its hate, int(10 / 8 *
// 20000), which the curse then stops.
func TestRaidPrivateCursesOverLevelledAttacker(t *testing.T) {
	t.Parallel()
	w, leader, private := raidParty(t, 20, 1)
	w.srv.InstallNPCCastSeam(t, private, modelskill.NewTable([]modelskill.Definition{{
		ID: raidCurseID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, Magic: true, SkillType: "MDAM",
		CastRange: 600, HitTime: 1000, StaticHitTime: true, StaticReuse: true, Power: 1,
	}}))

	// Another creature on its hate list keeps the stopped hate from sending
	// the private back to peace, which in Go still drops every queued
	// desire (#3576); the reference's back to peace keeps them.
	w.onQueue(t, private, func() { private.AI().Hates().Add(leader, 1) })

	w.hit(t, private, 10)
	d, ok := desire(private, actorai.IntentionCast)
	if !ok || d.Skill.ID != raidCurseID || d.Weight != 1000000 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("raid private cast desire = %+v (%v), want its raid curse on the character, weight 1000000", d, ok)
	}
	if got := private.AggroHate(w.player); got != 0 {
		t.Fatalf("raid private hate after cursing the character = %v, want its hate stopped", got)
	}
	if d, ok := desire(private, actorai.IntentionAttack); ok {
		t.Fatalf("raid private still wants to attack the character it cursed: %+v", d)
	}
}

// TestRaidPrivateJumpsToItsTopDesireTarget ports the party-attacked hook's
// path-failure branch, which reads the AI's top desire target (#3545). A
// raid private that has taken up following its master has that master as
// its top desire target; hit after more than 10 failed paths, it is
// teleported onto that target, its master, within 1000 of it, not onto the
// character that hit it. Neither level branch applies: the character is
// more than 8 levels above the master and at most 8 above the private.
func TestRaidPrivateJumpsToItsTopDesireTarget(t *testing.T) {
	t.Parallel()
	w, leader, private := raidParty(t, 1, 20)

	w.ticks(t, 3)
	if got := private.AI().CurrentIntention(); got != actorai.IntentionFollow {
		t.Fatalf("raid private intention = %v, want follow", got)
	}
	if top := private.AI().TopDesireTarget(); top == nil || top.ObjectID() != leader.ObjectID() {
		t.Fatalf("raid private top desire target = %v, want its master", top)
	}
	w.teleport(t, private, 300, 0)
	w.onQueue(t, private, func() {
		for range 11 {
			private.AddGeoPathFailCount()
		}
	})

	w.hit(t, private, 10)
	w.srv.ReadQueued(t, w.srv.Client)
	if got, want := positionOf(private), positionOf(leader); got != want {
		t.Fatalf("raid private at %v after the hit, want on its master at %v (the character stands at %v)", got, want, w.at)
	}
	if got := private.AggroHate(w.player); got != 0 {
		t.Fatalf("raid private hate = %v, want none: no level branch applies", got)
	}
}
