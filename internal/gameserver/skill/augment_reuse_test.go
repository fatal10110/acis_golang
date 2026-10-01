package skill

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

type openGeo struct{}

func (openGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (openGeo) Height(_, _, z int) int16          { return int16(z) }
func (openGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (openGeo) Walkable(int, int, int) bool { return true }
func (openGeo) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

// TestAugmentationActiveSkillRedisabledOnEquip pins Augmentation.applyBonus
// for an active skill: equipping the augmented weapon while the skill's
// reuse timer still runs disables it until that timer's own expiry and
// reports the timers changed; with the timer run out, or for a passive
// skill, nothing is disabled and no timer change is reported.
func TestAugmentationActiveSkillRedisabledOnEquip(t *testing.T) {
	const (
		swordID   int32 = 50
		activeID  int32 = 3203
		passiveID int32 = 3256
	)
	// A shared reuse makes the reuse key differ from both the skill id and
	// its own id/level key.
	active := modelskill.Definition{ID: modelskill.ID(activeID), Level: 2, Activation: modelskill.ActivationActive, SharedReuse: &modelskill.Ref{ID: 9000, Level: 1}}
	passive := modelskill.Definition{ID: modelskill.ID(passiveID), Level: 1, Activation: modelskill.ActivationPassive}
	tmpl := &item.Template{ID: swordID, Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}}

	setup := func(t *testing.T, skillID int32, skillLevel int32) (*Persistence, *player.Character, *sim.Inline, *item.Instance) {
		t.Helper()
		clock := sim.NewInline(time.Unix(1_000_000, 0))
		c := &player.Character{ID: 1}
		live, err := creature.NewLive(location.Location{}, 0, openGeo{}, c)
		if err != nil {
			t.Fatal(err)
		}
		live.SetQueue(clock.NewQueue("augment"))
		c.Live = live
		inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{tmpl}))
		inst := inv.AddNew(swordID, 1, 100)
		if !inv.SetAugmentation(inst, item.Augmentation{Attributes: 1, SkillID: skillID, SkillLevel: skillLevel}) {
			t.Fatal("SetAugmentation failed")
		}
		inv.EquipItem(inst, tmpl)
		return NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{active, passive})), c, clock, inst
	}
	equip := func(t *testing.T, p *Persistence, c *player.Character, inst *item.Instance) (reported []SkillChange) {
		t.Helper()
		if _, _, err := p.EquipItemStatsReporting(c, inst, tmpl, func(ch SkillChange) { reported = append(reported, ch) }); err != nil {
			t.Fatalf("EquipItemStatsReporting: %v", err)
		}
		return reported
	}

	key := cast.ReuseKey(active)
	t.Run("reuse running", func(t *testing.T) {
		p, c, clock, inst := setup(t, activeID, 2)
		expiry := clock.Now().Add(40 * time.Second)
		// The timer alone, as the cast armed it, with nothing disabled yet.
		c.SetSkillReuse(modelskill.Ref{ID: modelskill.ID(activeID), Level: 2}, key, time.Minute, expiry)
		if c.SkillDisabled(key) {
			t.Fatal("skill disabled before equip")
		}
		got := equip(t, p, c, inst)
		if len(got) != 1 || !got[0].SkillsChanged || !got[0].TimersChanged {
			t.Fatalf("reported %+v, want one change with skills and timers changed", got)
		}
		if c.SkillLevel(int(activeID)) != 2 {
			t.Fatalf("SkillLevel = %d, want 2", c.SkillLevel(int(activeID)))
		}
		if !c.SkillDisabled(key) {
			t.Fatal("skill usable after equip with its reuse running")
		}
		clock.Advance(40*time.Second - time.Millisecond)
		if !c.SkillDisabled(key) {
			t.Fatal("skill usable before its original expiry")
		}
		clock.Advance(time.Millisecond)
		if c.SkillDisabled(key) {
			t.Fatal("skill still disabled at its original expiry")
		}
	})
	t.Run("reuse ended", func(t *testing.T) {
		p, c, clock, inst := setup(t, activeID, 2)
		c.SetSkillReuse(modelskill.Ref{ID: modelskill.ID(activeID), Level: 2}, key, time.Minute, clock.Now())
		got := equip(t, p, c, inst)
		if len(got) != 1 || !got[0].SkillsChanged || got[0].TimersChanged {
			t.Fatalf("reported %+v, want skills changed and timers unchanged", got)
		}
		if c.SkillDisabled(key) {
			t.Fatal("skill disabled with its reuse already ended")
		}
	})
	t.Run("passive skill", func(t *testing.T) {
		p, c, clock, inst := setup(t, passiveID, 1)
		pkey := cast.ReuseKey(passive)
		c.SetSkillReuse(modelskill.Ref{ID: modelskill.ID(passiveID), Level: 1}, pkey, time.Minute, clock.Now().Add(time.Minute))
		got := equip(t, p, c, inst)
		if len(got) != 1 || !got[0].SkillsChanged || got[0].TimersChanged {
			t.Fatalf("reported %+v, want skills changed and timers unchanged", got)
		}
		if c.SkillDisabled(pkey) {
			t.Fatal("passive skill disabled on equip")
		}
	})
}
