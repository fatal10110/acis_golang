package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: AbstractNpcInfo.NpcInfo (AbstractNpcInfo.java:105-106) with
// npcs.properties ShowNpcLevel (Config.java:735, shipped False): the title
// of a Monster's NpcInfo becomes "Lv " + level + ("* " when its template's
// aggro range is positive, else " ") + the title it shows otherwise, the
// server-side title or "". Only a Monster (and its subclasses) gets it.

// npcInfoTitle decodes an NpcInfo frame's title.
func npcInfoTitle(t *testing.T, frame []byte) string {
	t.Helper()
	if frame[0] != serverpackets.OpcodeNPCInfo {
		t.Fatalf("frame %#x is no NpcInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	for range 18 { // object id .. walk speeds
		r.ReadInt32()
	}
	for range 4 { // speed multipliers, collision radius and height
		r.ReadFloat64()
	}
	for range 3 { // right hand, chest, left hand
		r.ReadInt32()
	}
	for range 5 { // name shown .. summon animation
		r.ReadUint8()
	}
	r.ReadString()
	return r.ReadString()
}

// levelTemplate is a still hostile fixture of kind and id, level 33, with
// aggro as its aggro range, showing title as its server-side title when
// title is set.
func levelTemplate(kind string, id, aggro int, title string) *npc.Template {
	tmpl := stillHostileTemplate(kind)
	tmpl.ID, tmpl.TemplateID, tmpl.Level, tmpl.AggroRange = id, id, 33, aggro
	if title != "" {
		tmpl.Title, tmpl.UsingServerSideTitle = title, true
	}
	return tmpl
}

// TestShowNPCLevelTitle pins the NpcInfo title a player is shown as each
// NPC enters its sight: with ShowNpcLevel a monster's starts with its level,
// starred for an aggressive one; a guard and a civilian keep theirs; without
// the switch every title is the plain one.
func TestShowNPCLevelTitle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                      string
		show                      bool
		aggressive, passive, kept string
	}{
		{"shown", true, "Lv 33* ", "Lv 33 Elite", "Gate Guard"},
		{"switched off", false, "", "Elite", "Gate Guard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var extra []gameservertest.Option
			if tc.show {
				extra = append(extra, gameservertest.WithShowNPCLevel())
			}
			w := bootFolkWorld(t, nil, extra...)
			for _, n := range []struct {
				what string
				tmpl *npc.Template
				dx   int
				want string
			}{
				{"aggressive monster", levelTemplate("Monster", 20101, 300, ""), -100, tc.aggressive},
				{"passive monster", levelTemplate("Monster", 20102, 0, "Elite"), -120, tc.passive},
				{"guard", levelTemplate("Guard", 30103, 300, "Gate Guard"), -140, tc.kept},
			} {
				h := w.srv.SpawnHostileNPCTemplateAt(t, n.tmpl, location.Location{X: w.at.X + n.dx, Y: w.at.Y, Z: w.at.Z})
				if got := npcInfoTitle(t, npcInfoOf(t, drainFrames(t, w.c), h.ObjectID())); got != n.want {
					t.Errorf("%s NpcInfo title = %q, want %q", n.what, got, n.want)
				}
			}
			merchant := folkTemplate("Merchant", merchantID)
			merchant.Title, merchant.UsingServerSideTitle = "Trader", true
			f, frames := w.spawnFolkFrames(t, merchant, 80)
			if got := npcInfoTitle(t, npcInfoOf(t, frames, f.ObjectID())); got != "Trader" {
				t.Errorf("merchant NpcInfo title = %q, want %q", got, "Trader")
			}
		})
	}
}

// TestShowNPCLevelTitleOnRefresh pins the level on the NpcInfo a monster is
// shown again with: a refreshed view starts with the level once.
func TestShowNPCLevelTitleOnRefresh(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithShowNPCLevel())
	h := w.srv.SpawnHostileNPCTemplateAt(t, levelTemplate("Monster", 20101, 300, "Elite"), location.Location{X: w.at.X - 100, Y: w.at.Y, Z: w.at.Z})
	drainFrames(t, w.c)

	done := make(chan struct{})
	if !h.Queue().Post(func() {
		defer close(done)
		h.UpdateAbnormalEffect()
	}) {
		t.Fatal("the monster's queue is closed")
	}
	<-done
	w.srv.Settle(t)
	if got := npcInfoTitle(t, npcInfoOf(t, drainFrames(t, w.c), h.ObjectID())); got != "Lv 33* Elite" {
		t.Fatalf("refreshed NpcInfo title = %q, want %q", got, "Lv 33* Elite")
	}
}
