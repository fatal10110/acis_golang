package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	sqltest "github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// baseStats is the six base attributes in INT, STR, CON, MEN, DEX, WIT
// order, CharSelected's own field order.
type baseStats struct{ INT, STR, CON, MEN, DEX, WIT int32 }

// charSelectedStats decodes the six base attributes of a CharSelected frame
// (CharSelected.java:28-51: name, object id, title, session id, clan id, 0,
// sex, race, class, 1, x, y, z, HP, MP, SP, exp, level, karma, PK kills,
// then INT, STR, CON, MEN, DEX, WIT).
func charSelectedStats(t *testing.T, frame []byte) baseStats {
	t.Helper()
	if frame[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", frame[0])
	}
	r := wire.NewReader(frame[1:])
	r.ReadString()
	r.ReadInt32() // object id
	r.ReadString()
	for range 3 + 3 + 1 + 3 { // session, clan, 0; sex, race, class; 1; x, y, z
		r.ReadInt32()
	}
	r.ReadFloat64() // HP
	r.ReadFloat64() // MP
	r.ReadInt32()   // SP
	r.ReadInt64()   // exp
	for range 3 {   // level, karma, PK kills
		r.ReadInt32()
	}
	return baseStats{
		INT: r.ReadInt32(), STR: r.ReadInt32(), CON: r.ReadInt32(),
		MEN: r.ReadInt32(), DEX: r.ReadInt32(), WIT: r.ReadInt32(),
	}
}

// userInfoStats decodes the six base attributes of a UserInfo frame
// (UserInfo.java:32-47: location, heading, object id, name, race, sex,
// class, level, exp, then STR, DEX, CON, INT, WIT, MEN).
func userInfoStats(t *testing.T, frame []byte) baseStats {
	t.Helper()
	if frame[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("opcode = %#x, want UserInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64()
	var s baseStats
	s.STR, s.DEX, s.CON = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	s.INT, s.WIT, s.MEN = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	return s
}

// TestCharSelectedCarriesComputedBaseStats covers issue #4370: CharSelected
// writes the restored character's computed attributes
// (CharSelected.java:46-51 through PlayerStatus.getINT..getWIT), so the
// worn weapon's STR/DEX funcs and the known passive's INT/MEN funcs already
// show, and the values match the burst's UserInfo rather than the class
// template's base.
func TestCharSelectedCarriesComputedBaseStats(t *testing.T) {
	t.Parallel()
	const (
		swordID          int32 = 30
		swordObjectID    int32 = 600
		passiveID              = 3098
		strBonus, dexB         = 5, 2
		intBonus, menB         = 3, 4
		tmplSTR, tmplCON       = 40, 43
		tmplDEX, tmplINT       = 30, 21
		tmplWIT, tmplMEN       = 11, 25
	)
	class := gameservertest.ClassTemplate()
	class.STR, class.CON, class.DEX, class.INT, class.WIT, class.MEN = tmplSTR, tmplCON, tmplDEX, tmplINT, tmplWIT, tmplMEN

	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == swordID {
			rigged := *tmpl
			rigged.Modifiers = []item.StatModifier{
				{Op: item.FuncAdd, Stat: "STR", Value: strBonus},
				{Op: item.FuncAdd, Stat: "DEX", Value: dexB},
			}
			tmpl = &rigged
		}
		templates = append(templates, tmpl)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: passiveID, Level: 1, Activation: modelskill.ActivationPassive,
		Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "INT", Value: intBonus},
			{Op: modelskill.FuncAdd, Stat: "MEN", Value: menB},
		},
	}}), gamesql.NewCharacterSkillStore(db))

	srv := gameservertest.Boot(t,
		gameservertest.WithClassTemplate(class),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithSkills(skills),
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
			objID := seedCharacter(t, chars, "Sturdy", 5, 0)
			if err := items.Create(context.Background(), objID, item.Instance{
				ObjectID: swordObjectID, TemplateID: swordID, OwnerID: objID, Count: 1,
				Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
			}); err != nil {
				t.Fatalf("seed worn sword: %v", err)
			}
		}),
	)
	objID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, passiveID, 1); err != nil {
		t.Fatalf("grant passive: %v", err)
	}

	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	got := charSelectedStats(t, c.Read())
	want := baseStats{
		INT: tmplINT + intBonus, STR: tmplSTR + strBonus, CON: tmplCON,
		MEN: tmplMEN + menB, DEX: tmplDEX + dexB, WIT: tmplWIT,
	}
	if got != want {
		t.Fatalf("CharSelected stats = %+v, want %+v (template %d/%d/%d/%d/%d/%d plus worn and passive funcs)",
			got, want, tmplINT, tmplSTR, tmplCON, tmplMEN, tmplDEX, tmplWIT)
	}

	c.Send(encodeEnterWorld())
	frames := readUntil(t, c, serverpackets.OpcodeUserInfo)
	if user := userInfoStats(t, frames[len(frames)-1]); user != got {
		t.Fatalf("burst UserInfo stats = %+v, CharSelected sent %+v", user, got)
	}
}
