package player

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect/effecttest"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from character_cast_test.go ----
// spyCastController records InterruptCastOnDamage calls and StopCast/
// InterruptCast invocations, so a test can pin exactly what a damage- or
// actor-state-driven abort trigger passes onto the live cast controller
// without depending on the real cast package (which already imports this
// one).
type spyCastController struct {
	casting   bool
	magic     bool
	abortable bool

	damageCalls []spyDamageCall
	damageBreak bool

	stopCalls      int
	interruptCalls int
}

type spyDamageCall struct {
	damage       float64
	men          int
	attackCancel float64
	roll         int
	immune       bool
}

func (s *spyCastController) CastingNow() bool          { return s.casting }
func (s *spyCastController) CurrentSkillIsMagic() bool { return s.magic }
func (s *spyCastController) InterruptCast()            { s.interruptCalls++ }
func (s *spyCastController) StopCast()                 { s.stopCalls++ }
func (s *spyCastController) CanAbortCast() bool        { return s.abortable }

func (s *spyCastController) InterruptCastOnDamage(damage float64, men int, attackCancel func(float64) float64, roll int, immune bool) bool {
	cancelled := 0.0
	if attackCancel != nil {
		cancelled = attackCancel(0)
	}
	s.damageCalls = append(s.damageCalls, spyDamageCall{damage: damage, men: men, attackCancel: cancelled, roll: roll, immune: immune})
	return s.damageBreak
}

// ---- from character_cc_test.go ----
type ccGeo struct{}

func (ccGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (ccGeo) Height(_, _, _ int) int16          { return 0 }

// ccGeo never blocks in these tests, so pathfinding and fall-back queries
// never need a useful answer: return no path and reflect the origin.
func (ccGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (ccGeo) Walkable(int, int, int) bool                                 { return true }

func (g ccGeo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g ccGeo) ValidFlyLocation(ox, oy, oz int, _ float64, tx, ty, tz int) location.Location {
	return g.ValidLocation(ox, oy, oz, tx, ty, tz)
}

func (ccGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

// ccFleeTarget satisfies the flee hook a Fear effect's runtime needs, so it
// activates regardless of what its actual effected actor is.
type ccFleeTarget struct {
	world.Presence
	effecttest.Actor
}

func (*ccFleeTarget) FleeFrom(effector effect.Actor, distance int) {}

func attachTestLive(t *testing.T, c *Character) {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 0, ccGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	c.Live = live
}

func addCharacterEffect(t *testing.T, c *Character, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("effect.New(%q) error: %v", name, err)
	}
	e.Effected = &ccFleeTarget{}
	c.EffectList().Add(e)
	return e
}

// deathPenaltyKiller is a minimal killer double: a non-Player actor whose
// raid-relation is fixed at construction.
type deathPenaltyKiller struct {
	attackabletest.Combatant
	raidRelated bool
}

func (k deathPenaltyKiller) RaidRelated() bool { return k.raidRelated }

// ---- from character_forces_test.go ----
// permissiveGeo is a test-only move.Geo that permits every move, needed
// only because creature.NewLive requires a non-nil Geo.
type permissiveGeo struct{}

func (permissiveGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }
func (permissiveGeo) Height(x, y, z int) int16                { return int16(z) }
func (permissiveGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}

func (g permissiveGeo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g permissiveGeo) ValidFlyLocation(ox, oy, oz int, _ float64, tx, ty, tz int) location.Location {
	return g.ValidLocation(ox, oy, oz, tx, ty, tz)
}
func (permissiveGeo) Walkable(int, int, int) bool { return true }
func (permissiveGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

func withEffectList(t *testing.T, c *Character) *Character {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	c.Live = live
	return c
}

// ---- from character_reducehp_cc_test.go ----
// reduceHPPlayableAttacker is a minimal Playable-attacker stub for CP
// absorption tests (a distinct actor from the target, unlike self-damage).
type reduceHPPlayableAttacker struct {
	attackabletest.Combatant
}

func (reduceHPPlayableAttacker) ObjectID() int32 { return 99 }
func (reduceHPPlayableAttacker) Dead() bool      { return false }
func (reduceHPPlayableAttacker) Playable() bool  { return true }

// reduceHPNpcAttacker is a non-Playable attacker stub.
type reduceHPNpcAttacker struct {
	world.Presence
	effecttest.Actor
}

func (*reduceHPNpcAttacker) ObjectID() int32 { return 98 }
func (*reduceHPNpcAttacker) Dead() bool      { return false }
func (*reduceHPNpcAttacker) Playable() bool  { return false }

func rawPDef(c *Character) float64 {
	return c.calcStat(stat.PowerDefence, positiveTemplateStat(c.template().PDef))
}

func rawMDef(c *Character) float64 {
	return c.calcStat(stat.MagicDefence, positiveTemplateStat(c.template().MDef))
}

func positiveTemplateStat(v float64) float64 {
	if v > 0 {
		return v
	}
	return 1
}

func shieldDefenseItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: 3, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
		{ID: 4, Kind: item.KindEtcItem, Slot: item.SlotLHand, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
		{ID: 5, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorLight}},
	})
}

func equippedShield() *item.Instance {
	return &item.Instance{ObjectID: 30, TemplateID: 3, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand}
}

// ---- from character_stats_test.go ----
func closeFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// testModOwner returns a fresh, disposable stat-Mod owner identity for
// tests that need one only to attach a Mod, never to exercise
// RemoveStatsByOwner's identity matching.
func testModOwner() effect.ModOwner {
	return effect.ModOwnerEffect(&effect.Effect{})
}

// ---- from character_test.go ----
func humanFighterTemplate() *Template {
	return &Template{
		ID:        0,
		BaseLevel: 1,
		CON:       43,
		MEN:       25,
		HPTable:   []float64{80, 91.83},
		MPTable:   []float64{30, 35.46},
		CPTable:   []float64{32, 36.732},
		Spawns: []location.Location{
			{X: 1, Y: 2, Z: 3},
		},
	}
}

// ---- from fixtures_test.go ----
func zeroRoll(int) int { return 0 }

func combatTemplate() *Template {
	return &Template{
		ID: 0, FistsItemID: 1,
		STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
		PAtk: 5, PDef: 50,
		CollisionRadius: 9, CollisionHeight: 23,
		HPTable: []float64{100}, MPTable: []float64{30}, CPTable: []float64{0},
		Spawns: []location.Location{{X: 0, Y: 0, Z: 0}},
	}
}

func combatItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}, Modifiers: []item.StatModifier{
			{Op: item.FuncSet, Stat: "pAtk", Value: 5},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 300},
		}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalD, Weapon: &item.WeaponDetail{Type: item.WeaponSword, ReuseDelay: 1200, RandomDamage: 0}, Modifiers: []item.StatModifier{
			{Op: item.FuncSet, Stat: "pAtk", Value: 100},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 433},
			{Op: item.FuncSet, Stat: "rCrit", Value: 4},
		}},
		{ID: 3, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalD, Weapon: &item.WeaponDetail{
			Type: item.WeaponSword, SoulshotCount: 2, SpiritshotCount: 1,
		}},
	})
}

func liveCharacter(id int32, tmpl *Template, items *item.Table, equipped ...*item.Instance) *Character {
	c := &Character{
		ID: id, Name: "char",
		Race: RaceHuman, CharLevel: 1,
		Location: location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	c.SetClassID(tmpl.ID)
	c.SetBaseClassID(tmpl.ID)
	c.SetResourceValues(Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 30, CurrentMP: 30})
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, items, equipped))
	c.SetRollSource(zeroRoll)
	c.perfectShieldBlockRate = 5
	return c
}

// ---- from progression_test.go ----
// realPlayerLevelExp holds the requiredExpToLevelUp value for every level
// 1-81 of the shipped player level table (level 81 is the sentinel entry
// that closes level 80's experience band). Values generated by running the
// reference level-progression logic against the shipped data file.
var realPlayerLevelExp = []int64{
	0, 68, 363, 1168, 2884, 6038, 11287, 19423, 31378, 48229,
	71201, 101676, 141192, 191452, 254327, 331864, 426284, 539995, 675590, 835854,
	1023775, 1242536, 1495531, 1786365, 2118860, 2497059, 2925229, 3407873, 3949727, 4555766,
	5231213, 5981539, 6812472, 7729999, 8740372, 9850111, 11066012, 12395149, 13844879, 15422851,
	17137002, 18995573, 21007103, 23180442, 25524751, 28049509, 30764519, 33679907, 36806133, 40153995,
	45524865, 51262204, 57383682, 63907585, 70852742, 80700339, 91162131, 102265326, 114038008, 126509030,
	146307211, 167243291, 189363788, 212716741, 237351413, 271973532, 308441375, 346825235, 387197529, 429632402,
	474205751, 532692055, 606319094, 696376867, 804219972, 931275828, 1151275834, 1511275834, 2099275834, 4200000000,
	6299994999,
}

func realLevelTable(t *testing.T) *LevelTable {
	t.Helper()
	levels := make(map[int]Level, len(realPlayerLevelExp))
	for i, exp := range realPlayerLevelExp {
		levels[i+1] = Level{RequiredExpToLevelUp: exp}
	}
	table, err := NewLevelTable(levels)
	if err != nil {
		t.Fatalf("realLevelTable: %v", err)
	}
	return table
}

// progressionTrace lists, in order, the client-facing progression events rec
// saw.
func progressionTrace(rec *event.Recorder) []string {
	var out []string
	for _, e := range rec.Events() {
		switch e := e.(type) {
		case event.UserInfoChanged:
			out = append(out, "userinfo")
		case event.SPChanged:
			out = append(out, fmt.Sprintf("sp %d", e.SP))
		case event.ExpSPGained:
			out = append(out, fmt.Sprintf("gain %d/%d", e.Exp, e.SP))
		case event.ExpSPLost:
			out = append(out, fmt.Sprintf("lost %d/%d", e.Exp, e.SP))
		case event.LeveledUp:
			out = append(out, "leveledup")
		}
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// recordEvents attaches a Recorder as c's sink and returns it.
func recordEvents(c *Character) *event.Recorder {
	rec := &event.Recorder{}
	c.sink = rec
	return rec
}

// countVitals counts VitalsChanged events.
func countVitals(rec *event.Recorder) int {
	return event.Count[event.VitalsChanged](rec)
}

func (*reduceHPNpcAttacker) Kind() actor.Kind { return actor.KindNPC }

func (reduceHPPlayableAttacker) Kind() actor.Kind { return actor.KindPlayer }

func (reduceHPPlayableAttacker) Heading() int { return 0 }

func (reduceHPPlayableAttacker) Position() (x, y, z int) { return 0, 0, 0 }

func (deathPenaltyKiller) Heading() int { return 0 }

func (deathPenaltyKiller) Position() (x, y, z int) { return 0, 0, 0 }

// idleQueue is a queue on a virtual clock no test advances: timers armed on
// it never fire.
func idleQueue() *sim.Queue { return sim.NewInline(time.Unix(0, 0)).NewQueue("test") }

// attachIdleLive puts c in the world on a queue nothing advances, so the
// timers it arms (the charge auto-clear) never fire.
func attachIdleLive(t *testing.T, c *Character) *Character {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 0, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	c.Live = live
	return c
}
