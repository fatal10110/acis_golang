package ai

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPartyPrivateCast3SkillsMagicalCastsAtItsMostHatedFromAfar ports
// PartyPrivateCast3SkillsMagical's attacked hook, every draw 0. The first
// hit, before the NPC hates the character, wants nothing. A hit by its
// most hated standing more than 100 away wants both DD magics and the
// debuff on it with weight 1000000; from 50 away, none.
func TestPartyPrivateCast3SkillsMagicalCastsAtItsMostHatedFromAfar(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		dx   int
		cast bool
	}{{"far", 300, true}, {"near", 50, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Template(ind4Cast3ID, false))
			n := w.spawn(t, ind4Cast3ID, row.dx, 0)
			w.srv.InstallNPCCastSeam(t, n, ind4Skills())
			me := w.player.ObjectID()

			w.hit(t, n, 10)
			w.wantCasts(t, "after the first hit", n, nil)
			w.clearDesires(t, n)
			w.hit(t, n, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.cast {
				want = map[modelskill.ID]ind1Cast{ind4DDMagic1Skill: {me, 1000000}, ind4DDMagic2Skill: {me, 1000000}, ind4DebuffSkill: {me, 1000000}}
			}
			w.wantCasts(t, "after a hit by the most hated", n, want)
		})
	}
}

// TestPartyPrivateCast3SkillsMagicalAnswersItsClan ports
// PartyPrivateCast3SkillsMagical's clan-attacked hook, every draw 0: a
// settled clan member that is not attacking wants both DD magics and the
// debuff on another member's attacker standing more than 100 from it, and
// only the debuff on one standing at 50. A member that is attacking wants
// none, and neither does a young one.
func TestPartyPrivateCast3SkillsMagicalAnswersItsClan(t *testing.T) {
	t.Parallel()
	all := []modelskill.ID{ind4DDMagic1Skill, ind4DDMagic2Skill, ind4DebuffSkill}
	for _, row := range []struct {
		name      string
		settle    int
		attacking bool
		dx        int
		casts     []modelskill.ID
	}{
		{"far", 8, false, 0, all},
		{"near", 8, false, -50, []modelskill.ID{ind4DebuffSkill}},
		{"attacking", 8, true, 0, nil},
		{"young", 0, false, 0, nil},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Template(ind4Cast3ID, true), ind4Template(ind4Cast3TwoID, true))
			victim := w.spawn(t, ind4Cast3ID, 1500, 0)
			member := w.spawn(t, ind4Cast3TwoID, 1500, 200)
			w.ticks(t, row.settle)
			if row.attacking {
				// With no skills yet, the hit by the character beside it
				// leaves it only its attack; the character then steps back
				// to 300.
				w.moveCharacter(t, member, 50, 0)
				w.hit(t, member, 10)
				w.ticks(t, 1)
				if member.AI().CurrentIntention() != actorai.IntentionAttack {
					t.Fatalf("setup: member intention = %v, want attack", member.AI().CurrentIntention())
				}
				w.moveCharacter(t, member, 300, 0)
			}
			w.srv.InstallNPCCastSeam(t, member, ind4Skills())
			if row.dx != 0 {
				w.moveCharacter(t, member, row.dx, 0)
			}
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			for _, id := range row.casts {
				want[id] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
	}
}

// TestPartyPrivateCastCurseCursesWhatItFights ports PartyPrivateCastCurse's
// attacked and clan-attacked hooks, every draw 0. Its first hit, hating
// nobody yet, wants nothing; a hit by its most hated wants the debuff on
// it with weight 1000000. A settled clan member wants the debuff on
// another member's attacker; a young one does not.
func TestPartyPrivateCastCurseCursesWhatItFights(t *testing.T) {
	t.Parallel()
	t.Run("attacked", func(t *testing.T) {
		t.Parallel()
		w := bootIndividual4(t, raidBoot{}, ind4Template(ind4CurseID, false))
		n := w.spawn(t, ind4CurseID, 300, 0)
		w.srv.InstallNPCCastSeam(t, n, ind4Skills())
		w.hit(t, n, 10)
		w.wantCasts(t, "after the first hit", n, nil)
		w.clearDesires(t, n)
		w.hit(t, n, 10)
		w.wantCasts(t, "after a hit by the most hated", n, map[modelskill.ID]ind1Cast{ind4DebuffSkill: {w.player.ObjectID(), 1000000}})
	})
	for _, row := range []struct {
		name   string
		settle int
		curse  bool
	}{{"settled member", 8, true}, {"young member", 0, false}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual4(t, raidBoot{}, ind4Template(ind4CurseID, true))
			victim := w.spawn(t, ind4CurseID, 1500, 0)
			member := w.spawn(t, ind4CurseID, 1500, 200)
			w.srv.InstallNPCCastSeam(t, member, ind4Skills())
			w.ticks(t, row.settle)
			w.clearDesires(t, member)
			w.hit(t, victim, 10)
			want := map[modelskill.ID]ind1Cast{}
			if row.curse {
				want[ind4DebuffSkill] = ind1Cast{w.player.ObjectID(), 1000000}
			}
			w.wantCasts(t, "member after the hit", member, want)
		})
	}
}

// TestPartyPrivateHereticCastCurseFollowsItsMastersCalls ports
// PartyPrivateHereticCastCurse's created, party-attacked and script-event
// hooks. It spawns with i_ai4 0, and a hit on its master while it neither
// attacks nor casts sets i_ai4. Event 10035 from its master clears i_ai4,
// drops its attack desires and wants to walk to its spawn point with
// weight 100. An event whose second argument is not its master's id is
// ignored. Event 10002 from its master drops its attack desires and wants
// to attack the named player with weight 200; 10033 does the same and
// sets i_ai4.
func TestPartyPrivateHereticCastCurseFollowsItsMastersCalls(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4HereticID, 1), ind4Template(ind4HereticID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	heretic := privatesOf(t, leader, 1)[0]
	me, master := w.player.ObjectID(), leader.ObjectID()
	if got := ind1Slot(heretic, script.SlotIntAI4); got != 0 {
		t.Fatalf("created i_ai4 = %d, want 0", got)
	}
	if i := heretic.AI().CurrentIntention(); i == actorai.IntentionAttack || i == actorai.IntentionCast {
		t.Fatalf("setup: heretic intention = %v, want neither attack nor cast", i)
	}
	w.hit(t, leader, 10)
	if got := ind1Slot(heretic, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after a hit on its master = %d, want 1", got)
	}

	w.sendEvent(t, heretic, 10035, 0, master)
	if got := ind1Slot(heretic, script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 after event 10035 = %d, want 0", got)
	}
	if d, ok := desire(heretic, actorai.IntentionAttack); ok {
		t.Fatalf("attack desire after event 10035 = %+v, want none", d)
	}
	home := script.NPCOf(heretic).SpawnLocation()
	if d, ok := desire(heretic, actorai.IntentionMoveTo); !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
		t.Fatalf("move desire after event 10035 = %+v (%v), want weight 100 to its spawn point %d,%d", d, ok, home.X, home.Y)
	}

	w.sendEvent(t, heretic, 10033, me, me)
	if d, ok := desire(heretic, actorai.IntentionAttack); ok || ind1Slot(heretic, script.SlotIntAI4) != 0 {
		t.Fatalf("after an event not from its master: attack desire %+v (%v), i_ai4 %d, want neither", d, ok, ind1Slot(heretic, script.SlotIntAI4))
	}

	w.sendEvent(t, heretic, 10002, me, master)
	wantDesire(t, "after event 10002", heretic, actorai.IntentionAttack, me, 200)
	if got := ind1Slot(heretic, script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 after event 10002 = %d, want 0", got)
	}
	w.sendEvent(t, heretic, 10033, me, master)
	// The earlier desire is dropped, not added to.
	wantDesire(t, "after event 10033", heretic, actorai.IntentionAttack, me, 200)
	if got := ind1Slot(heretic, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after event 10033 = %d, want 1", got)
	}
}

// TestPartyPrivateHereticCastCurseLeavesTerritoryOnceHit ports
// PartyPrivateHereticCastCurse's attacked hook on a heretic of no party: a
// hit while it neither attacks nor casts sets i_ai4; a hit while it
// attacks leaves i_ai4 clear.
func TestPartyPrivateHereticCastCurseLeavesTerritoryOnceHit(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Template(ind4HereticID, false))
	idle := w.spawn(t, ind4HereticID, 300, 0)
	w.hit(t, idle, 10)
	if got := ind1Slot(idle, script.SlotIntAI4); got != 1 {
		t.Fatalf("i_ai4 after a hit while idle = %d, want 1", got)
	}

	fighting := w.spawn(t, ind4HereticID, 0, 300)
	w.hit(t, fighting, 10)
	w.ticks(t, 1)
	if fighting.AI().CurrentIntention() != actorai.IntentionAttack {
		t.Fatalf("setup: intention = %v, want attack", fighting.AI().CurrentIntention())
	}
	w.onQueue(t, fighting, func() { script.NPCOf(fighting).SetInt(script.SlotIntAI4, 0) })
	w.hit(t, fighting, 10)
	if got := ind1Slot(fighting, script.SlotIntAI4); got != 0 {
		t.Fatalf("i_ai4 after a hit while attacking = %d, want 0", got)
	}
}

// ind4HereticDen is an npcmaker spawning one heretic in a 400 by 400
// territory.
const ind4HereticDen = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="heretic_den" minZ="-1000" maxZ="1000"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="heretic_den" territory="heretic_den" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="22157" total="1" pos="40;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// ind4WalkTo has h walk to to through its AI and waits for its arrival.
func ind4WalkTo(t *testing.T, w *ind1World, h *npc.Hostile, to location.Location) {
	t.Helper()
	if !h.AI().AddMoveToDesire(to, 1e6) {
		t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
	}
	for i := 0; !h.IsMoving(); i++ {
		if i == 5 {
			t.Fatalf("no walk to %+v after five cycles", to)
		}
		cycle(t, w.srv)
	}
	for i := 0; h.IsMoving(); i++ {
		if i >= int(20*time.Second/move.PositionUpdateInterval) {
			t.Fatal("walk never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// TestPartyPrivateHereticCastCurseWalksHomeFromOutside ports
// PartyPrivateHereticCastCurse's out-of-territory hook on a heretic its
// npcmaker spawned. Arriving outside its territory with i_ai4 clear, it
// wants to walk to its spawn point with weight 100 (dropping its attack
// desires, as event 10035 shows). Released from the territory (i_ai4 set),
// its next arrival outside wants no walk home.
func TestPartyPrivateHereticCastCurseWalksHomeFromOutside(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		released bool
	}{{"bound", false}, {"released", true}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(ind4HereticDen), 0o644); err != nil {
				t.Fatalf("write spawnlist: %v", err)
			}
			table, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
			if err != nil {
				t.Fatalf("load spawnlist: %v", err)
			}
			w := bootIndividual4(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithNpcSpawns(table)}}, ind4Template(ind4HereticID, false))
			var heretic *npc.Hostile
			for _, obj := range w.srv.State.Objects() {
				if h, ok := obj.(*npc.Hostile); ok && h.NpcID() == int(ind4HereticID) {
					heretic = h
				}
			}
			if heretic == nil {
				t.Fatal("the den spawned no heretic")
			}
			if row.released {
				w.onQueue(t, heretic, func() { script.NPCOf(heretic).SetInt(script.SlotIntAI4, 1) })
			}
			home := script.NPCOf(heretic).SpawnLocation()

			ind4WalkTo(t, w, heretic, location.Location{X: 40, Y: 700, Z: 30})
			if heretic.InTerritory() {
				t.Fatal("setup: the walk ended inside the territory")
			}
			d, ok := desire(heretic, actorai.IntentionMoveTo)
			if row.released {
				if ok {
					t.Fatalf("released, move desire after arriving outside = %+v, want none", d)
				}
				return
			}
			if !ok || d.Weight != 100 || d.Location.X != home.X || d.Location.Y != home.Y {
				t.Fatalf("move desire after arriving outside = %+v (%v), want weight 100 to its spawn point %d,%d", d, ok, home.X, home.Y)
			}
		})
	}
}

// TestPartyPrivateCastDDHealHealsItsParty ports PartyPrivateCastDDHeal's
// attacked and party-attacked hooks for two privates of one master, every
// draw 0. The hooks run before the hit takes its HP, so the hit that
// takes the victim to half its HP heals nothing: the victim's own party
// call and the other private's each want the DD magic on the character
// standing more than 100 away, and the victim's attacked hook wants none
// while the character is not its top desire target. The next hit, the
// victim under 70%, also makes the victim want its heal on itself and the
// other private its heal on the victim. With the character its top desire
// target, the victim's attacked hook adds a DD magic of its own.
func TestPartyPrivateCastDDHealHealsItsParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4DDHealID, 2), ind4Template(ind4DDHealID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	privates := privatesOf(t, leader, 2)
	victim, other := privates[0], privates[1]
	reset := func() {
		for _, h := range privates {
			w.clearDesires(t, h)
			w.onQueue(t, h, func() { script.NPCOf(h).SetTopDesireTarget(nil) })
		}
	}
	for _, h := range privates {
		w.srv.InstallNPCCastSeam(t, h, ind4Skills())
	}
	me := w.player.ObjectID()
	reset()

	w.hit(t, victim, int(victim.MaxHPValue()/2))
	if r := script.NPCOf(victim).HPRatio(); r >= 0.7 {
		t.Fatalf("setup: victim HP ratio after the hit = %v, want under 0.7", r)
	}
	w.wantCasts(t, "victim after the hit to half its HP", victim, map[modelskill.ID]ind1Cast{ind4DDMagicSkill: {me, 1000000}})
	w.wantCasts(t, "other private after the hit to half its HP", other, map[modelskill.ID]ind1Cast{ind4DDMagicSkill: {me, 1000000}})

	reset()
	w.hit(t, victim, 1)
	heal := ind1Cast{victim.ObjectID(), 1000000}
	w.wantCasts(t, "victim after a hit under 70%", victim, map[modelskill.ID]ind1Cast{ind4HealSkill: heal, ind4DDMagicSkill: {me, 1000000}})
	w.wantCasts(t, "other private after a hit under 70%", other, map[modelskill.ID]ind1Cast{ind4HealSkill: heal, ind4DDMagicSkill: {me, 1000000}})

	reset()
	w.onQueue(t, victim, func() { script.NPCOf(victim).SetTopDesireTarget(script.PlayerOf(w.player)) })
	w.hit(t, victim, 1)
	w.wantCasts(t, "victim after a hit by its top desire target", victim, map[modelskill.ID]ind1Cast{ind4HealSkill: heal, ind4DDMagicSkill: {me, 2000000}})
}

// TestPartyPrivateCastDDMagicCurseStonePetrifiesAllies ports
// PartyPrivateCastDDMagicCurseStone's attacked and clan-attacked hooks for
// a Ketra and a Varka private of no party, every draw 0. A character
// allied with its faction that hits it makes it want to petrify the
// character (4515, weight 1000000 from its attacked hook and as much again
// from its own clan call), stop hating it and want neither its debuff nor
// its DD magic. A character of no alliance gets its DD magic on the first
// hit, and its debuff too once it is the most hated.
func TestPartyPrivateCastDDMagicCurseStonePetrifiesAllies(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		faction  string
		standing int
	}{{"ketra", "ketra_orc_clan", 1}, {"varka", "varka_silenos_clan", -1}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			tmpl := ind4Template(ind4StoneID, false)
			tmpl.Clans, tmpl.ClanRange = []string{row.faction}, 500
			w := bootIndividual4(t, raidBoot{}, tmpl)
			ch := ind3Character(t, w)
			skills := modelskill.NewTable(append(ind4Skills().All(), npcSkill(petrifyID, 1, true)))
			me := w.player.ObjectID()

			ch.SetVarkaKetraAlliance(row.standing)
			allied := w.spawn(t, ind4StoneID, 300, 0)
			w.srv.InstallNPCCastSeam(t, allied, skills)
			w.hit(t, allied, 10)
			w.wantCasts(t, "after an ally's hit", allied, map[modelskill.ID]ind1Cast{petrifyID: {me, 2000000}})
			if got := allied.AggroHate(w.player); got != 0 {
				t.Fatalf("hate of the ally that hit it = %v, want none", got)
			}

			ch.SetVarkaKetraAlliance(0)
			open := w.spawn(t, ind4StoneID, 0, 300)
			w.srv.InstallNPCCastSeam(t, open, skills)
			w.hit(t, open, 10)
			w.wantCasts(t, "after the first hit of no ally", open, map[modelskill.ID]ind1Cast{ind4DDMagicSkill: {me, 1000000}})
			w.clearDesires(t, open)
			w.hit(t, open, 10)
			w.wantCasts(t, "after a hit by the most hated", open, map[modelskill.ID]ind1Cast{ind4DDMagicSkill: {me, 1000000}, ind4DebuffSkill: {me, 1000000}})
		})
	}
}

// TestPartyPrivateCastDDMagicCurseStoneAnswersItsParty ports
// PartyPrivateCastDDMagicCurseStone's party-attacked hook, every draw 0: a
// hit on its master by a character of no alliance, while the private is
// not attacking, wants its DD magic on the character with weight 1000000,
// and its debuff only once the character is its most hated, which the
// first party call makes it.
func TestPartyPrivateCastDDMagicCurseStoneAnswersItsParty(t *testing.T) {
	t.Parallel()
	w := bootIndividual4(t, raidBoot{}, ind4Leader(ind4StoneID, 1), ind4Template(ind4StoneID, false))
	leader := w.spawn(t, leaderID, 400, 0)
	private := privatesOf(t, leader, 1)[0]
	w.srv.InstallNPCCastSeam(t, private, ind4Skills())
	me := w.player.ObjectID()
	w.clearDesires(t, private)

	w.hit(t, leader, 10)
	w.wantCasts(t, "after the first hit on its master", private, map[modelskill.ID]ind1Cast{ind4DDMagicSkill: {me, 1000000}})
	if private.AI().CurrentIntention() == actorai.IntentionAttack {
		t.Fatal("setup: the private took up its attack")
	}
	w.clearDesires(t, private)
	w.hit(t, leader, 10)
	w.wantCasts(t, "after the second hit on its master", private, map[modelskill.ID]ind1Cast{ind4DDMagicSkill: {me, 1000000}, ind4DebuffSkill: {me, 1000000}})
}
