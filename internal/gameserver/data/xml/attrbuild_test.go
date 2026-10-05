package xml

import (
	"path/filepath"
	"testing"
)

// attrSet is an element's attribute values for the builders below.
type attrSet map[string]string

func (s attrSet) with(key, value string) attrSet {
	out := make(attrSet, len(s)+1)
	for k, v := range s {
		out[k] = v
	}
	out[key] = value
	return out
}

func (s attrSet) without(key string) attrSet {
	out := make(attrSet, len(s))
	for k, v := range s {
		if k != key {
			out[k] = v
		}
	}
	return out
}

func (s attrSet) values() *attrValues { return newAttrValues(s, "") }

// attrBuilders covers every element builder that replaced a commons.StatSet
// constructor: one valid attribute set each, plus the attributes that must
// be present and the integer ones that must parse.
var attrBuilders = []struct {
	name     string
	valid    attrSet
	required []string
	ints     []string
	build    func(*attrValues) error
}{
	{
		name:     "access level",
		valid:    attrSet{"level": "7", "name": "Admin"},
		required: []string{"level", "name"},
		ints:     []string{"level", "childLevel"},
		build:    func(a *attrValues) error { _, err := buildAccessLevel(a); return err },
	},
	{
		name:     "admin command",
		valid:    attrSet{"name": "admin_ann", "accessLevel": "7"},
		required: []string{"name", "accessLevel"},
		ints:     []string{"accessLevel"},
		build:    func(a *attrValues) error { _, err := buildAdminCommand(a); return err },
	},
	{
		name:     "observer location",
		valid:    attrSet{"locId": "1", "x": "1", "y": "2", "z": "3", "yaw": "4", "pitch": "5", "cost": "6", "castle": "7"},
		required: []string{"locId", "x", "y", "z", "yaw", "pitch", "cost", "castle"},
		ints:     []string{"locId", "x", "y", "z", "yaw", "pitch", "cost", "castle"},
		build:    func(a *attrValues) error { _, err := buildObserverLocation(a); return err },
	},
	{
		name:     "observer spawn",
		valid:    attrSet{"id": "1", "x": "1", "y": "2", "z": "3", "groups": "1;2"},
		required: []string{"id", "x", "y", "z", "groups"},
		ints:     []string{"id", "x", "y", "z", "groups"},
		build:    func(a *attrValues) error { _, err := buildObserverSpawn(a); return err },
	},
	{
		name:     "static object",
		valid:    attrSet{"id": "1", "x": "1", "y": "2", "z": "3", "type": "0", "texture": "town_map", "mapX": "1", "mapY": "2"},
		required: []string{"id", "x", "y", "z", "type", "texture", "mapX", "mapY"},
		ints:     []string{"id", "x", "y", "z", "type", "mapX", "mapY"},
		build:    func(a *attrValues) error { _, err := buildStaticObject(a); return err },
	},
	{
		name:     "teleport",
		valid:    attrSet{"desc": "Gludio", "x": "1", "y": "2", "z": "3", "priceId": "57", "priceCount": "100"},
		required: []string{"desc", "x", "y", "z", "priceId", "priceCount"},
		ints:     []string{"x", "y", "z", "priceId", "priceCount", "castleId"},
		build:    func(a *attrValues) error { _, err := buildTeleport(a); return err },
	},
	{
		name:     "boat location",
		valid:    attrSet{"x": "1", "y": "2", "z": "3"},
		required: []string{"x", "y", "z"},
		ints:     []string{"x", "y", "z", "speed", "rotation", "busy"},
		build:    func(a *attrValues) error { _, err := buildBoatLocation(a); return err },
	},
	{
		name:     "walker location",
		valid:    attrSet{"x": "1", "y": "2", "z": "3"},
		required: []string{"x", "y", "z"},
		ints:     []string{"x", "y", "z", "delay", "fstring", "socialId"},
		build:    func(a *attrValues) error { _, err := buildWalkerLocation(a); return err },
	},
	{
		name: "manor seed",
		valid: attrSet{
			"id": "1", "seedId": "2", "matureId": "3", "level": "4", "reward1": "5",
			"reward2": "6", "seedsLimit": "7", "cropsLimit": "8",
		},
		required: []string{"id", "seedId", "matureId", "level", "reward1", "reward2", "seedsLimit", "cropsLimit"},
		ints:     []string{"id", "seedId", "matureId", "level", "reward1", "reward2", "seedsLimit", "cropsLimit"},
		build:    func(a *attrValues) error { _, err := buildSeed(a, 1); return err },
	},
	{
		name: "recipe",
		valid: attrSet{
			"id": "1", "material": "1864-1", "product": "1-1", "itemId": "1", "level": "1",
			"mpConsume": "1", "successRate": "100", "isDwarven": "true", "alias": "a",
		},
		required: []string{"id", "material", "product", "itemId", "level", "mpConsume", "successRate", "isDwarven", "alias"},
		ints:     []string{"id", "itemId", "level", "mpConsume", "successRate"},
		build:    func(a *attrValues) error { _, err := buildRecipe(a); return err },
	},
	{
		name:     "buylist product",
		valid:    attrSet{"id": "57"},
		required: []string{"id"},
		ints:     []string{"id", "price", "restockDelay", "count"},
		build:    func(a *attrValues) error { _, err := buildBuyListProduct(a, 1); return err },
	},
	{
		name: "armor set",
		valid: attrSet{
			"name": "set", "chest": "1", "legs": "2", "head": "3", "gloves": "4", "feet": "5",
			"skillId": "6", "shield": "7", "shieldSkillId": "8", "enchant6Skill": "9",
		},
		required: []string{"name", "chest", "legs", "head", "gloves", "feet", "skillId", "shield", "shieldSkillId", "enchant6Skill"},
		ints:     []string{"chest", "legs", "head", "gloves", "feet", "skillId", "shield", "shieldSkillId", "enchant6Skill"},
		build:    func(a *attrValues) error { _, err := buildArmorSet(a); return err },
	},
	{
		name:     "multisell ingredient",
		valid:    attrSet{"id": "57", "count": "1"},
		required: []string{"id", "count"},
		ints:     []string{"id", "count"},
		build:    func(a *attrValues) error { _, err := buildMultiSellIngredient(a, nil); return err },
	},
}

// TestAttrBuildersRejectMalformedValues pins the accepted input set the
// element builders inherited from the StatSet constructors they replace
// (StatSet.getInteger and friends in the reference, Integer.parseInt based):
// a valid element builds, a missing required attribute fails, and an empty,
// space-padded or non-numeric integer fails instead of reading as 0.
func TestAttrBuildersRejectMalformedValues(t *testing.T) {
	t.Parallel()
	for _, b := range attrBuilders {
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

// TestAttrBuildersRejectInt32Overflow: the int32 ids still fail past int32,
// as StatSet.GetInt32 did, instead of wrapping.
func TestAttrBuildersRejectInt32Overflow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		builder, key string
	}{
		{"recipe", "itemId"},
		{"buylist product", "id"},
		{"armor set", "chest"},
		{"multisell ingredient", "id"},
		{"observer spawn", "x"},
	} {
		for _, b := range attrBuilders {
			if b.name != tc.builder {
				continue
			}
			if err := b.build(b.valid.with(tc.key, "2147483648").values()); err == nil {
				t.Errorf("%s: %s=2147483648 built, want an error", tc.builder, tc.key)
			}
		}
	}
}

func TestBuildAccessLevelDefaults(t *testing.T) {
	t.Parallel()
	got, err := buildAccessLevel(attrSet{
		"level": "7", "name": "Admin", "childLevel": "6", "isGM": "true", "allowFixedRes": "true", "allowAltg": "TRUE",
	}.values())
	if err != nil {
		t.Fatalf("buildAccessLevel() error: %v", err)
	}
	if got.Level != 7 || got.Name != "Admin" || got.NameColor != "FFFFFF" || got.TitleColor != "FFFF77" || !got.IsGM ||
		got.ChildLevel != 6 || !got.AllowFixedRes || !got.AllowAltG || !got.AllowTransaction || !got.GiveDamage {
		t.Fatalf("buildAccessLevel() = %+v", got)
	}
}

// TestBuildAccessLevelRejectsUnreadableColor pins the color attributes to the
// reference's Integer.decode("0x" + value) (Java probe, OpenJDK 21.0.11): six
// hex digits read, while a sign, a second prefix, a value past int32 or a
// non-hex digit fails the access level.
func TestBuildAccessLevelRejectsUnreadableColor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		color string
		ok    bool
	}{
		{"CC3333", true},
		{"7FFFFFFF", true},
		{"80000000", false},
		{"-1", false},
		{"+1", false},
		{"0x10", false},
		{"GG0000", false},
		{"", false},
	} {
		for _, key := range []string{"nameColor", "titleColor"} {
			_, err := buildAccessLevel(attrSet{"level": "0", "name": "User", key: tc.color}.values())
			if (err == nil) != tc.ok {
				t.Errorf("buildAccessLevel(%s=%q) error = %v, want ok=%v", key, tc.color, err, tc.ok)
			}
		}
	}
}

func TestBuildAdminCommand(t *testing.T) {
	t.Parallel()
	got, err := buildAdminCommand(attrSet{
		"name": "admin_ann", "accessLevel": "7", "params": "message", "desc": "Broadcast the message.",
	}.values())
	if err != nil {
		t.Fatalf("buildAdminCommand() error: %v", err)
	}
	if got.Name != "admin_ann" || got.AccessLevel != 7 || got.Params != "message" || got.Description != "Broadcast the message." {
		t.Fatalf("buildAdminCommand() = %+v", got)
	}
}

// TestLoadAnnouncementsSchedule: an automatic announcement reads its
// integer-literal schedule and must state all three values (the reference
// unboxes an absent one and fails); a negative limit reads as 0; a manual
// one ignores the schedule attributes entirely.
func TestLoadAnnouncementsSchedule(t *testing.T) {
	t.Parallel()
	load := func(body string) ([]announcementResult, error) {
		path := filepath.Join(t.TempDir(), "announcements.xml")
		writeXMLFixture(t, path, `<?xml version="1.0" encoding="UTF-8"?><list>`+body+`</list>`)
		list, err := LoadAnnouncements(path)
		out := make([]announcementResult, len(list))
		for i, a := range list {
			out[i] = announcementResult{a.Message, a.Critical, a.Auto, a.InitialDelay, a.Delay, a.Limit}
		}
		return out, err
	}

	got, err := load(`<announcement message="Restart soon." critical="true" auto="true" initial_delay="0x3C" delay="300" limit="-5"/>` +
		`<announcement message="Manual." delay="bad"/>` +
		`<announcement message=""/>`)
	if err != nil {
		t.Fatalf("LoadAnnouncements() error: %v", err)
	}
	want := []announcementResult{{"Restart soon.", true, true, 60, 300, 0}, {"Manual.", false, false, 0, 0, 0}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("LoadAnnouncements() = %+v, want %+v", got, want)
	}

	for _, bad := range []string{
		`<announcement message="m" auto="true" initial_delay="1" delay="1"/>`,
		`<announcement message="m" auto="true" initial_delay="1" delay="" limit="1"/>`,
		`<announcement message="m" auto="true" initial_delay="x" delay="1" limit="1"/>`,
	} {
		if got, err := load(bad); err == nil {
			t.Errorf("LoadAnnouncements(%s) = %+v, want an error", bad, got)
		}
	}
}

type announcementResult struct {
	message             string
	critical, auto      bool
	initial, delay, lim int
}
