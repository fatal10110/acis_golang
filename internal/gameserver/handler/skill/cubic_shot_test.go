package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// shotCubicCaster is a player cubic summoner holding spiritshot charges.
type shotCubicCaster struct {
	*fakeCubicSummoner
	dead    bool
	charged map[item.ShotKind]bool
}

func newShotCubicCaster(charged ...item.ShotKind) *shotCubicCaster {
	c := &shotCubicCaster{fakeCubicSummoner: newFakeCubicSummoner(true), charged: map[item.ShotKind]bool{}}
	for _, k := range charged {
		c.charged[k] = true
	}
	return c
}

func (*shotCubicCaster) Kind() actor.Kind          { return actor.KindPlayer }
func (c *shotCubicCaster) AlikeDead() bool         { return c.dead }
func (c *shotCubicCaster) SpiritshotCharged() bool { return c.charged[item.ShotSpirit] }

func (c *shotCubicCaster) BlessedSpiritshotCharged() bool         { return c.charged[item.ShotBlessedSpirit] }
func (c *shotCubicCaster) SetChargedShot(k item.ShotKind, v bool) { c.charged[k] = v }

// TestCubicSummonSpendsSpiritshot pins the spend that ends every cubic
// branch of L2SkillSummon.useSkill (L2SkillSummon.java:179): the blessed
// spiritshot when one is charged, else the plain one, written with the
// skill's static-reuse flag. A caster alike dead returns before anything
// (L2SkillSummon.java:110-111).
func TestCubicSummonSpendsSpiritshot(t *testing.T) {
	storm := modelskill.Definition{SkillType: "SUMMON", IsCubic: true}.WithExtra(modelskill.Extra{NpcID: int(cubic.Storm)})
	for _, tc := range []struct {
		name      string
		charged   []item.ShotKind
		static    bool
		mass      bool
		dead      bool
		wantSPS   bool
		wantBSPS  bool
		wantCubic bool
	}{
		{name: "single, blessed charged", charged: []item.ShotKind{item.ShotSpirit, item.ShotBlessedSpirit}, wantSPS: true, wantCubic: true},
		{name: "mass, plain charged", charged: []item.ShotKind{item.ShotSpirit}, mass: true, wantCubic: true},
		{name: "static reuse keeps the charge", charged: []item.ShotKind{item.ShotSpirit}, static: true, wantSPS: true, wantCubic: true},
		{name: "alike dead", charged: []item.ShotKind{item.ShotSpirit, item.ShotBlessedSpirit}, dead: true, wantSPS: true, wantBSPS: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newShotCubicCaster(tc.charged...)
			caster.dead = tc.dead
			def := storm
			def.StaticReuse = tc.static
			targets := []Actor{caster}
			if tc.mass {
				targets = append(targets, newFakeCubicSummoner(true))
			}
			cubicHandler{}.UseResult(Cast{Caster: caster, Skill: def, Targets: targets})

			if got := caster.added[cubic.Storm]; got != tc.wantCubic {
				t.Fatalf("caster cubic admitted = %v, want %v", got, tc.wantCubic)
			}
			if sps, bsps := caster.SpiritshotCharged(), caster.BlessedSpiritshotCharged(); sps != tc.wantSPS || bsps != tc.wantBSPS {
				t.Fatalf("spiritshot/blessed = %v/%v, want %v/%v", sps, bsps, tc.wantSPS, tc.wantBSPS)
			}
		})
	}
}
