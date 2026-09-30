package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// Mount pet data rows from the shipped npc data
// (aCis_datapack/data/xml/npcs/12000-12999.xml): the Wind Strider (12526)
// at level 55 (maxMeal 4208, speedOnRide "150;150;70;70;0;0",
// atkSpdOnRide 350.0, pAtkOnRide and mAtkOnRide 156.244534528007) and the
// Wyvern (12621) at level 70 (maxMeal 5728, speedOnRide
// "250;250;140;140;250;250", pAtkOnRide and mAtkOnRide 0.0); both have
// hungryLimit 0.5. The wyvern's fly speed is bumped to 260 so the tests
// tell it from its run speed.
var (
	striderLevel55 = MountData{
		MaxMeal: 4208, MealInNormal: 40, MealInBattle: 43, HungryLimit: 0.5,
		RunSpeed: 150, SwimSpeed: 70, AtkSpd: 350,
		PAtk: striderRideAtk, MAtk: striderRideAtk,
	}
	wyvernLevel70 = MountData{
		MaxMeal: 5728, MealInNormal: 45, MealInBattle: 47, HungryLimit: 0.5,
		RunSpeed: 250, SwimSpeed: 140, FlySpeed: 260,
	}
)

const (
	striderNPCID int32 = 12526
	// striderRideAtk is the level 55 Wind Strider's pAtkOnRide and
	// mAtkOnRide.
	striderRideAtk = 156.244534528007
)

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

// wantServerSpeed checks the live movement's speed, the pace the server
// moves the rider at, against want.
func wantServerSpeed(t *testing.T, when string, c *Character, want float64) {
	t.Helper()
	if got := c.Move().Speed(); got != want {
		t.Fatalf("%s: Move().Speed() = %v, want %v", when, got, want)
	}
}

// TestRiderServerMoveSpeedFollowsEveryMountChange pins that the server's
// movement is re-paced whenever the rider's base speed changes without a
// packet asking for it: a feed tick making the mount hungry, food lifting
// it back over its hungry limit, the rider leveling across the 9-level gap,
// and the mount throwing its starved rider.
func TestRiderServerMoveSpeedFollowsEveryMountChange(t *testing.T) {
	c, _ := riderAt(t, 70)
	attachIdleLive(t, c)
	c.SetRunning(true)
	c.Exp = realLevelTable(t).RequiredExpForLevel(70)
	foot := c.RunSpeed()
	c.refreshMoveSpeed()
	wantServerSpeed(t, "on foot", c, foot)

	c.Mount(wyvernNPCID, 77)
	c.StartMountFeed()
	fed := c.RunSpeed()
	if fed == foot {
		t.Fatalf("mounted RunSpeed() = %v, the foot speed", fed)
	}
	wantServerSpeed(t, "fed", c, fed)

	gen := func() uint64 {
		c.mountFeed.mu.Lock()
		defer c.mountFeed.mu.Unlock()
		return c.mountFeed.gen
	}

	// 2863 + 45 eaten leaves 2863, below the 2864 hungry limit.
	setFeed(c, 2863+wyvernLevel70.MealInNormal)
	c.tickMountFeed(gen())
	hungry := c.RunSpeed()
	if hungry == fed {
		t.Fatalf("hungry RunSpeed() = %v, the fed speed", hungry)
	}
	wantServerSpeed(t, "after the tick that made it hungry", c, hungry)

	c.AddMountFeed(1)
	wantServerSpeed(t, "after food lifted it to the hungry limit", c, fed)

	table := realLevelTable(t)
	c.AddLevel(table, nil, -10) // 70 - 60 = 10
	gapped := c.RunSpeed()
	if gapped == fed {
		t.Fatalf("RunSpeed() 10 levels below the mount = %v, the fed speed", gapped)
	}
	wantServerSpeed(t, "10 levels below the mount", c, gapped)
	c.AddLevel(table, nil, 1) // 70 - 61 = 9
	wantServerSpeed(t, "9 levels below the mount", c, fed)

	setFeed(c, wyvernLevel70.MealInNormal)
	c.tickMountFeed(gen())
	if c.Mounted() {
		t.Fatal("the starved mount kept its rider")
	}
	wantServerSpeed(t, "thrown by the starved mount", c, foot)
}

// javaLevelMod is CreatureStatus.getLevelMod (CreatureStatus.java:831-834).
func javaLevelMod(level int) float64 { return (100.0 - 11 + float64(level)) / 100.0 }

// TestRiderAttacksFromTheMountsAtk pins PlayerStatus.getPAtk and getMAtk
// for a rider (PlayerStatus.java:985-999, 1017-1031): the base is the
// mount's pAtkOnRide / mAtkOnRide at the level it was mounted at, scaled by
// 0.5 - (min(gap, 10) - 5) * 0.05 once the mount outlevels its rider by
// more than 4, then finalized through POWER_ATTACK (FuncPAtkMod: STR bonus
// and level mod) and MAGIC_ATTACK (FuncMAtkMod: squared INT bonus and
// level mod) at the rider's current level. The class template and the
// weapon play no part; getPAtk and getMAtk truncate to int.
func TestRiderAttacksFromTheMountsAtk(t *testing.T) {
	tmpl := combatTemplate()
	for _, tc := range []struct {
		gap int
		mul float64 // the level-gap multiplier, spelled out per gap
	}{
		{0, 1}, {4, 1}, {5, 0.5}, {7, 0.4}, {10, 0.25}, {12, 0.25},
	} {
		c, _ := riderAt(t, 55)
		if !c.Mount(striderNPCID, 88) {
			t.Fatal("Mount(strider) = false")
		}
		c.CharLevel = 55 - tc.gap
		lm := javaLevelMod(c.CharLevel)
		base := striderRideAtk * tc.mul

		wantP := base * statbonus.STRBonus[tmpl.STR] * lm
		if got := c.PAtk(); !closeFloat(got, wantP) || int(got) != int(wantP) {
			t.Fatalf("gap %d: PAtk() = %v, want %v (int %d)", tc.gap, got, wantP, int(wantP))
		}
		intMod := statbonus.INTBonus[tmpl.INT]
		wantM := base * ((lm * lm) * (intMod * intMod))
		if got := c.MAtk(); !closeFloat(got, wantM) || int(got) != int(wantM) {
			t.Fatalf("gap %d: MAtk() = %v, want %v (int %d)", tc.gap, got, wantM, int(wantM))
		}
	}
}

// TestRiderAtkLeavesWithTheMount covers the class P.Atk./M.Atk. on foot,
// their replacement on the mount, their return on dismount, and a wyvern
// whose pAtkOnRide/mAtkOnRide are 0: the stat pipeline's floor of 1
// (CreatureStatus.calcStat, cantBeNegative) is all its rider hits with.
func TestRiderAtkLeavesWithTheMount(t *testing.T) {
	c, _ := riderAt(t, 70)
	footP, footM := c.PAtk(), c.MAtk()

	c.Mount(striderNPCID, 88)
	if got := c.PAtk(); got == footP {
		t.Fatalf("strider rider PAtk() = %v, the foot value", got)
	}
	c.Dismount()
	if got, gotM := c.PAtk(), c.MAtk(); got != footP || gotM != footM {
		t.Fatalf("dismounted PAtk()/MAtk() = %v/%v, want the foot %v/%v", got, gotM, footP, footM)
	}

	c.Mount(wyvernNPCID, 77)
	if got, gotM := c.PAtk(), c.MAtk(); got != 1 || gotM != 1 {
		t.Fatalf("wyvern rider PAtk()/MAtk() = %v/%v, want 1/1", got, gotM)
	}
}

// TestMountWithoutPetDataKeepsClassAtk covers a mount with no pet data row
// for its rider's level: the rider keeps its own P.Atk., M.Atk. and cast
// speed.
func TestMountWithoutPetDataKeepsClassAtk(t *testing.T) {
	c, data := riderAt(t, 55)
	footP, footM, footC := c.PAtk(), c.MAtk(), c.MagicAttackSpeed()
	delete(data.npcs, striderNPCID)
	c.Mount(striderNPCID, 88)
	if p, m, cs := c.PAtk(), c.MAtk(), c.MagicAttackSpeed(); p != footP || m != footM || cs != footC {
		t.Fatalf("mounted without pet data PAtk/MAtk/C.Spd = %v/%v/%d, want the foot %v/%v/%d", p, m, cs, footP, footM, footC)
	}
}

// TestHungryMountHalvesCastSpeed pins PlayerStatus.getMAtkSpd
// (PlayerStatus.java:1002-1014): the 333 base is halved while the rider's
// mount is hungry (fed below hungryLimit of its max meal), then goes through
// MAGIC_ATTACK_SPEED (FuncMAtkSpeed: WIT bonus) and truncates to int. A fed
// mount and no mount leave it whole.
func TestHungryMountHalvesCastSpeed(t *testing.T) {
	wit := statbonus.WITBonus[combatTemplate().WIT]
	fed, hungry := int(333*wit), int(166.5*wit)
	if fed == hungry {
		t.Fatalf("fixture cannot tell fed %d from hungry %d", fed, hungry)
	}
	for _, npcID := range []int32{striderNPCID, wyvernNPCID} {
		c, _ := riderAt(t, 70)
		if got := c.MagicAttackSpeed(); got != fed {
			t.Fatalf("npc %d: on foot MagicAttackSpeed() = %d, want %d", npcID, got, fed)
		}
		c.Mount(npcID, 88)
		c.StartMountFeed()
		if got := c.MagicAttackSpeed(); got != fed {
			t.Fatalf("npc %d: fed mount MagicAttackSpeed() = %d, want %d", npcID, got, fed)
		}
		setFeed(c, 0)
		if got := c.MagicAttackSpeed(); got != hungry {
			t.Fatalf("npc %d: hungry mount MagicAttackSpeed() = %d, want %d", npcID, got, hungry)
		}
		c.Dismount()
		if got := c.MagicAttackSpeed(); got != fed {
			t.Fatalf("npc %d: dismounted MagicAttackSpeed() = %d, want %d", npcID, got, fed)
		}
	}
}
