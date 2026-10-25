package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// gordonID is Gordon's NPC id.
const gordonID = int32(29095)

// bootGordon boots the raid boss world of bootRaidBosses with Gordon's
// behavior in its chain, Zariche as the one cursed weapon, beside tmpl and
// others, which no behavior drives.
func bootGordon(t *testing.T, tmpl *npc.Template, others ...*npc.Template) *ind7World {
	t.Helper()
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.RaidBoss.RaidBossStandard",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Gordon",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1",
	}
	catalog := script.Catalog{
		paths[0]: defaultnpc.New, paths[1]: monster.New, paths[2]: raid.BossStandard, paths[3]: raid.Gordon, paths[4]: raid.BossType1,
	}
	var list []script.Listing
	for _, p := range paths {
		list = append(list, script.Listing{Path: p})
	}
	zariche, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{
		ItemID: zaricheItemID, Skill: modelskill.Ref{ID: 3603, Level: 1}, Name: "Demonic Sword Zariche",
		DropRate: 1, Duration: 72, DurationLost: 24, DisappearChance: 50, StageKills: 10,
	}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	return bootRaidBosses(t, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithCursedWeapons(zariche),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{gordonID: script.KindHostile}, list, catalog),
	}}, append([]*npc.Template{tmpl}, others...)...)
}

// gordonGroundItem returns the one itemID lying in the world.
func gordonGroundItem(t *testing.T, srv *gameservertest.Server, itemID int32) *grounditem.Item {
	t.Helper()
	for _, obj := range srv.State.Objects() {
		if g, ok := obj.(*grounditem.Item); ok && g.ItemID() == itemID {
			return g
		}
	}
	t.Fatalf("no item %d on the ground", itemID)
	return nil
}

// layZariche starts Zariche's life as a monster kill's drop does and lays
// it at at.
func layZariche(t *testing.T, srv *gameservertest.Server, at location.Location) *grounditem.Item {
	t.Helper()
	if id, ok := srv.CursedWeapons.RollDrop(func(int) int { return 0 }); !ok || id != zaricheItemID {
		t.Fatalf("RollDrop = %d, %v; want Zariche", id, ok)
	}
	srv.SeedGroundItem(t, 0, zaricheItemID, 1, at.X, at.Y, at.Z)
	zariche := gordonGroundItem(t, srv, zaricheItemID)
	x, y, z := zariche.Position()
	srv.CursedWeapons.PlaceOnGround(zaricheItemID, zariche.ObjectID(), location.Location{X: x, Y: y, Z: z})
	srv.ReadQueued(t, srv.Client)
	return zariche
}

// TestGordonPicksUpACursedWeaponAndLeaves ports Gordon's created, timer,
// see-item and picked-item hooks. Gordon appears with no raid music, in
// its walk stance, wanting to walk the gordon route at 10. 3 s later, and
// every 3 s, it looks for Zariche or Akamanah within 500: Zariche lies 15
// from it, so it wants to pick it up at 1000000. Its AI picks it up, which
// leaves Gordon in the world; 3 s later Gordon leaves it.
func TestGordonPicksUpACursedWeaponAndLeaves(t *testing.T) {
	t.Parallel()
	w := bootGordon(t, rbTemplate(gordonID, 20, nil, nil))
	s := w.spawnFrames(t, gordonID, 200, 0)
	gordon := s.h
	if got := ind8Pick(s.frames, serverpackets.OpcodePlaySound); len(got) != 0 {
		t.Fatalf("Gordon's PlaySound frames = %x, want none", got)
	}
	if gordon.Running() {
		t.Fatal("Gordon runs, want its walk stance")
	}
	if d, ok := desire(gordon, actorai.IntentionMoveRoute); !ok || d.Weight != 10 {
		t.Fatalf("route desire = %+v (%v), want weight 10", d, ok)
	}

	at := positionOf(gordon)
	at.X += 15
	zariche := layZariche(t, w.srv, at)
	if _, ok := desire(gordon, actorai.IntentionPickUp); ok {
		t.Fatal("pick-up desire before Gordon's first look, want none")
	}
	w.srv.Advance(t, 3*time.Second)
	d, ok := desire(gordon, actorai.IntentionPickUp)
	if !ok || d.Weight != 1000000 {
		t.Fatalf("pick-up desire after the look = %+v (%v), want weight 1000000", d, ok)
	}
	w.srv.AdvanceUntil(t, "Zariche picked up", func() bool {
		cycle(t, w.srv)
		_, lying := w.srv.State.Object(zariche.ObjectID())
		return !lying
	})
	if got := w.liveNPCs(gordonID); len(got) != 1 {
		t.Fatal("Gordon left the world as it picked Zariche up")
	}
	w.srv.Advance(t, 3*time.Second)
	if got := w.liveNPCs(gordonID); len(got) != 0 {
		t.Fatal("Gordon still in the world 3 s after picking Zariche up")
	}
}

// TestGordonLooksForCursedWeaponsAfterAKill ports Gordon's attack finished
// hook: a swing the character lives through leaves Zariche, lying 15 from
// Gordon, unnoticed; the swing that kills the character has Gordon look
// for Zariche and Akamanah within 500 at once, wanting to pick Zariche up
// at 1000000 before its 3 s look comes round.
func TestGordonLooksForCursedWeaponsAfterAKill(t *testing.T) {
	t.Parallel()
	tmpl := gameservertest.AttackingHostileTemplate()
	tmpl.ID, tmpl.TemplateID, tmpl.Type, tmpl.Level = int(gordonID), int(gordonID), "RaidBoss", 20
	w := bootGordon(t, tmpl)
	at := w.at
	at.X += 40
	gordon := w.srv.SpawnAttackingHostileNPCTemplate(t, tmpl, at)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	near := at
	near.X += 15
	layZariche(t, w.srv, near)

	me := w.player.ObjectID()
	gordon.DoAttack(t, w.player)
	w.srv.Settle(t)
	if d, ok := desire(gordon.Hostile, actorai.IntentionPickUp); ok {
		t.Fatalf("pick-up desire after a swing the character lived through = %+v, want none", d)
	}
	w.srv.DamagePlayerHP(t, me, w.srv.PlayerCurrentHP(t, me)-1)
	gordon.DoAttack(t, w.player)
	w.srv.Settle(t)
	if !w.player.Dead() {
		t.Fatal("the character outlived the killing swing")
	}
	if d, ok := desire(gordon.Hostile, actorai.IntentionPickUp); !ok || d.Weight != 1000000 {
		t.Fatalf("pick-up desire after the killing swing = %+v (%v), want weight 1000000", d, ok)
	}
}

// TestGordonHuntsTheWielderOfACursedWeapon ports Gordon's see-creature
// hook, which its look at the creatures around it raises every 40 s: a
// raid boss sees within its aggro range, here 1000. Seeing the character,
// Gordon wants nothing of it; once the character wields Zariche, picked up
// at its feet, Gordon's next look wants to attack it at 1000000.
func TestGordonHuntsTheWielderOfACursedWeapon(t *testing.T) {
	t.Parallel()
	tmpl := rbTemplate(gordonID, 20, nil, nil)
	tmpl.AggroRange, tmpl.CanMove = 1000, false
	w := bootGordon(t, tmpl)
	gordon := w.spawn(t, gordonID, 400, 0)

	w.srv.Advance(t, 40*time.Second)
	if d, ok := attackOn(gordon, w.player); ok {
		t.Fatalf("attack desire on the unarmed character = %+v, want none", d)
	}

	zariche := layZariche(t, w.srv, w.at)
	p := wire.NewPacketWriter(clientpackets.OpcodeAction)
	for _, v := range []int32{zariche.ObjectID(), int32(w.at.X), int32(w.at.Y), int32(w.at.Z)} {
		p.WriteInt32(v)
	}
	p.WriteUint8(0)
	w.srv.Client.Send(p.Bytes())
	w.srv.AdvanceUntil(t, "Zariche picked up", func() bool {
		_, lying := w.srv.State.Object(zariche.ObjectID())
		return !lying
	})
	w.srv.SettlePickup(t, w.player.ObjectID())

	w.srv.Advance(t, 40*time.Second)
	if got := w.attackWeight(gordon); got != 1000000 {
		t.Fatalf("attack desire on the wielder = %v, want 1000000", got)
	}
}

// gordonServitorID is the character's servitor in the Gordon scenarios.
const gordonServitorID = int32(14001)

// TestGordonStopsAtAServitorsHit ports Gordon's attacked hook on a hit
// from a creature with no inventory: Gordon, level 60, is hit for 10 by
// the character's level 20 servitor. Asking the servitor for Zariche or
// Akamanah ends the hook there, so RaidBossStandard, which would want to
// attack a playable attacker by its hit, never answers it: Gordon wants
// nothing of the servitor. The character's own hit after it still runs
// the whole chain.
func TestGordonStopsAtAServitorsHit(t *testing.T) {
	t.Parallel()
	tmpl := rbTemplate(gordonID, 60, nil, nil)
	tmpl.CanMove = false
	servitorTmpl := &npc.Template{
		ID: int(gordonServitorID), TemplateID: int(gordonServitorID), Type: "Servitor", Name: "Servitor", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	w := bootGordon(t, tmpl, servitorTmpl)
	gordon := w.spawn(t, gordonID, 100, 0)
	w.onCharacter(t, func(c *player.Character) {
		c.SummonServitor(modelskill.Definition{ID: 1111, Level: 1, SkillType: "SUMMON", NpcID: int(gordonServitorID), SummonTotalLifeTime: 1_200_000})
	})
	var servitor *summon.Actor
	w.srv.AdvanceUntil(t, "the servitor is in the world", func() bool {
		obj, ok := w.srv.State.Summon(w.player.ObjectID())
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	w.srv.ReadQueued(t, w.srv.Client)

	done := make(chan struct{})
	if !servitor.Queue().Post(func() {
		defer close(done)
		gordon.TakeDamage(10, servitor)
	}) {
		t.Fatal("the servitor's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	if d, ok := attackOn(gordon, servitor); ok {
		t.Fatalf("attack desire on the servitor = %+v, want none", d)
	}
	w.hit(t, gordon, 10)
	if got, want := w.attackWeight(gordon), rbHate(10, 60); got != want {
		t.Fatalf("attack desire on the character after its hit = %v, want %v", got, want)
	}
}
