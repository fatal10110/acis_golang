package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The hall behaviors' NPC ids, and those of the privates Gustav and Lidia
// raise that no behavior of the boot is bound to.
const (
	partisanSorcererID = int32(35373) // Agit01PartisanSorcerer1
	dietrichID         = int32(35408) // Agit02Dietrich
	mikhailID          = int32(35409) // Agit02Mikhail
	gustavID           = int32(35410) // Agit02Gustav
	doomArcherID       = int32(35413)
	doomKnightID       = int32(35416)
	lidiaID            = int32(35629) // Agit03Lidia
	alfredID           = int32(35630) // Agit03Alfred
	giselleID          = int32(35631) // Agit03Giselle
	vampireID          = int32(35647)
)

// The skills the hall NPCs cast: the sorcerer's 4043, the Devastated Castle
// defenders' escape (4235), Gustav's and Mikhail's strikes (4236, 4237) and
// Dietrich's debuff (4238, 75 s of reuse), and the fixture skills the von
// Hellmanns' templates list by type.
const (
	hallSorcererSkill = modelskill.ID(4043)
	hallEscape        = modelskill.ID(4235)
	hallGustavHit     = modelskill.ID(4236)
	hallMikhailHit    = modelskill.ID(4237)
	hallReuseCurse    = modelskill.ID(4238)
	hallDDMagic       = modelskill.ID(9501)
	hallSelfBuff      = modelskill.ID(9502)
	hallSelfRangeBuff = modelskill.ID(9503)
	hallSelfRangeDD   = modelskill.ID(9504)
)

// hallScripts are the hall behaviors and the class they derive from, in
// scripts.xml order.
func hallScripts() ([]script.Listing, script.Catalog) {
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":                defaultnpc.New,
		"script.ai.siegablehall.Agit01PartisanSorcerer1": hall.PartisanSorcerer1,
		"script.ai.siegablehall.Agit02Dietrich":          hall.Dietrich,
		"script.ai.siegablehall.Agit02Gustav":            hall.Gustav,
		"script.ai.siegablehall.Agit02Mikhail":           hall.Mikhail,
		"script.ai.siegablehall.Agit03Alfred":            hall.Alfred,
		"script.ai.siegablehall.Agit03Giselle":           hall.Giselle,
		"script.ai.siegablehall.Agit03Lidia":             hall.Lidia,

		"script.ai.siegablehall.AzitWateringGroud":                           hall.Gourd,
		"script.ai.siegablehall.AzitWateringMimic":                           hall.Mimic,
		"script.ai.siegablehall.AzitWateringMimicMagical":                    hall.MimicMagical,
		"script.ai.siegablehall.AzitWateringRecycleCan":                      hall.RecycleCan,
		"script.ai.siegablehall.AzitWateringTreasureBox":                     hall.TreasureBox,
		"script.ai.siegablehall.Agit01Move1.Agit01Move1":                     hall.Move1,
		"script.ai.siegablehall.Agit01Move1.Agit01PartisanSorcerer2":         hall.PartisanSorcerer2,
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer1": hall.PartisanHealer1,
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer2": hall.PartisanHealer2,
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand1":                   hall.Stand1,
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand2":                   hall.Stand2,
		"script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka1":           hall.Nurka1,
	}
	var list []script.Listing
	for _, path := range []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.siegablehall.Agit01PartisanSorcerer1",
		"script.ai.siegablehall.Agit02Dietrich",
		"script.ai.siegablehall.Agit02Gustav",
		"script.ai.siegablehall.Agit02Mikhail",
		"script.ai.siegablehall.Agit03Alfred",
		"script.ai.siegablehall.Agit03Giselle",
		"script.ai.siegablehall.Agit03Lidia",
		"script.ai.siegablehall.AzitWateringGroud",
		"script.ai.siegablehall.AzitWateringMimic",
		"script.ai.siegablehall.AzitWateringMimicMagical",
		"script.ai.siegablehall.AzitWateringRecycleCan",
		"script.ai.siegablehall.AzitWateringTreasureBox",
		"script.ai.siegablehall.Agit01Move1.Agit01Move1",
		"script.ai.siegablehall.Agit01Move1.Agit01PartisanSorcerer2",
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer1",
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer2",
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand1",
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand2",
		"script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka1",
	} {
		list = append(list, script.Listing{Path: path})
	}
	return list, catalog
}

// agitTemplate is a level 1 fixture monster of id, of the clan
// "hall_clan" with a clan range of 300, whose 1000 base hit points come to
// siegeGuardMaxHP (a hit of siegeTwentieth takes a twentieth of them). Its
// defenses keep the character's spells' damage low.
func agitTemplate(id int32) *npc.Template {
	tmpl := warriorTemplate(id, nil)
	tmpl.Clans, tmpl.ClanRange = []string{"hall_clan"}, 300
	tmpl.PDef, tmpl.MDef = 1000, 1000
	return tmpl
}

// agitCasterTemplate is agitTemplate listing each skill under its type.
func agitCasterTemplate(id int32, skills map[npc.SkillType]modelskill.ID) *npc.Template {
	tmpl := agitTemplate(id)
	tmpl.SkillsByType = map[npc.SkillType]modelskill.Ref{}
	for t, skillID := range skills {
		tmpl.SkillsByType[t] = modelskill.Ref{ID: skillID, Level: 1}
	}
	return tmpl
}

// hallNPCSkills are the skills the hall NPCs cast: one-target strikes of
// reach 600, and self skills.
func hallNPCSkills() *modelskill.Table {
	strike := func(id modelskill.ID) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			Offensive: true, Magic: true, SkillType: "MDAM", CastRange: 600, HitTime: 1000, StaticHitTime: true, StaticReuse: true, Power: 1,
		}
	}
	self := func(id modelskill.ID, reuse int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 1000, StaticHitTime: true, StaticReuse: true, ReuseDelay: reuse,
		}
	}
	return modelskill.NewTable([]modelskill.Definition{
		strike(hallSorcererSkill), strike(hallGustavHit), strike(hallMikhailHit), strike(hallDDMagic), strike(hallSelfRangeDD),
		strike(hallNurkaStrike),
		self(hallEscape, 0), self(hallReuseCurse, 75000), self(hallSelfBuff, 0), self(hallSelfRangeBuff, 0),
	})
}

// bootHalls boots the character (level 20, of class, a human fighter for
// 0) beside the hall templates, every script draw draw, with extra; the
// NPCs cast hallNPCSkills. A non-zero at is where the character enters the
// world.
func bootHalls(t *testing.T, class, draw int, at location.Location, extra raidBoot, templates ...*npc.Template) *warriorWorld {
	t.Helper()
	list, catalog := hallScripts()
	kinds := map[int32]script.NPCKind{}
	for _, tmpl := range templates {
		kinds[int32(tmpl.ID)] = script.KindHostile
		if tmpl.Type == "Folk" {
			kinds[int32(tmpl.ID)] = script.KindFolk
		}
	}
	classTemplate := func(id int) *player.Template {
		tmpl := gameservertest.ClassTemplate()
		tmpl.ID = id
		if at != (location.Location{}) {
			tmpl.Spawns = []location.Location{at}
		}
		return tmpl
	}
	character := gameservertest.WithCharacter("Hunter", 20, 0)
	extra.opts = append(extra.opts, gameservertest.WithClassTemplate(classTemplate(0)))
	if class != 0 {
		var classes []*player.Template
		for id, ok := class, true; ok && id > 0; id, ok = player.ClassParent(id) {
			classes = append(classes, classTemplate(id))
		}
		character = gameservertest.WithClassCharacter("Hunter", 20, 0, class)
		extra.opts = append(extra.opts, gameservertest.WithClassTemplates(classes...))
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		character,
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNpcSpawnSkills(hallNPCSkills()),
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(func(int) int { return draw }),
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

// spawnSeen places an NPC of id at at through the live population, so its
// created hooks run, and returns it with the frames the character was sent.
func (w *warriorWorld) spawnSeen(t *testing.T, id int32, at location.Location) (*npc.Hostile, [][]byte) {
	t.Helper()
	n, err := w.srv.NpcSpawns.AddSpawn(id, at.X, at.Y, at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn %d: %v", id, err)
	}
	w.srv.Settle(t)
	return n.(*npc.Hostile), w.srv.ReadQueued(t, w.srv.Client)
}

// wantCast fails unless h has a cast desire of skill id on target with
// weight, walking to it when moving.
func wantCast(t *testing.T, name string, h *npc.Hostile, id modelskill.ID, target attackable.Combatant, weight float64, moving bool) {
	t.Helper()
	d, ok := castDesireOf(h, id)
	if !ok || d.Weight != weight || d.MoveToTarget != moving || d.FinalTarget == nil || d.FinalTarget.ObjectID() != target.ObjectID() {
		t.Fatalf("%s cast desire of %d = %+v, %v, want one on %d, weight %v, moving %v", name, id, d, ok, target.ObjectID(), weight, moving)
	}
}

// TestPartisanSorcererCastsAtWhatItSees ports Agit01PartisanSorcerer1's
// see-creature hook: the sorcerer that sees the character wants to cast
// 4043 on it from where it stands, weight 1000000, and casts it at its
// next think.
func TestPartisanSorcererCastsAtWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootHalls(t, 0, 0, location.Location{}, raidBoot{}, agitTemplate(partisanSorcererID))
	sorcerer := w.spawn(t, partisanSorcererID, 150, 0)

	frames := w.ticks(t, 1)
	wantCast(t, "sorcerer", sorcerer, hallSorcererSkill, w.player, 1000000, false)
	w.srv.AdvanceUntil(t, "the sorcerer's cast", func() bool {
		frames = append(frames, cycle(t, w.srv)...)
		return len(castsOf(frames, sorcerer, hallSorcererSkill)) > 0
	})
	if got := castsOf(frames, sorcerer, hallSorcererSkill); len(got) != 1 || got[0] != w.player.ObjectID() {
		t.Fatalf("sorcerer casts of 4043 = %v, want one on the character", got)
	}
}

// TestPartisanSorcererAnswersAClanCall ports Agit01PartisanSorcerer1's
// clan-attacked hook: before it has seen anyone, the character's hit on a
// clan mate makes the sorcerer want 4043 on the character from where it
// stands, weight 1000000, on a draw under 10, and nothing on a draw of 10.
func TestPartisanSorcererAnswersAClanCall(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		draw int
		want bool
	}{{"draw 0", 0, true}, {"draw 10", 10, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootHalls(t, 0, c.draw, location.Location{}, raidBoot{}, agitTemplate(partisanSorcererID), agitTemplate(siegeDummyID))
			mate := w.spawn(t, siegeDummyID, 100, 0)
			sorcerer := w.spawn(t, partisanSorcererID, 100, 150)

			w.hit(t, mate, siegeTwentieth)
			if _, ok := castDesireOf(sorcerer, hallSorcererSkill); ok != c.want {
				t.Fatalf("sorcerer has a cast desire of 4043 = %v, want %v", ok, c.want)
			}
			if c.want {
				wantCast(t, "sorcerer", sorcerer, hallSorcererSkill, w.player, 1000000, false)
			}
		})
	}
}

// hallTimerStep is how long a hall NPC's first fixed-rate check waits.
const hallTimerStep = 1000 * time.Millisecond

// noAttackDesire fails when h still wants to attack anyone.
func noAttackDesire(t *testing.T, name string, h *npc.Hostile) {
	t.Helper()
	if d, ok := desire(h, actorai.IntentionAttack); ok {
		t.Fatalf("%s still wants to attack: %+v", name, d)
	}
}
