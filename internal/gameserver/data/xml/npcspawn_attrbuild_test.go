package xml

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// npcSpawnBuilders covers the npc template and spawnlist element builders
// that replaced the npc.New*/spawn.New* StatSet constructors (#3470): one
// valid attribute set each, the attributes that must be present, and the
// integer ones that must parse.
var npcSpawnBuilders = []struct {
	name     string
	valid    attrSet
	required []string
	ints     []string
	build    func(*attrValues) error
}{
	{
		name:     "npc private",
		valid:    attrSet{"id": "20001", "weight": "1", "respawn": "30sec"},
		required: []string{"id", "weight", "respawn"},
		ints:     []string{"id", "weight"},
		build:    func(a *attrValues) error { _, err := buildNPCPrivate(a); return err },
	},
	{
		name:     "pet data",
		valid:    attrSet{"food1": "2515", "food2": "0", "autoFeedLimit": "0.55", "hungryLimit": "0.5", "unsummonLimit": "0.4"},
		required: []string{"food1", "food2", "autoFeedLimit", "hungryLimit", "unsummonLimit"},
		ints:     []string{"food1", "food2"},
		build:    func(a *attrValues) error { _, err := buildPetData(a, nil); return err },
	},
	{
		name: "pet level stats",
		valid: attrSet{
			"exp": "0", "maxMeal": "248", "expType": "-1", "mealInBattle": "2", "mealInNormal": "2",
			"pAtk": "2.1", "pDef": "11.1", "mAtk": "1.4", "mDef": "8.1", "hp": "19.8", "mp": "20",
			"hpRegen": "2", "mpRegen": "0.9", "ssCount": "1", "spsCount": "1",
		},
		required: []string{"exp", "maxMeal", "expType", "mealInBattle", "mealInNormal", "pAtk", "pDef", "mAtk", "mDef", "hp", "mp", "hpRegen", "mpRegen", "ssCount", "spsCount"},
		ints:     []string{"exp", "maxMeal", "expType", "mealInBattle", "mealInNormal", "ssCount", "spsCount", "mealInBattleOnRide", "mealInNormalOnRide", "speedOnRide"},
		build:    func(a *attrValues) error { _, err := buildPetLevelStats(a); return err },
	},
	{
		name:     "spawn private",
		valid:    attrSet{"id": "20001", "weight": "1"},
		required: []string{"id", "weight"},
		ints:     []string{"id", "weight"},
		build:    func(a *attrValues) error { _, err := buildSpawnPrivate(a); return err },
	},
}

// TestNPCSpawnBuildersRejectMalformedValues pins the accepted input set the
// builders inherited from the StatSet constructors (StatSet.getInteger and
// friends in the reference, Integer.parseInt based).
func TestNPCSpawnBuildersRejectMalformedValues(t *testing.T) {
	t.Parallel()
	for _, b := range npcSpawnBuilders {
		if err := b.build(b.valid.values()); err != nil {
			t.Errorf("%s: valid %v: %v", b.name, b.valid, err)
		}
		for _, key := range b.required {
			if err := b.build(b.valid.without(key).values()); err == nil {
				t.Errorf("%s: missing %s built, want an error", b.name, key)
			}
		}
		for _, key := range b.ints {
			for _, bad := range []string{"", " 1", "1 ", "x"} {
				if err := b.build(b.valid.with(key, bad).values()); err == nil {
					t.Errorf("%s: %s=%q built, want an error", b.name, key, bad)
				}
			}
		}
	}
}

// TestBuildSpawnPrivateRejectsInt32Overflow: the private's npc id still
// fails past int32, as StatSet.GetInt32 did.
func TestBuildSpawnPrivateRejectsInt32Overflow(t *testing.T) {
	t.Parallel()
	if _, err := buildSpawnPrivate(attrSet{"id": "2147483648", "weight": "1"}.values()); err == nil {
		t.Fatal("buildSpawnPrivate(id=2147483648) built, want an error")
	}
}

func TestBuildNPCPrivateKeepsIDInFieldError(t *testing.T) {
	t.Parallel()
	valid := attrSet{"id": "123", "weight": "1", "respawn": "1sec"}
	for _, missing := range []string{"weight", "respawn"} {
		_, err := buildNPCPrivate(valid.without(missing).values())
		if err == nil || !strings.Contains(err.Error(), "npc: private entry 123") {
			t.Errorf("missing %s: error = %v, want entry id", missing, err)
		}
	}
}

// TestBuildPetLevelStatsMountSpeeds: speedOnRide needs at least five values
// and fills the base, water and fly speeds from slots 0, 2 and 4; a row
// without it leaves the mount speeds at 0.
func TestBuildPetLevelStatsMountSpeeds(t *testing.T) {
	t.Parallel()
	row := npcSpawnBuilders[2].valid
	got, err := buildPetLevelStats(row.with("speedOnRide", "1;2;3;4;5;6").values())
	if err != nil {
		t.Fatalf("buildPetLevelStats() error: %v", err)
	}
	if got.MountBaseSpeed != 1 || got.MountWaterSpeed != 3 || got.MountFlySpeed != 5 {
		t.Fatalf("mount speeds = %d/%d/%d, want 1/3/5", got.MountBaseSpeed, got.MountWaterSpeed, got.MountFlySpeed)
	}
	if _, err := buildPetLevelStats(row.with("speedOnRide", "1;2;3;4").values()); err == nil {
		t.Fatal("buildPetLevelStats(speedOnRide with 4 values) built, want an error")
	}
	got, err = buildPetLevelStats(row.values())
	if err != nil || got.MountBaseSpeed != 0 || got.MountFlySpeed != 0 {
		t.Fatalf("buildPetLevelStats() without speedOnRide = %+v, %v", got, err)
	}
}

// TestBuildNPCTemplateSetOverridesDecodedLiteral: the <npc> id and
// idTemplate attributes decode as integer literals, and a <set> child of
// the same name still overrides the decoded value, read as plain decimal.
func TestBuildNPCTemplateSetOverridesDecodedLiteral(t *testing.T) {
	t.Parallel()
	el := npcElement{
		Attrs: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: "0x10"},
			{Name: xml.Name{Local: "idTemplate"}, Value: "010"},
			{Name: xml.Name{Local: "name"}, Value: "n"},
		},
		Sets: []setElem{
			{Name: "type", Val: "Monster"},
			{Name: "baseDamageRange", Val: "0;0;80;120"},
			{Name: "pAtk", Val: "1"},
			{Name: "mAtk", Val: "1"},
			{Name: "pDef", Val: "1"},
			{Name: "mDef", Val: "1"},
			{Name: "radius", Val: "1"},
			{Name: "height", Val: "1"},
		},
	}
	tpl, err := buildNPCTemplate(el, nil, nil, zerolog.Nop())
	if err != nil {
		t.Fatalf("buildNPCTemplate() error: %v", err)
	}
	if tpl.ID != 16 || tpl.TemplateID != 8 {
		t.Fatalf("id/idTemplate = %d/%d, want 16/8", tpl.ID, tpl.TemplateID)
	}

	el.Sets = append(el.Sets, setElem{Name: "idTemplate", Val: "010"})
	tpl, err = buildNPCTemplate(el, nil, nil, zerolog.Nop())
	if err != nil {
		t.Fatalf("buildNPCTemplate() with <set idTemplate> error: %v", err)
	}
	if tpl.TemplateID != 10 {
		t.Fatalf("idTemplate overridden by <set> = %d, want 10", tpl.TemplateID)
	}
	if tpl.Race != npc.Race(0) || tpl.Skills != nil {
		t.Fatalf("template without <skills> = race %v, skills %v", tpl.Race, tpl.Skills)
	}
}
