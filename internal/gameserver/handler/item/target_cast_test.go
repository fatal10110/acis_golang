package item

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target/targettest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// targetCastUser is a TargetCastUser whose posture and target are set per
// case.
type targetCastUser struct {
	target    world.Tracked
	seated    bool
	disabled  bool
	moveSpeed float64
}

func (u targetCastUser) Target() world.Tracked  { return u.target }
func (u targetCastUser) Seated() bool           { return u.seated }
func (u targetCastUser) MovementDisabled() bool { return u.disabled }
func (u targetCastUser) MoveSpeed() float64     { return u.moveSpeed }

// targetCastNPC is a creature target: a chest, a feedable beast, or
// neither.
type targetCastNPC struct {
	world.Presence
	targettest.Actor
	chest, feedable, dead, claimed bool
}

func (n *targetCastNPC) ObjectID() int32           { return 9 }
func (n *targetCastNPC) Kind() actor.Kind          { return actor.KindNPC }
func (n *targetCastNPC) Position() (int, int, int) { return 0, 0, 0 }
func (n *targetCastNPC) Heading() int              { return 0 }
func (n *targetCastNPC) Dead() bool                { return n.dead }
func (n *targetCastNPC) AlikeDead() bool           { return n.dead }
func (n *targetCastNPC) Chest() bool               { return n.chest }
func (n *targetCastNPC) Interacted() bool          { return n.claimed }
func (n *targetCastNPC) FeedableBeast() bool       { return n.feedable }

// trackedOnly is a world object that is no creature, such as an item on
// the ground.
type trackedOnly struct{ world.Presence }

func (*trackedOnly) ObjectID() int32  { return 10 }
func (*trackedOnly) Kind() actor.Kind { return actor.KindItem }

func targetCastTemplate(handler string, skills ...modelitem.SkillRef) *modelitem.Template {
	return &modelitem.Template{
		Kind:           modelitem.KindEtcItem,
		EtcItem:        &modelitem.EtcItemDetail{Handler: handler},
		AttachedSkills: skills,
	}
}

// TestResolveTargetCastRefusals covers each refusal of the key, soul
// crystal and beast spice handlers, including the branches no shipped item
// reaches: a key with no skill, a user whose move speed is 0, a crystal
// whose first skill is not Soul Crystal or does not resolve, and a spice
// whose skill does not resolve.
func TestResolveTargetCastRefusals(t *testing.T) {
	t.Parallel()
	unlock := modelskill.Definition{ID: 2065, Level: 1}
	crystal := modelskill.Definition{ID: soulCrystalSkillID, Level: 1}
	other := modelskill.Definition{ID: 2097, Level: 1}
	spice := modelskill.Definition{ID: 2188, Level: 1}
	defs := fakeDefinitionTable{
		{ID: unlock.ID, Level: 1}:  unlock,
		{ID: crystal.ID, Level: 1}: crystal,
		{ID: other.ID, Level: 1}:   other,
		{ID: spice.ID, Level: 1}:   spice,
	}
	ref := func(def modelskill.Definition) modelitem.SkillRef {
		return modelitem.SkillRef{ID: int32(def.ID), Level: int32(def.Level)}
	}
	missing := modelitem.SkillRef{ID: 9999, Level: 1}
	chest := &targetCastNPC{chest: true}
	beast := &targetCastNPC{feedable: true}
	plain := &targetCastNPC{}
	standing := func(target world.Tracked) targetCastUser {
		return targetCastUser{target: target, moveSpeed: 120}
	}

	for _, tc := range []struct {
		name   string
		tmpl   *modelitem.Template
		user   targetCastUser
		want   TargetCastRefusal
		skills []modelskill.ID
		ctrl   bool
	}{
		{name: "key on chest", tmpl: targetCastTemplate(KeysHandler, ref(unlock)), user: standing(chest), skills: []modelskill.ID{unlock.ID}},
		{name: "key seated", tmpl: targetCastTemplate(KeysHandler, ref(unlock)), user: targetCastUser{target: chest, seated: true, moveSpeed: 120}, want: TargetCastSitting},
		{name: "key movement disabled", tmpl: targetCastTemplate(KeysHandler, ref(unlock)), user: targetCastUser{target: chest, disabled: true, moveSpeed: 120}, want: TargetCastSilent},
		{name: "key move speed zero", tmpl: targetCastTemplate(KeysHandler, ref(unlock)), user: targetCastUser{target: chest}, want: TargetCastSilent},
		{name: "key no target", tmpl: targetCastTemplate(KeysHandler, ref(unlock)), user: standing(nil), want: TargetCastInvalidTarget},
		{name: "key non-chest", tmpl: targetCastTemplate(KeysHandler, ref(unlock)), user: standing(plain), want: TargetCastInvalidTarget},
		{name: "key dead chest", tmpl: targetCastTemplate(KeysHandler, ref(unlock)), user: standing(&targetCastNPC{chest: true, dead: true}), want: TargetCastInvalidTarget},
		{name: "key claimed chest", tmpl: targetCastTemplate(KeysHandler, ref(unlock)), user: standing(&targetCastNPC{chest: true, claimed: true}), want: TargetCastInvalidTarget},
		{name: "key no skills", tmpl: targetCastTemplate(KeysHandler), user: standing(chest), want: TargetCastNoSkills},
		{name: "crystal on creature", tmpl: targetCastTemplate(SoulCrystalsHandler, ref(crystal)), user: standing(plain), skills: []modelskill.ID{crystal.ID}, ctrl: true},
		{name: "crystal no skills", tmpl: targetCastTemplate(SoulCrystalsHandler), user: standing(plain), want: TargetCastSilent},
		{name: "crystal first skill not soul crystal", tmpl: targetCastTemplate(SoulCrystalsHandler, ref(other), ref(crystal)), user: standing(plain), want: TargetCastSilent},
		{name: "crystal skill unresolved", tmpl: targetCastTemplate(SoulCrystalsHandler, missing), user: standing(plain), want: TargetCastSilent},
		{name: "crystal non-creature target", tmpl: targetCastTemplate(SoulCrystalsHandler, ref(crystal)), user: standing(&trackedOnly{}), want: TargetCastInvalidTarget},
		{name: "crystal no target", tmpl: targetCastTemplate(SoulCrystalsHandler, ref(crystal)), user: standing(nil), want: TargetCastInvalidTarget},
		{name: "spice on beast", tmpl: targetCastTemplate(BeastSpicesHandler, ref(spice)), user: standing(beast), skills: []modelskill.ID{spice.ID}},
		{name: "spice non-beast", tmpl: targetCastTemplate(BeastSpicesHandler, ref(spice)), user: standing(plain), want: TargetCastInvalidTarget},
		{name: "spice no skills", tmpl: targetCastTemplate(BeastSpicesHandler), user: standing(beast), want: TargetCastSilent},
		{name: "spice skill unresolved", tmpl: targetCastTemplate(BeastSpicesHandler, missing), user: standing(beast), want: TargetCastSilent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveTargetCast(tc.tmpl, tc.user, defs)
			if !got.Handled {
				t.Fatal("Handled = false, want true")
			}
			if got.Refusal != tc.want {
				t.Fatalf("Refusal = %d, want %d", got.Refusal, tc.want)
			}
			if got.CtrlForces != tc.ctrl {
				t.Fatalf("CtrlForces = %v, want %v", got.CtrlForces, tc.ctrl)
			}
			if tc.want != TargetCastAllowed {
				if len(got.Skills) != 0 {
					t.Fatalf("refused use carries skills %v", got.Skills)
				}
				return
			}
			if len(got.Skills) != len(tc.skills) {
				t.Fatalf("Skills = %v, want ids %v", got.Skills, tc.skills)
			}
			for i, id := range tc.skills {
				if got.Skills[i].ID != id {
					t.Fatalf("Skills[%d] = %d, want %d", i, got.Skills[i].ID, id)
				}
			}
		})
	}
}

// TestResolveTargetCastIgnoresOtherItems leaves every other item, and a
// missing user, to the rest of the use-item path.
func TestResolveTargetCastIgnoresOtherItems(t *testing.T) {
	t.Parallel()
	user := targetCastUser{target: &targetCastNPC{chest: true}, moveSpeed: 120}
	for name, tmpl := range map[string]*modelitem.Template{
		"nil template":  nil,
		"other handler": targetCastTemplate("ItemSkills", modelitem.SkillRef{ID: 2065, Level: 1}),
		"weapon":        {Kind: modelitem.KindWeapon},
	} {
		if got := ResolveTargetCast(tmpl, user, fakeDefinitionTable{}); got.Handled {
			t.Fatalf("%s: Handled = true, want false", name)
		}
	}
	if got := ResolveTargetCast(targetCastTemplate(KeysHandler), nil, fakeDefinitionTable{}); got.Handled {
		t.Fatal("nil user: Handled = true, want false")
	}
}
