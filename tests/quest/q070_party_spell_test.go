package quest

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// What the saga core does with the spells cast at the corrupted knight and
// with the party that kills an Archon of Halisha in the Four Sepulchers
// (ThirdClassQuest.java onSeeSpell, onMyDying).

// The skills the saga's players cast in these tests, each with a fixed
// half-second cast: an offensive spell at one target that neither damages
// nor debuffs it and adds no hate, so the target's attacked hooks never
// run and the spell reaches the saga through its see-spell hook alone; and
// a buff of the caster alone.
const (
	sagaSpell    = 9310
	sagaSelfBuff = 9311
)

func sagaSkills() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: sagaSpell, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			SkillType: "DUMMY", Offensive: true, CastRange: 900,
			HitTime: 500, StaticHitTime: true, StaticReuse: true,
		},
		{
			ID: sagaSelfBuff, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "BUFF", HitTime: 500, StaticHitTime: true, StaticReuse: true,
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true, StackType: "saga_self", StackOrder: 1}},
		},
	}
}

// sagaSkillOption gives the server the saga test skills.
func sagaSkillOption(t *testing.T) gameservertest.Option {
	t.Helper()
	db := sqltest.SharedDB(t)
	return gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(sagaSkills()), gamesql.NewCharacterSkillStore(db)))
}

// knowSagaSkills makes the character objID know the saga test skills at
// its next selection.
func knowSagaSkills(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	for _, def := range sagaSkills() {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(def.ID), def.Level); err != nil {
			t.Fatal(err)
		}
	}
}

// cast has the client c of caster target obj (zero keeps the target), cast
// skill and lets the cast end; it fails unless the caster was seen casting
// skill at the role target.
func (w *dialogWorld) cast(t *testing.T, c *testsupport.ScriptedClient, caster string, obj int32, skill int32, target string) {
	t.Helper()
	if obj != 0 {
		c.Send(encodeTutorialAction(obj, w.at))
		w.srv.ReadQueued(t, c)
	}
	c.Send(encodeLowHPSkillUse(skill))
	w.srv.Advance(t, time.Second)
	want := "S MagicSkillUse caster=" + caster + " target=" + target + " skill=" + strconv.Itoa(int(skill)) + "-1 "
	got := w.render(t, w.srv.ReadQueued(t, c))
	if !slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, want) }) {
		t.Fatalf("%s casting %d: got %q, want %q", caster, skill, got, want)
	}
}

// TestQ070CorruptedKnightLeavesWhenAnotherCastsAtIt: the knight stays
// when the player who woke it casts at it, and when another player of the
// saga casts a spell whose targets leave it out; another player's spell at
// it sends it away at once, with no line, and frees the tablet, which
// wakes a new knight for the player. The spell runs no attacked hook, so
// the see-spell hook alone sends the knight away.
func TestQ070CorruptedKnightLeavesWhenAnotherCastsAtIt(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 8, nil, sagaSkillOption(t))
	w.srv.RunQuest(t, w.player, q070Name, func(_ *script.Quests, c *player.Character, _ *script.Script) {
		for _, def := range sagaSkills() {
			c.SetSkillLevel(int(def.ID), def.Level)
		}
	})
	w.spawnFolk(t, "tablet3", tablet3ID, -40, 0, 0)
	_, mateClient := sagaMate(t, w, 8, false, func(objID int32) { knowSagaSkills(t, w.srv, objID) })

	w.talk(t, "tablet3")
	wantLines(t, "7-1", w.sagaEvent(t, "7-1"), w.sagaPage(t, "tablet3", "7-02.htm"))
	knight := w.spawned(t, "corrupted", corruptedID)
	w.advance(t, time.Second)

	w.cast(t, w.srv.Client, "player", knight.ObjectID(), sagaSpell, "corrupted")
	if n := len(npcsOf(w.srv, corruptedID)); n != 1 {
		t.Fatalf("%d knights after the player's own spell at it, want 1", n)
	}

	w.cast(t, mateClient, "mate", 0, sagaSelfBuff, "mate")
	if n := len(npcsOf(w.srv, corruptedID)); n != 1 {
		t.Fatalf("%d knights after the mate's self buff, want 1", n)
	}

	w.cast(t, mateClient, "mate", knight.ObjectID(), sagaSpell, "corrupted")
	if n := len(npcsOf(w.srv, corruptedID)); n != 0 {
		t.Fatalf("%d knights after the mate's spell at it, want 0", n)
	}
	saysNone(t, "the mate's spell", w.advance(t, time.Second))

	w.talk(t, "tablet3")
	wantLines(t, "7-1 after", w.sagaEvent(t, "7-1"), w.sagaPage(t, "tablet3", "7-02.htm"))
	if n := len(npcsOf(w.srv, corruptedID)); n != 1 {
		t.Fatalf("%d knights after the tablet was freed, want 1", n)
	}
}

// TestQ070FourSepulchersArchonCreditsTheParty: an Archon of Halisha killed
// in the Four Sepulchers by a party gives every member of the saga's class
// at cond 15 the fifth amulet, taking all of its marks.
func TestQ070FourSepulchersArchonCreditsTheParty(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 15, map[int32]int32{halishaMark: 50})
	mate, _ := sagaMate(t, w, 15, true, func(objID int32) { w.srv.GiveItem(t, objID, halishaMark, 30) })
	archon := w.spawnHostile(t, "archon", sepulchersArchonID, "Monster", 40)

	killOnPlayerQueue(t, w.srv, w.player, archon)
	for _, objID := range []int32{w.player, mate} {
		if vars := w.srv.QuestVars(t, objID, q070Name); vars["<cond>"] != "16" {
			t.Fatalf("player %d: vars %v, want cond 16", objID, vars)
		}
		if n := w.srv.PlayerItemCount(t, objID, amulet5th); n != 1 {
			t.Fatalf("player %d: fifth amulets %d, want 1", objID, n)
		}
		if n := w.srv.PlayerItemCount(t, objID, halishaMark); n != 0 {
			t.Fatalf("player %d: marks %d, want 0", objID, n)
		}
	}
}

// TestQ070FourSepulchersArchonSkipsAnotherClass: a party member at cond 15
// whose class is not the saga's gets nothing from the archon's death; the
// killer of the saga's class does.
func TestQ070FourSepulchersArchonSkipsAnotherClass(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 15, nil)
	mate, _ := sagaMate(t, w, 15, true, func(objID int32) {
		if _, err := w.srv.DB.ExecContext(context.Background(), "UPDATE characters SET classid = ?, base_class = ? WHERE obj_Id = ?",
			knightClass, knightClass, objID); err != nil {
			t.Fatal(err)
		}
		w.srv.GiveItem(t, objID, halishaMark, 30)
	})
	archon := w.spawnHostile(t, "archon", sepulchersArchonID, "Monster", 40)

	killOnPlayerQueue(t, w.srv, w.player, archon)
	if vars := w.srv.QuestVars(t, w.player, q070Name); vars["<cond>"] != "16" {
		t.Fatalf("the killer: vars %v, want cond 16", vars)
	}
	if vars := w.srv.QuestVars(t, mate, q070Name); vars["<cond>"] != "15" {
		t.Fatalf("the Knight: vars %v, want cond 15", vars)
	}
	if n := w.srv.PlayerItemCount(t, mate, amulet5th); n != 0 {
		t.Fatalf("the Knight's fifth amulets %d, want 0", n)
	}
	if n := w.srv.PlayerItemCount(t, mate, halishaMark); n != 30 {
		t.Fatalf("the Knight's marks %d, want 30 kept", n)
	}
}
