package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// The int lookup reads the spawn's value before the template's, as an int
// read with a default: the template value is read first as the default of
// the spawn read, so a template value that is not an int gives the
// caller's default even when the spawn has a good value.
func TestResolveAIInt(t *testing.T) {
	tmpl := AIParams{"Shared": "10", "TemplateOnly": "20", "BadTemplate": "x1", "Signed": "+7"}
	spawn := AIParams{"Shared": "11", "SpawnOnly": "-3", "BadSpawn": "1.5", "BadTemplate": "4", "Huge": "2147483648"}
	cases := []struct {
		name string
		def  int32
		want int32
	}{
		{"Shared", 0, 11},
		{"TemplateOnly", 0, 20},
		{"SpawnOnly", 0, -3},
		{"Missing", 5, 5},
		{"Signed", 0, 7},
		{"BadSpawn", 9, 9},
		{"BadTemplate", 9, 9},
		{"Huge", 1, 1},
	}
	for _, c := range cases {
		if got := resolveAIInt(spawn, tmpl, c.name, c.def); got != c.want {
			t.Errorf("resolveAIInt(%q, %d) = %d, want %d", c.name, c.def, got, c.want)
		}
	}
	if got := resolveAIInt(nil, nil, "Any", 3); got != 3 {
		t.Errorf("resolveAIInt over no parameters = %d, want the default 3", got)
	}
}

func TestResolveAIString(t *testing.T) {
	tmpl := AIParams{"Shared": "template", "TemplateOnly": "t"}
	spawn := AIParams{"Shared": "spawn", "Empty": ""}
	cases := []struct{ name, def, want string }{
		{"Shared", "d", "spawn"},
		{"TemplateOnly", "d", "t"},
		{"Empty", "d", ""},
		{"Missing", "d", "d"},
	}
	for _, c := range cases {
		if got := resolveAIString(spawn, tmpl, c.name, c.def); got != c.want {
			t.Errorf("resolveAIString(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestResolveAISkill(t *testing.T) {
	tmpl := AIParams{"Buff": "1040-3", "Shadowed": "1-1"}
	spawn := AIParams{"Shadowed": "4001-2-9", "NoLevel": "4001", "Trailing": "4001-", "Bad": "a-1", "BadLevel": "1-b"}

	for name, want := range map[string]skill.Ref{
		"Buff":     {ID: 1040, Level: 3},
		"Shadowed": {ID: 4001, Level: 2},
	} {
		got, ok, err := resolveAISkill(spawn, tmpl, name)
		if err != nil || !ok || got != want {
			t.Errorf("resolveAISkill(%q) = %v, %v, %v; want %v", name, got, ok, err, want)
		}
	}
	if _, ok, err := resolveAISkill(spawn, tmpl, "Missing"); ok || err != nil {
		t.Errorf("resolveAISkill(Missing) = ok %v, err %v; want neither", ok, err)
	}
	for _, name := range []string{"NoLevel", "Trailing", "Bad", "BadLevel"} {
		if _, ok, err := resolveAISkill(spawn, tmpl, name); ok || err == nil {
			t.Errorf("resolveAISkill(%q) = ok %v, err %v; want an error", name, ok, err)
		}
	}
}
