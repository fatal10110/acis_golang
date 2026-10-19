package ai

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/siegeguard"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The siege guard ids the scenarios spawn, Aden Castle's: Aden lists them
// among its NPCs, so a guard spawned bare belongs to it.
const (
	adenClericID      = int32(35237) // GludioCleric
	adenClericStandID = int32(35250) // GludioClericStand
	adenWizardStandID = int32(35249) // GludioWizardStand
	adenBowGuardID    = int32(35241) // GludioBowGuard
	adenCourtGuardID  = int32(35238) // GludioCourtGuard
	adenDukeID        = int32(35234) // GludioDukeLewinWaldner
	adenKnightID      = int32(35235) // GludioKnight

	// siegeDummyID is a monster bound to no behavior.
	siegeDummyID = int32(20999)
)

// The skills of the siege guard scenarios: the guards' heal and DD magic,
// the character's Seal of Ruler, a good skill that is not it, and an
// offensive spell whose aggro points come to a twentieth of a guard's hit
// points.
const (
	siegeHealID    = modelskill.ID(9401)
	siegeDDMagicID = modelskill.ID(9402)
	sealOfRulerID  = modelskill.ID(246)
	siegeBlessID   = modelskill.ID(4101)
	siegeBoltID    = modelskill.ID(4102)
)

// siegeGuardScripts are the behaviors this batch ports and the class they
// derive from, in scripts.xml order.
func siegeGuardScripts() ([]script.Listing, script.Catalog) {
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":                         defaultnpc.New,
		"script.ai.siegeguards.GludioCleric":                      siegeguard.Cleric,
		"script.ai.siegeguards.GludioClericStand":                 siegeguard.ClericStand,
		"script.ai.siegeguards.GludioWizardStand":                 siegeguard.WizardStand,
		"script.ai.siegeguards.GludioHold.GludioBowGuard":         siegeguard.BowGuard,
		"script.ai.siegeguards.GludioHold.GludioCourtGuard":       siegeguard.CourtGuard,
		"script.ai.siegeguards.GludioHold.GludioDukeLewinWaldner": siegeguard.DukeLewinWaldner,
		"script.ai.siegeguards.GludioHold.GludioHold":             siegeguard.Hold,
		"script.ai.siegeguards.GludioHold.GludioKnight":           siegeguard.Knight,
	}
	var list []script.Listing
	for _, path := range []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.siegeguards.GludioCleric",
		"script.ai.siegeguards.GludioClericStand",
		"script.ai.siegeguards.GludioWizardStand",
		"script.ai.siegeguards.GludioHold.GludioBowGuard",
		"script.ai.siegeguards.GludioHold.GludioCourtGuard",
		"script.ai.siegeguards.GludioHold.GludioDukeLewinWaldner",
		"script.ai.siegeguards.GludioHold.GludioHold",
		"script.ai.siegeguards.GludioHold.GludioKnight",
	} {
		list = append(list, script.Listing{Path: path})
	}
	return list, catalog
}

// siegeGuardTemplate is a level 1 fixture siege guard of id, of the clan
// "aden_siege_clan" with a clan range of 300. Its 1000 base hit points come
// to siegeGuardMaxHP once its stats apply.
func siegeGuardTemplate(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.Type, tmpl.Name = "SiegeGuard", "Siege Guard"
	tmpl.Clans, tmpl.ClanRange = []string{"aden_siege_clan"}, 300
	return tmpl
}

// siegeGuardMaxHP is a fixture siege guard's maximum hit points: a hit of
// siegeTwentieth takes a twentieth of them, the step of the guards' damage
// weights.
const (
	siegeGuardMaxHP = 440
	siegeTwentieth  = 22
)

// siegeCasterTemplate is siegeGuardTemplate listing ref under t.
func siegeCasterTemplate(id int32, t npc.SkillType, ref modelskill.ID) *npc.Template {
	tmpl := siegeGuardTemplate(id)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{t: {ID: ref, Level: 1}}
	return tmpl
}

// siegeNPCSkills are the skills the guards cast: a heal of one target and
// a DD magic of one target, each of reach 600.
func siegeNPCSkills() *modelskill.Table {
	return modelskill.NewTable([]modelskill.Definition{
		{
			ID: siegeHealID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			Magic: true, SkillType: "HEAL", CastRange: 600, HitTime: 1000, StaticHitTime: true, StaticReuse: true, Power: 1,
		},
		{
			ID: siegeDDMagicID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			Offensive: true, Magic: true, SkillType: "MDAM", CastRange: 600, HitTime: 1000, StaticHitTime: true, StaticReuse: true, Power: 1,
		},
	})
}

// siegeCharacterSkills are the skills the character casts at the guards.
func siegeCharacterSkills() []modelskill.Definition {
	good := func(id modelskill.ID) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "BUFF", CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: fmt.Sprintf("siege_good_%d", id), StackOrder: 1}},
		}
	}
	return []modelskill.Definition{
		good(sealOfRulerID),
		good(siegeBlessID),
		{
			ID: siegeBoltID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			Offensive: true, Magic: true, SkillType: "MDAM", CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
			Power: 1, AggroPoints: siegeTwentieth,
		},
	}
}

// siegeSkillBoot boots the character knowing siegeCharacterSkills, which
// the scripts read too.
func siegeSkillBoot(t *testing.T) raidBoot {
	t.Helper()
	db := sqltest.SharedDB(t)
	defs := siegeCharacterSkills()
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	return raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			for _, def := range defs {
				if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(def.ID), 1); err != nil {
					t.Fatalf("seed skill %d: %v", def.ID, err)
				}
			}
		},
	}
}

// bootSiegeGuards boots the character (level 20) beside the siege guard
// templates, every script draw 0, with extra.
func bootSiegeGuards(t *testing.T, extra raidBoot, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	list, catalog := siegeGuardScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Hunter", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	}, extra.opts...)...)
	if extra.ready != nil {
		extra.ready(srv, srv.SoleObjectID(t))
	}
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

// spawnBelow places an NPC of id dx, dy from the character and dz below
// it, through the live population.
func (w *warriorWorld) spawnBelow(t *testing.T, id int32, dx, dy, dz int) *npc.Hostile {
	t.Helper()
	n, err := w.srv.NpcSpawns.AddSpawn(id, w.at.X+dx, w.at.Y+dy, w.at.Z-dz, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", id, err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	return n.(*npc.Hostile)
}

// selectTarget has the character select target.
func (w *warriorWorld) selectTarget(t *testing.T, target attackable.Combatant) {
	t.Helper()
	x, y, z := target.Position()
	w.srv.Client.Send(encodeFolkAction(target.ObjectID(), location.Location{X: x, Y: y, Z: z}))
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// cast has the character cast its skill id on its target, forced (a
// monster takes a good skill only with CTRL), and lets the cast land and
// end.
func (w *warriorWorld) cast(t *testing.T, id modelskill.ID) {
	t.Helper()
	p := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	p.WriteInt32(int32(id))
	p.WriteInt32(1)
	p.WriteUint8(0)
	w.srv.Client.Send(p.Bytes())
	var frames [][]byte
	w.srv.AdvanceUntil(t, "the character's cast to land and end", func() bool {
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		return slices.ContainsFunc(frames, func(f []byte) bool {
			return f[0] == serverpackets.OpcodeMagicSkillLaunched && int32(binary.LittleEndian.Uint32(f[1:5])) == w.player.ObjectID()
		}) && !w.srv.PlayerCastingNow(t, w.player.ObjectID())
	})
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// castsOf returns the MagicSkillUse frames h cast skill id with, each as
// its target's object id.
func castsOf(frames [][]byte, h *npc.Hostile, id modelskill.ID) []int32 {
	var out []int32
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, h.ObjectID()) {
		// opcode, caster, target, skill id, then the rest of the cast.
		if modelskill.ID(binary.LittleEndian.Uint32(f[9:13])) == id {
			out = append(out, int32(binary.LittleEndian.Uint32(f[5:9])))
		}
	}
	return out
}

// castDesire returns h's queued cast desire of skill id, and whether it
// has one.
func castDesire(h *npc.Hostile, id modelskill.ID) (actorai.Desire, bool) {
	d, ok := desire(h, actorai.IntentionCast)
	return d, ok && d.Skill.ID == id
}

// hates fails unless each guard hates the character by the weight given.
func (w *warriorWorld) hates(t *testing.T, when string, want map[*npc.Hostile]float64, names map[*npc.Hostile]string) {
	t.Helper()
	for h, weight := range want {
		if got := h.AggroHate(w.player); got != weight {
			t.Errorf("%s: %s hate of the character = %v, want %v", when, names[h], got, weight)
		}
	}
}

// TestGludioHoldGuardsAttackWhoeverHitsTheirClan ports the attacked and
// clan-attacked hooks of GludioHold and GludioBowGuard. A hit taking a
// twentieth of a knight's hit points (a GludioHold guard) makes it want to
// attack the character by 100 (attacked), and the hit's clan calls, which
// reach the knight itself first, then its clan mates in range, add 50 each:
// to the knight, to a court guard (GludioHold's clan-attacked hook) and to
// a bow guard (its own). The built-in attacked hate stays off for a bound
// id, so the hate is the hooks' weights alone. A second such hit, on the
// bow guard, adds its own 100 and 50, and 50 to each clan mate.
func TestGludioHoldGuardsAttackWhoeverHitsTheirClan(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeGuardTemplate(adenKnightID), siegeGuardTemplate(adenCourtGuardID), siegeGuardTemplate(adenBowGuardID))
	knight := w.spawn(t, adenKnightID, 100, 0)
	court := w.spawn(t, adenCourtGuardID, 0, 100)
	bow := w.spawn(t, adenBowGuardID, 0, -100)
	names := map[*npc.Hostile]string{knight: "knight", court: "court guard", bow: "bow guard"}
	if knight.MaxHP() != siegeGuardMaxHP {
		t.Fatalf("setup: knight max HP = %d, want %d", knight.MaxHP(), siegeGuardMaxHP)
	}

	w.hit(t, knight, siegeTwentieth)
	w.hates(t, "after the knight's hit", map[*npc.Hostile]float64{knight: 150, court: 50, bow: 50}, names)
	for h, name := range names {
		if d, ok := desire(h, actorai.IntentionAttack); !ok || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
			t.Errorf("%s attack desire = %+v, %v, want one on the character", name, d, ok)
		}
	}

	w.hit(t, bow, siegeTwentieth)
	w.hates(t, "after the bow guard's hit", map[*npc.Hostile]float64{knight: 200, court: 100, bow: 200}, names)
}

// TestGludioHoldRunsFromAnAttackerAbove ports GludioHold's attacked and
// clan-attacked hooks on an attacker standing more than 100 above: the
// knight hit from 150 above wants to run 150 from the character, weight
// 30, and does not attack it; the clan calls reach it and its court guard
// clan mate, also 150 below, for nothing. The bow guard helps a clan mate
// hit from any height (50), and hit from as high itself, attacks as usual
// (100, its own attacked hook has no height test, and 50 from its own
// call).
func TestGludioHoldRunsFromAnAttackerAbove(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeGuardTemplate(adenKnightID), siegeGuardTemplate(adenCourtGuardID), siegeGuardTemplate(adenBowGuardID))
	knight := w.spawnBelow(t, adenKnightID, 100, 0, 150)
	court := w.spawnBelow(t, adenCourtGuardID, 0, 100, 150)
	bow := w.spawnBelow(t, adenBowGuardID, 0, -100, 150)
	names := map[*npc.Hostile]string{knight: "knight", court: "court guard", bow: "bow guard"}

	w.hit(t, knight, siegeTwentieth)
	d, ok := desire(knight, actorai.IntentionFlee)
	if !ok || d.Distance != 150 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("knight flee desire = %+v, %v, want a 150 run from the character", d, ok)
	}
	if d, ok := desire(knight, actorai.IntentionAttack); ok {
		t.Fatalf("knight hit from above wants to attack: %+v", d)
	}
	w.hates(t, "after the knight's hit", map[*npc.Hostile]float64{knight: 0, court: 0, bow: 50}, names)

	w.hit(t, bow, siegeTwentieth)
	w.hates(t, "after the bow guard's hit", map[*npc.Hostile]float64{knight: 0, court: 0, bow: 200}, names)
	if d, ok := desire(bow, actorai.IntentionFlee); ok {
		t.Fatalf("bow guard hit from above wants to run: %+v", d)
	}
}

// TestGludioHoldInPeaceGoesHome ports the peace zone branch of GludioHold's
// attacked and see-creature hooks: a knight standing in a peace zone off
// its spawn point, hit by the character, is teleported back to its spawn
// point and drops its attack desires and hate, the 100 the attacked hook
// just added included; the hit's clan call to the knight itself comes
// after, so the knight is left wanting to attack the character by its 50.
// Displaced again, it goes back as soon as it sees the character.
func TestGludioHoldInPeaceGoesHome(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(200, 3_000, -1_000, 1_000, -1_000, 1_000)
	if err != nil {
		t.Fatalf("zone form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewPeace(1, form))
	w := bootSiegeGuards(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithZones(zones)}}, siegeGuardTemplate(adenKnightID))
	knight := w.spawnAt(t, adenKnightID, location.Location{X: 300, Y: w.at.Y, Z: w.at.Z})
	home := script.NPCOf(knight).SpawnLocation()
	homeLoc := location.Location{X: home.X, Y: home.Y, Z: home.Z}

	w.teleport(t, knight, 300, 0)
	if positionOf(knight) == homeLoc || !knight.InsideZone(zone.FlagPeace) {
		t.Fatalf("setup: knight at %v, in peace %v, want off its spawn point %v inside the peace zone", positionOf(knight), knight.InsideZone(zone.FlagPeace), homeLoc)
	}
	w.hit(t, knight, siegeTwentieth)
	if got := positionOf(knight); got != homeLoc {
		t.Fatalf("knight at %v after a hit in peace, want its spawn point %v", got, homeLoc)
	}
	if d, ok := desire(knight, actorai.IntentionAttack); !ok || d.Weight != 50 {
		t.Fatalf("knight attack desire after a hit in peace = %+v, %v, want the clan call's 50 alone", d, ok)
	}
	if got := knight.AggroHate(w.player); got != 50 {
		t.Fatalf("knight hate after a hit in peace = %v, want the clan call's 50 alone", got)
	}

	// Within the 450 a guard sees, still in the peace zone.
	w.teleport(t, knight, 100, 0)
	w.instantTeleport(t, location.Location{X: w.at.X + 6000, Y: w.at.Y, Z: w.at.Z})
	w.ticks(t, 1)
	w.instantTeleport(t, w.at)
	if got := positionOf(knight); got != homeLoc {
		t.Fatalf("knight at %v after seeing the character in peace, want its spawn point %v", got, homeLoc)
	}
}

// TestGludioHoldAttacksWhatItSeesOnceSettled ports GludioHold's
// see-creature hook: a knight that sees the character in its first AI
// cycles leaves it alone, as its life time is not past 7 yet; once it is,
// seeing the character again makes it want to attack it with weight 200.
func TestGludioHoldAttacksWhatItSeesOnceSettled(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeGuardTemplate(adenKnightID))
	knight := w.spawn(t, adenKnightID, 150, 0)

	w.ticks(t, 2)
	if got := knight.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the character seen at life time %d = %v, want none", knight.LifeTime(), got)
	}
	w.ticks(t, 8)
	if lt := knight.LifeTime(); lt <= 7 {
		t.Fatalf("setup: knight life time %d, want past 7", lt)
	}
	if got := knight.AggroHate(w.player); got != 0 {
		t.Fatalf("hate of the character still in sight = %v, want none: it is seen once while it stays", got)
	}

	w.instantTeleport(t, location.Location{X: w.at.X + 6000, Y: w.at.Y, Z: w.at.Z})
	w.ticks(t, 1)
	w.instantTeleport(t, w.at)
	if got := knight.AggroHate(w.player); got != 200 {
		t.Fatalf("hate of the character seen again past life time 7 = %v, want 200", got)
	}
}

// TestGludioHoldWalksBackToItsPost ports GludioHold's no-desire and
// move-finished hooks: a knight standing off its spawn point, out of the
// character's sight, with nothing to do wants to walk back there with
// weight 30, and walks; arriving, it wants to do nothing for 40 with weight
// 30.
func TestGludioHoldWalksBackToItsPost(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeGuardTemplate(adenKnightID))
	knight := w.spawn(t, adenKnightID, 700, 0)
	home := positionOf(knight)
	w.teleport(t, knight, 100, 0)

	// An NPC first idles on its second cycle; the cycle after acts on what
	// its hook queued.
	w.ticks(t, 2)
	if d, ok := desire(knight, actorai.IntentionMoveTo); !ok || d.Weight != 30 || d.Location != home {
		t.Fatalf("knight off its post move desire = %+v (%v), want weight 30 to %+v", d, ok, home)
	}
	frames := w.ticks(t, 1)
	if moves := framesOf(frames, serverpackets.OpcodeMoveToLocation, knight.ObjectID()); len(moves) != 1 || moveDestination(moves[0]) != home {
		t.Fatalf("knight walk frames = %d (%v), want one MoveToLocation to %+v", len(moves), moves, home)
	}
	for i := 0; knight.IsMoving(); i++ {
		if i >= 200 {
			t.Fatal("the knight never arrived")
		}
		w.srv.TickPositions()
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := positionOf(knight); got != home {
		t.Fatalf("knight stands at %+v, want its post %+v", got, home)
	}
	if d, ok := desire(knight, actorai.IntentionNothing); !ok || d.Weight != 30 || d.Timer != 40 {
		t.Fatalf("knight on its post do-nothing desire = %+v (%v), want timer 40, weight 30", d, ok)
	}
}

// TestGludioHoldGuardsAttackTheSealOfRuler ports the spelled hooks of
// GludioBowGuard, GludioCourtGuard, GludioDukeLewinWaldner and
// GludioKnight: Seal of Ruler landing on any of them from the character,
// who neither defends nor owns their castle, makes it want to attack the
// character with weight 50000; another good skill does nothing.
func TestGludioHoldGuardsAttackTheSealOfRuler(t *testing.T) {
	t.Parallel()
	ids := []int32{adenBowGuardID, adenCourtGuardID, adenDukeID, adenKnightID}
	var templates []*npc.Template
	for _, id := range ids {
		templates = append(templates, siegeGuardTemplate(id))
	}
	w := bootSiegeGuards(t, siegeSkillBoot(t), templates...)
	for i, id := range ids {
		// Out of one another's clan range: no hit here calls a clan mate.
		h := w.spawn(t, id, 100, -600+400*i)
		w.selectTarget(t, h)
		w.cast(t, siegeBlessID)
		if got := h.AggroHate(w.player); got != 0 {
			t.Fatalf("guard %d hate after another good skill = %v, want none", id, got)
		}
		w.cast(t, sealOfRulerID)
		if got := h.AggroHate(w.player); got != 50000 {
			t.Fatalf("guard %d hate after Seal of Ruler = %v, want 50000", id, got)
		}
	}
}

// TestGludioHoldAttacksWhoSpellsWhatItFights ports GludioHold's see-spell
// hook: a knight fighting a monster (called by a clan mate the monster
// hit, the knight wants to attack the monster) sees the character cast an
// offensive spell on that monster whose aggro points come to a twentieth
// of the knight's hit points, and wants to attack the character by 50. A
// good skill on the monster adds nothing. Hurt and past 10 failed paths, the knight seeing
// the spell again also jumps onto the character.
func TestGludioHoldAttacksWhoSpellsWhatItFights(t *testing.T) {
	t.Parallel()
	dummy := warriorTemplate(siegeDummyID, nil)
	// It outlives the fight and the character's spells.
	dummy.HPMax, dummy.PDef, dummy.MDef = 1_000_000, 1000, 1000
	boot := siegeSkillBoot(t)
	boot.opts = append(boot.opts, gameservertest.WithAttackStanceClock(time.Now))
	w := bootSiegeGuards(t, boot, siegeGuardTemplate(adenKnightID), siegeGuardTemplate(adenCourtGuardID), dummy)
	knight := w.spawn(t, adenKnightID, 150, 0)
	court := w.spawn(t, adenCourtGuardID, 150, 60)
	monster := w.spawn(t, siegeDummyID, 180, 30)

	// The monster hurts the knight (its attacked hook ignores an attacker
	// no player controls), then hits the court guard, whose call makes the
	// knight attack it.
	w.onQueue(t, monster, func() {
		knight.TakeDamage(10, monster)
		court.TakeDamage(50, monster)
	})
	if top := knight.AI().TopDesireTarget(); top == nil || top.ObjectID() != monster.ObjectID() {
		t.Fatalf("knight top desire target = %v, want the monster", top)
	}
	w.srv.AdvanceUntil(t, "the knight's fight with the monster", func() bool {
		cycle(t, w.srv)
		w.srv.TickPositions()
		return knight.InCombat()
	})

	w.selectTarget(t, monster)
	w.cast(t, siegeBlessID)
	if got := knight.AggroHate(w.player); got != 0 {
		t.Fatalf("knight hate after a good skill on the monster = %v, want none", got)
	}
	w.cast(t, siegeBoltID)
	if got := knight.AggroHate(w.player); got != 50 {
		t.Fatalf("knight hate after a spell on the monster it fights = %v, want 50", got)
	}

	w.onQueue(t, knight, func() {
		for range 11 {
			knight.AddGeoPathFailCount()
		}
		knight.AI().SetTopDesireTarget(monster)
	})
	w.cast(t, siegeBoltID)
	x, y, z := w.srv.PlayerPosition(t, w.player.ObjectID())
	if got := positionOf(knight); got != (location.Location{X: x, Y: y, Z: z}) {
		t.Fatalf("hurt knight past 10 failed paths at %v, want on the character at %v", got, location.Location{X: x, Y: y, Z: z})
	}
}

// TestGludioClericsHealAHurtClanMate ports the clan-attacked hooks of
// GludioCleric and GludioClericStand. A hit's clan calls run before the hit
// lowers the knight's hit points, so the clerics read what the knight had
// before it: at full hit points, and then at 270 of 440 (61%), the calls
// leave them idle; at 260 (59%), with a draw under 20, the cleric wants to
// cast its heal on the knight, walking to it, and the standing cleric wants
// the same from where it stands, each with weight 1000000. At their next
// think both cast it on the knight.
func TestGludioClericsHealAHurtClanMate(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeGuardTemplate(adenKnightID),
		siegeCasterTemplate(adenClericID, npc.SkillTypeMagicHeal, siegeHealID),
		siegeCasterTemplate(adenClericStandID, npc.SkillTypeMagicHeal, siegeHealID))
	knight := w.spawn(t, adenKnightID, 100, 0)
	cleric := w.spawn(t, adenClericID, 100, 150)
	stand := w.spawn(t, adenClericStandID, 100, -150)
	for _, h := range []*npc.Hostile{cleric, stand} {
		w.srv.InstallNPCCastSeam(t, h, siegeNPCSkills())
	}

	for _, damage := range []int{170, 10} {
		w.hit(t, knight, damage)
		for name, h := range map[string]*npc.Hostile{"cleric": cleric, "standing cleric": stand} {
			if d, ok := desire(h, actorai.IntentionCast); ok {
				t.Fatalf("%s wants %+v for a clan mate at %v HP before the hit", name, d, knight.HP()+float64(damage))
			}
		}
	}

	w.hit(t, knight, 10)
	for name, c := range map[string]struct {
		h    *npc.Hostile
		move bool
	}{"cleric": {cleric, true}, "standing cleric": {stand, false}} {
		d, ok := castDesire(c.h, siegeHealID)
		if !ok || d.Weight != 1000000 || d.MoveToTarget != c.move || d.FinalTarget == nil || d.FinalTarget.ObjectID() != knight.ObjectID() {
			t.Fatalf("%s heal desire = %+v, %v, want one on the knight, weight 1000000, walking %v", name, d, ok, c.move)
		}
	}
	var frames [][]byte
	w.srv.AdvanceUntil(t, "both heals", func() bool {
		frames = append(frames, cycle(t, w.srv)...)
		return len(castsOf(frames, cleric, siegeHealID)) > 0 && len(castsOf(frames, stand, siegeHealID)) > 0
	})
	for name, h := range map[string]*npc.Hostile{"cleric": cleric, "standing cleric": stand} {
		if got := castsOf(frames, h, siegeHealID); !slices.Equal(got, []int32{knight.ObjectID()}) {
			t.Fatalf("%s heal casts = %v, want one on the knight", name, got)
		}
	}
}

// TestGludioClericsSkipTheHealOnALosingDraw: a draw of 20 or more leaves
// both clerics idle, however hurt their clan mate (the second hit's calls
// read 40 of 440 hit points).
func TestGludioClericsSkipTheHealOnALosingDraw(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptRand(func(int) int { return 20 })}},
		siegeGuardTemplate(adenKnightID),
		siegeCasterTemplate(adenClericID, npc.SkillTypeMagicHeal, siegeHealID),
		siegeCasterTemplate(adenClericStandID, npc.SkillTypeMagicHeal, siegeHealID))
	knight := w.spawn(t, adenKnightID, 100, 0)
	cleric := w.spawn(t, adenClericID, 100, 150)
	stand := w.spawn(t, adenClericStandID, 100, -150)

	w.hit(t, knight, 400)
	w.hit(t, knight, 10)
	for name, h := range map[string]*npc.Hostile{"cleric": cleric, "standing cleric": stand} {
		if d, ok := desire(h, actorai.IntentionCast); ok {
			t.Fatalf("%s wants %+v on a draw of 20", name, d)
		}
	}
}

// TestGludioWizardStandCastsAtWhatItSees ports GludioWizardStand's
// see-creature hook: the standing wizard that sees the character wants to
// cast its DD magic on it from where it stands, weight 1000000, and casts
// it at its next think.
func TestGludioWizardStandCastsAtWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeCasterTemplate(adenWizardStandID, npc.SkillTypeDDMagic, siegeDDMagicID))
	wizard := w.spawn(t, adenWizardStandID, 150, 0)
	w.srv.InstallNPCCastSeam(t, wizard, siegeNPCSkills())

	frames := w.ticks(t, 1)
	d, ok := castDesire(wizard, siegeDDMagicID)
	if !ok || d.Weight != 1000000 || d.MoveToTarget || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("wizard DD magic desire = %+v, %v, want one on the character from where it stands, weight 1000000", d, ok)
	}
	w.srv.AdvanceUntil(t, "the wizard's DD magic", func() bool {
		frames = append(frames, cycle(t, w.srv)...)
		return len(castsOf(frames, wizard, siegeDDMagicID)) > 0
	})
	if got := castsOf(frames, wizard, siegeDDMagicID); !slices.Equal(got, []int32{w.player.ObjectID()}) {
		t.Fatalf("wizard DD magic casts = %v, want one on the character", got)
	}
}

// TestGludioWizardStandAnswersAClanCall ports GludioWizardStand's
// clan-attacked hook: before it has seen anyone, a hit on a clan mate
// makes the standing wizard want its DD magic on the character, weight
// 1000000, when the draw is under 10, and nothing on a draw of 10.
func TestGludioWizardStandAnswersAClanCall(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		draw int
		want bool
	}{{"draw 0", 0, true}, {"draw 10", 10, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootSiegeGuards(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptRand(func(int) int { return c.draw })}},
				siegeGuardTemplate(adenKnightID), siegeCasterTemplate(adenWizardStandID, npc.SkillTypeDDMagic, siegeDDMagicID))
			knight := w.spawn(t, adenKnightID, 100, 0)
			wizard := w.spawn(t, adenWizardStandID, 100, 150)
			w.srv.InstallNPCCastSeam(t, wizard, siegeNPCSkills())

			w.hit(t, knight, siegeTwentieth)
			d, ok := castDesire(wizard, siegeDDMagicID)
			if ok != c.want {
				t.Fatalf("wizard DD magic desire = %+v, %v, want %v", d, ok, c.want)
			}
			if ok && (d.Weight != 1000000 || d.MoveToTarget || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID()) {
				t.Fatalf("wizard DD magic desire = %+v, want one on the character from where it stands, weight 1000000", d)
			}
		})
	}
}
