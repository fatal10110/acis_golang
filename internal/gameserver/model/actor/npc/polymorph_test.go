package npc

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestFixedNPCPolymorphView pins the form an item-placed decoration and a
// signet point are shown in: NpcInfo takes the form's template id, held
// items and body, keeping the NPC's own name and title; the stationary view
// never takes it; Morph(nil) shows the NPC as itself again.
func TestFixedNPCPolymorphView(t *testing.T) {
	own := func(id int, kind string) *Template {
		return &Template{
			ID: id, TemplateID: id, Type: kind, Name: "Own", RunSpeed: 20, WalkSpeed: 20,
			CollisionRadius: 8, CollisionHeight: 20, RightHand: 7, LeftHand: 8, UsingServerSideName: true,
		}
	}
	form := &Template{ID: 20001, TemplateID: 20002, Name: "Gremlin", RightHand: 57, LeftHand: 58, CollisionRadius: 10, CollisionHeight: 15}
	type morphing interface {
		Polymorph() *Template
		Morph(*Template)
		NPCInfoSnapshot() npcinfo.Snapshot
		ServerObjectInfoSnapshot() npcinfo.Snapshot
	}
	for _, tc := range []struct {
		name  string
		tmpl  *Template
		build func(*testing.T, *Template) morphing
	}{
		{"decoration", own(13006, "ChristmasTree"), func(t *testing.T, tmpl *Template) morphing {
			inst, err := NewInstance(7, tmpl)
			if err != nil {
				t.Fatal(err)
			}
			d, err := NewDecoration(inst, "Own title")
			if err != nil {
				t.Fatal(err)
			}
			d.Attach(DecorationRuntime{World: world.New()})
			d.Spawn(10, 10, 0, 0)
			return d
		}},
		{"effect point", own(13018, "EffectPoint"), func(t *testing.T, tmpl *Template) morphing {
			q := sim.NewInline(time.Unix(0, 0)).NewQueue("point")
			ep, err := NewEffectPoint(7, tmpl, 1, q, nil)
			if err != nil {
				t.Fatal(err)
			}
			ep.Attach(Runtime{World: world.New()})
			ep.Spawn(10, 10, 0, 0)
			return ep
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.build(t, tc.tmpl)
			itself := n.NPCInfoSnapshot()
			type shownLook struct {
				TemplateID, RightHand, LeftHand  int
				Name, Title                      string
				CollisionRadius, CollisionHeight float64
			}
			look := func(s npcinfo.Snapshot) shownLook {
				return shownLook{
					TemplateID: s.TemplateID, Name: s.Name, Title: s.Title, RightHand: s.RightHand, LeftHand: s.LeftHand,
					CollisionRadius: s.CollisionRadius, CollisionHeight: s.CollisionHeight,
				}
			}
			want := look(itself)
			want.TemplateID, want.RightHand, want.LeftHand = form.TemplateID, form.RightHand, form.LeftHand
			want.CollisionRadius, want.CollisionHeight = form.CollisionRadius, form.CollisionHeight

			n.Morph(form)
			if n.Polymorph() != form {
				t.Fatalf("form = %+v, want %+v", n.Polymorph(), form)
			}
			if got := look(n.NPCInfoSnapshot()); got != want {
				t.Fatalf("polymorphed NpcInfo look = %+v, want %+v", got, want)
			}
			if got, base := look(n.ServerObjectInfoSnapshot()), look(itself); got.TemplateID != base.TemplateID ||
				got.CollisionRadius != base.CollisionRadius || got.CollisionHeight != base.CollisionHeight {
				t.Fatalf("polymorphed stationary look = %+v, want the NPC's own %+v", got, base)
			}
			n.Morph(nil)
			if got := look(n.NPCInfoSnapshot()); n.Polymorph() != nil || got != look(itself) {
				t.Fatalf("unpolymorphed NpcInfo look = %+v, want %+v", got, look(itself))
			}
		})
	}
}
