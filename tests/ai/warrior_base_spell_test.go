package ai

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The self buffs the characters of the see-spell scenarios know: one with
// the aggro points 10, one with 100, one with none.
const (
	spellHate10ID  = modelskill.ID(9441)
	spellHate100ID = modelskill.ID(9442)
	spellQuietID   = modelskill.ID(9443)
	// spellMystic is a class of the mystic teaching line.
	spellMystic = 10
)

// spellBoot boots the characters knowing the three self buffs, in the
// scripts' skill table too, with the mystic class available.
func spellBoot(t *testing.T) raidBoot {
	t.Helper()
	db := sqltest.SharedDB(t)
	buff := func(id modelskill.ID, aggro int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true, AggroPoints: aggro,
		}
	}
	defs := []modelskill.Definition{buff(spellHate10ID, 10), buff(spellHate100ID, 100), buff(spellQuietID, 0)}
	table := modelskill.NewTable(append([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}}, defs...))
	mystic := gameservertest.ClassTemplate()
	mystic.ID = spellMystic
	return raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(defs...),
			gameservertest.WithClassTemplates(mystic),
		},
		ready: func(srv *gameservertest.Server, objID int32) { teachSpells(t, srv, objID) },
	}
}

// teachSpells has the character objID know the three self buffs.
func teachSpells(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	for _, id := range []modelskill.ID{spellHate10ID, spellHate100ID, spellQuietID} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(id), 1); err != nil {
			t.Fatalf("teach skill %d: %v", id, err)
		}
	}
}

// spellCaster is a second character in the world, with its client.
type spellCaster struct {
	client *testsupport.ScriptedClient
	obj    attackable.Combatant
}

// joinCaster brings a second character, of class, into the world at at,
// knowing the three self buffs.
func (w *warriorWorld) joinCaster(t *testing.T, at location.Location, class int) spellCaster {
	t.Helper()
	const account, name = "caster", "Caster"
	id := w.srv.SeedCharacterForAt(t, account, name, 20, 0, at).ID
	if class != 0 {
		if _, err := w.srv.DB.Exec(`UPDATE characters SET classid = ?, base_class = ? WHERE obj_Id = ?`, class, class, id); err != nil {
			t.Fatalf("set the caster's class: %v", err)
		}
	}
	teachSpells(t, w.srv, id)
	c := w.srv.DialClient(t, account, 1)
	c.Send(encodeGameStart())
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no CharSelected for the caster within 100 frames")
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	w.srv.ReadQueued(t, c)
	w.srv.ReadQueued(t, w.srv.Client)
	obj, ok := w.srv.State.Object(id)
	if !ok {
		t.Fatal("the caster is not in the world")
	}
	return spellCaster{client: c, obj: obj.(attackable.Combatant)}
}

// castSelfBuffOf has the character on c cast its self buff id and lets the
// cast land.
func (w *warriorWorld) castSelfBuffOf(t *testing.T, c *testsupport.ScriptedClient, id modelskill.ID) {
	t.Helper()
	p := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	p.WriteInt32(int32(id))
	p.WriteInt32(0)
	p.WriteUint8(0)
	c.Send(p.Bytes())
	w.srv.Advance(t, 2*time.Second)
	w.srv.Settle(t)
	w.srv.ReadQueued(t, c)
	w.srv.ReadQueued(t, w.srv.Client)
}

// spellNPC is a WarriorBase NPC that stands still, of level.
func spellNPC(level int, ai npc.AIParams) *npc.Template {
	tmpl := warriorTemplate(warriorBaseOnlyID, ai)
	tmpl.Level, tmpl.CanMove = level, false
	return tmpl
}

// wantPlayer queues an attack desire of weight on the character on h's
// queue.
func (w *warriorWorld) wantPlayer(t *testing.T, h *npc.Hostile, weight float64) {
	t.Helper()
	w.onQueue(t, h, func() { script.NPCOf(h).AddAttackDesire(script.PlayerOf(w.player), weight) })
}

// TestWarriorBaseSeeSpellTurnsOnAMuchLowerCaster ports the first branch of
// WarriorBase's see-spell hook. Seeing a caster more than 15 levels under
// its own cast, the NPC drops every attack desire; when that caster is the
// one it hates most and it is attacking, a spell with aggro points, power
// or an offensive nature then makes it want the caster with ((p / (level +
// 7)) * 150), p being the larger of the aggro points, the power and 20.
func TestWarriorBaseSeeSpellTurnsOnAMuchLowerCaster(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name  string
		level int
		skill modelskill.ID
		// mostHated is whom the NPC hates most before the spell, "" when it
		// is the character.
		mostHated string
		idle      bool // the NPC hates the character but is not attacking
		want      float64
	}{
		{name: "the most hated, aggro 10 floors at 20", level: 40, skill: spellHate10ID, want: 20.0 / 47 * 150},
		{name: "the most hated, aggro 100", level: 40, skill: spellHate100ID, want: 100.0 / 47 * 150},
		{name: "the most hated, a quiet spell", level: 40, skill: spellQuietID},
		{name: "the most hated, not attacking", level: 40, skill: spellHate10ID, idle: true},
		{name: "not the most hated", level: 40, skill: spellHate10ID, mostHated: "other"},
		{name: "15 levels below", level: 35, skill: spellHate10ID},
		{name: "16 levels below", level: 36, skill: spellHate10ID, want: 20.0 / 43 * 150},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w, draws := bootWarriorBaseWith(t, spellBoot(t), spellNPC(row.level, nil), warriorTemplate(warriorFarID, nil))
			h := settleSight(t, w, draws, 150, 0)
			other := w.spawn(t, warriorFarID, -150, 0)
			switch {
			case row.idle:
				w.onQueue(t, h, func() { h.AI().Threats().AddDamage(w.player, 0, 500) })
			case row.mostHated == "other":
				w.wantPlayer(t, h, 100)
				w.hateOn(t, h, other, 500)
			default:
				w.wantPlayer(t, h, 500)
				w.hateOn(t, h, other, 100)
			}

			w.castSelfBuffOf(t, w.srv.Client, row.skill)
			_, onOther := attackOn(h, other)
			d, onPlayer := attackOn(h, w.player)
			if row.level == 35 {
				// 15 levels under is not more than 15: nothing is dropped.
				if !onOther || !onPlayer {
					t.Fatalf("desires on the other and the character = %v and %v, want both kept", onOther, onPlayer)
				}
				return
			}
			// Every attack desire the NPC held is gone but the one the spell
			// gave it.
			if onOther {
				t.Fatal("the NPC still wants the other")
			}
			if row.want == 0 {
				if onPlayer {
					t.Fatalf("attack desire on the character = %+v, want none", d)
				}
				return
			}
			if !onPlayer || math.Abs(d.Weight-row.want) > 0.001 {
				t.Fatalf("attack desire on the character = %+v (%v), want weight %v", d, onPlayer, row.want)
			}
		})
	}
}

// TestWarriorBaseSeeSpellTeleportsAStuckNPCToItsCaster ports the second half
// of WarriorBase's see-spell hook for a caster more than 15 levels under the
// NPC that it hates most: an NPC that has taken damage and failed to find a
// path 10 times stops and jumps to the caster; one at full HP, or with fewer
// failures, stays.
func TestWarriorBaseSeeSpellTeleportsAStuckNPCToItsCaster(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name     string
		hp       float64
		failures int
		jumps    bool
	}{
		{"hurt, 10 failures", 0.9, 10, true},
		{"hurt, 9 failures", 0.9, 9, false},
		{"full HP, 10 failures", 1, 10, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w, draws := bootWarriorBaseWith(t, spellBoot(t), spellNPC(40, nil))
			h := settleSight(t, w, draws, 300, 0)
			w.wantPlayer(t, h, 500)
			if row.hp < 1 {
				w.hitDownTo(t, h, row.hp)
			}
			w.onQueue(t, h, func() {
				for range row.failures {
					h.AddGeoPathFailCount()
				}
			})
			from := location.Location{}
			from.X, from.Y, from.Z = h.Position()

			w.castSelfBuffOf(t, w.srv.Client, spellHate10ID)
			x, y, z := h.Position()
			at := location.Location{X: x, Y: y, Z: z}
			if row.jumps {
				if want := w.at; at != want {
					t.Fatalf("the NPC stands at %v, want the caster's %v", at, want)
				}
				return
			}
			if at != from {
				t.Fatalf("the NPC stands at %v, want where it was, %v", at, from)
			}
		})
	}
}

// TestWarriorBaseSwapPositionSwapsTheCasterWithItsTarget ports the
// SwapPosition branch of WarriorBase's see-spell hook. An attacking NPC whose
// most hated creature is a character of the fighter or priest line, seeing
// another character cast from under 900 away while it stands nearer the
// creature it hates, on a draw of 100 under the parameter, makes the two
// characters swap places and wants the caster with weight 1000.
func TestWarriorBaseSwapPositionSwapsTheCasterWithItsTarget(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		// mostHated is "player" (the character, a fighter), "caster", "other"
		// (an NPC) or "idle" (the character, hated without attacking).
		mostHated string
		casterDX  int // the caster's offset from the character; the NPC stands 150 east of it
		class     int // the second character's class
		draw      int
		swap      bool
	}{
		{"the character is hated most, the caster 350 away", "player", 500, 0, 59, true},
		{"the draw is at the parameter", "player", 500, 0, 60, false},
		{"the caster is 900 away", "player", 1050, 0, 59, false},
		{"the caster is 899 away", "player", 1049, 0, 59, true},
		{"the NPC is nearer the caster", "player", 160, 0, 59, false},
		{"the NPC is attacking no one", "idle", 500, 0, 59, false},
		{"an NPC is hated most", "other", 500, 0, 59, false},
		{"a fighter is hated most, cast by the character", "caster", 200, 0, 59, true},
		{"a mystic is hated most, cast by the character", "caster", 200, spellMystic, 59, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w, draws := bootWarriorBaseWith(t, spellBoot(t), spellNPC(1, npc.AIParams{"SwapPosition": "60"}), warriorTemplate(warriorFarID, nil))
			h := settleSight(t, w, draws, 150, 0)
			other := w.spawn(t, warriorFarID, 0, 300)
			mate := w.joinCaster(t, location.Location{X: w.at.X + row.casterDX, Y: w.at.Y, Z: w.at.Z}, row.class)
			mateID := mate.obj.ObjectID()
			switch row.mostHated {
			case "player":
				w.wantPlayer(t, h, 500)
			case "caster":
				w.onQueue(t, h, func() { script.NPCOf(h).AddAttackDesire(script.PlayerOf(mate.obj), 500) })
			case "other":
				w.hateOn(t, h, other, 500)
			case "idle":
				w.onQueue(t, h, func() { h.AI().Threats().AddDamage(w.player, 0, 500) })
			}
			draws.set(row.draw)
			meAt := w.at
			mateAt := location.Location{}
			mateAt.X, mateAt.Y, mateAt.Z = w.srv.PlayerPosition(t, mateID)

			if row.mostHated == "caster" {
				// The character is the caster; the second character is hated.
				w.castSelfBuffOf(t, w.srv.Client, spellQuietID)
			} else {
				w.castSelfBuffOf(t, mate.client, spellQuietID)
			}
			gotMe := location.Location{}
			gotMe.X, gotMe.Y, gotMe.Z = w.srv.PlayerPosition(t, w.player.ObjectID())
			gotMate := location.Location{}
			gotMate.X, gotMate.Y, gotMate.Z = w.srv.PlayerPosition(t, mateID)
			if !row.swap {
				if gotMe != meAt || gotMate != mateAt {
					t.Fatalf("the characters stand at %v and %v, want %v and %v", gotMe, gotMate, meAt, mateAt)
				}
				return
			}
			if gotMe != mateAt || gotMate != meAt {
				t.Fatalf("the characters stand at %v and %v, want %v and %v swapped", gotMe, gotMate, mateAt, meAt)
			}
			casting := mate.obj
			if row.mostHated == "caster" {
				casting = w.player
			}
			if d, ok := attackOn(h, casting); !ok || d.Weight != 1000 {
				t.Fatalf("attack desire on the caster = %+v (%v), want weight 1000", d, ok)
			}
		})
	}
}
