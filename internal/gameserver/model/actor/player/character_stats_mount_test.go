package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// Mount pet data rows from the shipped npc data
// (aCis_datapack/data/xml/npcs/12000-12999.xml): the Wind Strider (12526)
// at level 55 (maxMeal 4208, speedOnRide "150;150;70;70;0;0",
// atkSpdOnRide 350.0) and the Wyvern (12621) at level 70 (maxMeal 5728,
// speedOnRide "250;250;140;140;250;250"); both have hungryLimit 0.5. The
// wyvern's fly speed is bumped to 260 so the tests tell it from its run
// speed.
var (
	striderLevel55 = MountData{
		MaxMeal: 4208, MealInNormal: 40, MealInBattle: 43, HungryLimit: 0.5,
		RunSpeed: 150, SwimSpeed: 70, AtkSpd: 350,
	}
	wyvernLevel70 = MountData{
		MaxMeal: 5728, MealInNormal: 45, MealInBattle: 47, HungryLimit: 0.5,
		RunSpeed: 250, SwimSpeed: 140, FlySpeed: 260,
	}
)

const striderNPCID int32 = 12526

const riderDEX = 30

// mountDataFixture answers every mount with one row, recording the level
// it was asked for.
type mountDataFixture struct {
	npcs   map[int32]MountData
	levels []int
}

func (f *mountDataFixture) MountData(npcID int32, level int) (MountData, bool) {
	f.levels = append(f.levels, level)
	d, ok := f.npcs[npcID]
	return d, ok
}

// riderAt is a level-level character with class speeds 115/80/50 and the
// strider and wyvern rows above as its mounts' pet data.
func riderAt(t *testing.T, level int) (*Character, *mountDataFixture) {
	t.Helper()
	tmpl := combatTemplate()
	tmpl.DEX = riderDEX
	tmpl.RunSpeed, tmpl.WalkSpeed, tmpl.SwimSpeed = 115, 80, 50
	c := liveCharacter(1, tmpl, combatItems())
	c.CharLevel = level
	data := &mountDataFixture{npcs: map[int32]MountData{striderNPCID: striderLevel55, wyvernNPCID: wyvernLevel70}}
	c.mountData = data
	return c, data
}

type riderSpeeds struct{ run, walk, swim, atkSpd int }

func speedsOf(c *Character) riderSpeeds {
	return riderSpeeds{c.BaseRunSpeed(), c.BaseWalkSpeed(), c.BaseSwimSpeed(), c.AttackSpeed()}
}

func wantSpeeds(t *testing.T, when string, c *Character, want riderSpeeds) {
	t.Helper()
	if got := speedsOf(c); got != want {
		t.Fatalf("%s: base run/walk/swim, P.Atk. speed = %+v, want %+v", when, got, want)
	}
}

// setFeed puts the mount's gauge at meal.
func setFeed(c *Character, meal int) {
	c.mountFeed.mu.Lock()
	c.mountFeed.current = meal
	c.mountFeed.mu.Unlock()
}

// dexAtkSpd is the POWER_ATTACK_SPEED pipeline over base for the rider's
// DEX (FuncPAtkSpeed: base * DEX bonus), truncated as getPAtkSpd does.
func dexAtkSpd(base float64) int {
	return int(base * statbonus.DEXBonus[riderDEX])
}

// TestWyvernRiderSpeedsFollowTheWyvern pins PlayerStatus.getBaseRunSpeed,
// getBaseSwimSpeed and getPAtkSpd for a flying rider
// (PlayerStatus.java:894-926, 1034-1038): the base run speed is the
// wyvern's fly speed, the swim speed its water speed, both halved while it
// is hungry (fed below hungryLimit of its max meal); the walk speed stays
// the class template's (CreatureStatus.getBaseWalkSpeed is not
// overridden), and P.Atk. speed is a flat 300, 150 while hungry, with no
// stat pipeline.
func TestWyvernRiderSpeedsFollowTheWyvern(t *testing.T) {
	c, data := riderAt(t, 70)
	c.SetRunning(true)
	wantSpeeds(t, "on foot", c, riderSpeeds{115, 80, 50, dexAtkSpd(300)})

	if !c.Mount(wyvernNPCID, 77) {
		t.Fatal("Mount(wyvern) = false")
	}
	if len(data.levels) != 1 || data.levels[0] != 70 {
		t.Fatalf("pet data asked for levels %v, want [70], the rider's level", data.levels)
	}
	wantSpeeds(t, "mounted", c, riderSpeeds{260, 80, 140, 300})
	c.StartMountFeed()
	wantSpeeds(t, "fed", c, riderSpeeds{260, 80, 140, 300})

	// 5728 * 0.5 = 2864: hungry strictly below it.
	setFeed(c, 2864)
	wantSpeeds(t, "at the hungry limit", c, riderSpeeds{260, 80, 140, 300})
	setFeed(c, 2863)
	wantSpeeds(t, "hungry", c, riderSpeeds{130, 80, 70, 150})

	run := float64(float32(130 * statbonus.DEXBonus[riderDEX]))
	if got := c.RunSpeed(); got != run {
		t.Fatalf("hungry RunSpeed() = %v, want %v (the halved fly speed through RUN_SPEED)", got, run)
	}
	if got, want := c.MovementSpeedMultiplier(), float32(c.RunSpeed())/130; got != want {
		t.Fatalf("hungry MovementSpeedMultiplier() = %v, want %v (run speed over the halved base)", got, want)
	}
	if got, want := c.AttackSpeedMultiplier(), float32(1.1*150.0/300); got != want {
		t.Fatalf("hungry AttackSpeedMultiplier() = %v, want %v", got, want)
	}

	c.Dismount()
	wantSpeeds(t, "dismounted", c, riderSpeeds{115, 80, 50, dexAtkSpd(300)})
}

// TestStriderRiderSpeedsFollowTheStrider pins the riding branches: the base
// run speed is the strider's mountBaseSpeed (not its fly speed), and
// P.Atk. speed runs the strider's atkSpdOnRide, halved while hungry,
// through the POWER_ATTACK_SPEED stat (PlayerStatus.java:1040-1046). A
// strider mounts as ride type 1 (Ride.java:20-22).
func TestStriderRiderSpeedsFollowTheStrider(t *testing.T) {
	c, _ := riderAt(t, 55)
	if !c.Mount(striderNPCID, 88) {
		t.Fatal("Mount(strider) = false")
	}
	if got := c.MountType(); got != MountTypeStrider {
		t.Fatalf("MountType() = %d, want %d", got, MountTypeStrider)
	}
	if c.Flying() {
		t.Fatal("a strider rider flies")
	}
	c.StartMountFeed()
	wantSpeeds(t, "fed", c, riderSpeeds{150, 80, 70, dexAtkSpd(350)})

	setFeed(c, 2103) // below 4208 * 0.5 = 2104
	wantSpeeds(t, "hungry", c, riderSpeeds{75, 80, 35, dexAtkSpd(175)})
}

// TestMountOutlevelingItsRiderHalvesItsSpeeds pins the level-gap halving:
// a mount more than 9 levels above its rider carries it at half its base
// speeds, and hunger halves them again (integer halvings).
func TestMountOutlevelingItsRiderHalvesItsSpeeds(t *testing.T) {
	c, _ := riderAt(t, 70)
	c.Mount(wyvernNPCID, 77)
	c.StartMountFeed()

	c.CharLevel = 61 // 70 - 61 = 9
	wantSpeeds(t, "9 levels below the mount", c, riderSpeeds{260, 80, 140, 300})
	c.CharLevel = 60 // 70 - 60 = 10
	wantSpeeds(t, "10 levels below the mount", c, riderSpeeds{130, 80, 70, 300})
	setFeed(c, 0)
	wantSpeeds(t, "10 levels below a hungry mount", c, riderSpeeds{65, 80, 35, 150})
}

// TestFreshMountHungerReadsTheLastGauge pins Player.checkFoodState's
// _canFeed and _curFeed: _canFeed is only set by the first startFeed and
// never cleared, and the gauge survives a dismount, so until a new mount's
// feed starts, its hunger is judged by the gauge the last mount left. The
// first mount of a session is never hungry before its feed starts.
func TestFreshMountHungerReadsTheLastGauge(t *testing.T) {
	c, _ := riderAt(t, 70)
	c.Mount(wyvernNPCID, 77)
	wantSpeeds(t, "first mount before its feed", c, riderSpeeds{260, 80, 140, 300})

	c.StartMountFeed()
	setFeed(c, 0)
	c.Dismount()
	c.Mount(wyvernNPCID, 78)
	wantSpeeds(t, "remount over a starved gauge", c, riderSpeeds{130, 80, 70, 150})
	c.StartMountFeed()
	wantSpeeds(t, "remount once fed", c, riderSpeeds{260, 80, 140, 300})
}

// TestMountWithoutPetDataKeepsClassSpeeds covers a mount whose pet data has
// no row for its rider's level: the rider keeps its own speeds.
func TestMountWithoutPetDataKeepsClassSpeeds(t *testing.T) {
	c, data := riderAt(t, 70)
	delete(data.npcs, wyvernNPCID)
	c.Mount(wyvernNPCID, 77)
	wantSpeeds(t, "mounted without pet data", c, riderSpeeds{115, 80, 50, dexAtkSpd(300)})
}
