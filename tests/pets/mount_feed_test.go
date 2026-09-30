package pets

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The wyvern's level-1 feeding row from the shipped npc data
// (aCis_datapack/data/xml/npcs/12000-12999.xml, npc 12621: <petdata
// autoFeedLimit="0.55" hungryLimit="0.5"> <stat level="1" maxMeal="508"
// mealInBattle="4" mealInNormal="2" mealInBattleOnRide="20"
// mealInNormalOnRide="10" speedOnRide="250;250;140;140;250;250">). The
// fixture owner is level 1, so a wyvern it mounts eats from this row and
// carries its rider at its speeds.
const (
	wyvernRideSpeed  = 250
	wyvernWaterSpeed = 140
	wyvernFlySpeed   = 250

	wyvernMaxMeal          = 508
	wyvernMealInBattle     = 4
	wyvernMealInNormal     = 2
	wyvernRideMealInBattle = 20
	wyvernRideMealInNormal = 10
	wyvernAutoFeedLimit    = 0.55

	// The feed task's fixed period (Player.startFeed schedules FeedTask
	// every 10000 ms).
	mountFeedPeriod = 10 * time.Second

	// The client gauge scale Player.setCurrentFeed sends: meal * 10000 /
	// the meal one tick takes.
	gaugeScale = 10000 / wyvernRideMealInNormal
)

// fedWyvernTemplate is the wyvern with its feeding data. Its food is the
// fixture wolf food, a PetFoods item whose feed skill the pet skill table
// defines, so the auto-feed runs the real item handler path.
func fedWyvernTemplate() *npc.Template {
	tpl := wyvernTemplate()
	tpl.Pet = &npc.PetData{
		Food1: int(wolfFoodID), AutoFeedLimit: wyvernAutoFeedLimit, HungryLimit: 0.5, UnsummonLimit: 0.4,
		Levels: map[int]npc.PetLevelStats{1: {
			MaxMeal: wyvernMaxMeal, MealInBattle: wyvernMealInBattle, MealInNormal: wyvernMealInNormal,
			MountMealInBattle: wyvernRideMealInBattle, MountMealInNormal: wyvernRideMealInNormal,
			MountBaseSpeed: wyvernRideSpeed, MountWaterSpeed: wyvernWaterSpeed, MountFlySpeed: wyvernFlySpeed,
		}},
	}
	return tpl
}

// mountRestartTown is the one town point the rider's restart table holds.
var mountRestartTown = location.Location{X: 200, Y: 400, Z: -300}

// mountRestartTable covers the owner's spawn region with mountRestartTown.
func mountRestartTable() *restart.Table {
	return &restart.Table{Points: []restart.Point{{
		Name:   "TestTown",
		Points: []location.Location{mountRestartTown},
		MapRegions: []location.Point{{
			X: (0-world.MinX)/world.TileSize + world.TileXMin,
			Y: (0-world.MinY)/world.TileSize + world.TileYMin,
		}},
	}}}
}

// bootWyvernRider boots the owner with a wyvern collar and the fed wyvern,
// and mounts it. It returns the harness and the frames the mount sent from
// its Ride through the rider's UserInfo.
func bootWyvernRider(t *testing.T, seeds ...seedItem) (*petWorld, [][]byte) {
	t.Helper()
	return bootWyvernRiderOpts(t, nil, seeds...)
}

// bootWyvernRiderOpts is bootWyvernRider with extra boot options.
func bootWyvernRiderOpts(t *testing.T, extra []gameservertest.Option, seeds ...seedItem) (*petWorld, [][]byte) {
	t.Helper()
	h := bootOwnerWithCollarOpts(t, append([]gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), fedWyvernTemplate()})),
		gameservertest.WithRestartPoints(mountRestartTable()),
	}, extra...), append([]seedItem{{TemplateID: wyvernCollarID, Count: 1}}, seeds...)...)
	if !h.srv.DrivesClock() {
		t.Skip("counting 10-second feed ticks needs the driven clock")
	}
	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	ride := readUntilOpcode(t, h.client, serverpackets.OpcodeRide, "mount Ride")
	return h, append(ride[len(ride)-1:], readUntilOpcode(t, h.client, serverpackets.OpcodeUserInfo, "mounted UserInfo")...)
}

// feedGauge is one green SetupGauge frame.
type feedGauge struct{ current, max int32 }

// feedGauges returns the green SetupGauge frames among frames, in order.
func feedGauges(t *testing.T, frames [][]byte) []feedGauge {
	t.Helper()
	var gauges []feedGauge
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSetupGauge {
			continue
		}
		r := wire.NewReader(f[1:])
		if color := r.ReadInt32(); color != int32(serverpackets.GaugeGreen) {
			continue
		}
		gauges = append(gauges, feedGauge{r.ReadInt32(), r.ReadInt32()})
	}
	return gauges
}

func wantGauges(t *testing.T, what string, got []feedGauge, want ...feedGauge) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: feed gauges = %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: feed gauges = %v, want %v", what, got, want)
		}
	}
}

func fullGauge() feedGauge {
	return feedGauge{wyvernMaxMeal * gaugeScale, wyvernMaxMeal * gaugeScale}
}

func gaugeAt(meal int32) feedGauge { return feedGauge{meal * gaugeScale, wyvernMaxMeal * gaugeScale} }

// advanceTicks lets n feed periods pass and returns every frame they sent.
func (h *petWorld) advanceTicks(t *testing.T, n int) [][]byte {
	t.Helper()
	for range n {
		h.srv.Advance(t, mountFeedPeriod)
	}
	return drainFrames(t, h.client)
}

// riderState is the slice of the live rider the feed scenarios read.
type riderState interface {
	Mounted() bool
	Flying() bool
	Running() bool
	Die(attackable.Combatant) bool
}

func (h *petWorld) rider(t *testing.T) riderState {
	t.Helper()
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	r, ok := obj.(riderState)
	if !ok {
		t.Fatalf("world player %T does not expose its mount state", obj)
	}
	return r
}

// TestWyvernFeedDrainsEveryTenSeconds pins Player.mount/startFeed and
// FeedTask: after the Ride and UserInfo the rider is shown its mount's full
// green gauge twice (setCurrentFeed, then startFeed's own), and every 10
// seconds out of combat the mount eats its ride meal and the gauge is
// resent.
func TestWyvernFeedDrainsEveryTenSeconds(t *testing.T) {
	t.Parallel()
	h, mountFrames := bootWyvernRider(t)
	assertFrameOpcode(t, mountFrames[0], serverpackets.OpcodeRide, "Ride")
	start := drainFrames(t, h.client)
	if len(start) < 2 || start[0][0] != serverpackets.OpcodeSetupGauge || start[1][0] != serverpackets.OpcodeSetupGauge {
		t.Fatalf("frames after the mounted UserInfo = %d (first %#x), want the two feed gauges first", len(start), start[0][0])
	}
	wantGauges(t, "mount", feedGauges(t, start), fullGauge(), fullGauge())

	h.srv.Advance(t, mountFeedPeriod-time.Second)
	wantGauges(t, "before the first period", feedGauges(t, drainFrames(t, h.client)))

	wantGauges(t, "first period", feedGauges(t, h.advanceTicks(t, 1)), gaugeAt(wyvernMaxMeal-wyvernRideMealInNormal))
	// Reading the client lets the driven clock run on a little, so later
	// periods are checked as a run of meals, one per period.
	later := feedGauges(t, h.advanceTicks(t, 2))
	if len(later) < 2 {
		t.Fatalf("two more periods: feed gauges = %v, want at least two", later)
	}
	for i, g := range later {
		if want := gaugeAt(wyvernMaxMeal - int32(i+2)*wyvernRideMealInNormal); g != want {
			t.Fatalf("two more periods: feed gauges = %v, want one meal of %d less per period", later, wyvernRideMealInNormal)
		}
	}
}

// TestWyvernFeedInCombatTakesBattleMeal pins Player.getFeedConsume: a
// rider in attack stance pays the mount's unmounted mealInBattle per tick,
// not its ride battle meal, and the gauge is scaled by that meal
// (setCurrentFeed divides both values by the tick's meal). Once the stance
// times out the tick takes the ride meal again, on the ride scale.
func TestWyvernFeedInCombatTakesBattleMeal(t *testing.T) {
	t.Parallel()
	var nowMS atomic.Int64
	h, _ := bootWyvernRiderOpts(t, []gameservertest.Option{
		gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(nowMS.Load()) }),
	})
	drainFrames(t, h.client)

	// A flying rider cannot attack, so a monster's hit is what puts it in
	// attack stance.
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	attacker := h.srv.SpawnAttackingHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	drainUntilQuiet(t, h.client)
	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	rider, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world player %T is not an online character", obj)
	}
	attacker.DoAttack(t, rider)
	readUntilOpcode(t, h.client, serverpackets.OpcodeAutoAttackStart, "rider AutoAttackStart")
	if !rider.InCombat() || rider.Dead() {
		t.Fatalf("rider after the hit: in combat %v dead %v, want in combat and alive", rider.InCombat(), rider.Dead())
	}

	const battleScale = 10000 / wyvernMealInBattle
	wantGauges(t, "battle period", feedGauges(t, h.advanceTicks(t, 1)),
		feedGauge{(wyvernMaxMeal - wyvernMealInBattle) * battleScale, wyvernMaxMeal * battleScale})

	drainUntilQuiet(t, h.client)
	nowMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := h.srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	readUntilOpcode(t, h.client, serverpackets.OpcodeAutoAttackStop, "rider AutoAttackStop")

	wantGauges(t, "period after the stance timed out", feedGauges(t, h.advanceTicks(t, 1)),
		gaugeAt(wyvernMaxMeal-wyvernMealInBattle-wyvernRideMealInNormal))
}

// TestHungryWyvernEatsRiderFood pins FeedTask's auto-feed: once a tick
// leaves the gauge below autoFeedLimit of the max meal, the mount eats one
// unit of its food from the rider's inventory through PetFoods — the unit
// is used up, the feed skill plays, the gauge rises by the skill's feed
// value (capped at the max meal) — and the rider is told
// PET_TOOK_S1_BECAUSE_HE_WAS_HUNGRY with the food's name.
func TestHungryWyvernEatsRiderFood(t *testing.T) {
	t.Parallel()
	h, _ := bootWyvernRider(t, seedItem{TemplateID: wolfFoodID, Count: 2})
	drainFrames(t, h.client)

	// 508 - 10n < 508 * 0.55 = 279.4 first holds at n = 23 (meal 278).
	const hungryTick = 23
	frames := h.advanceTicks(t, hungryTick-1)
	if got := feedGauges(t, frames); len(got) != hungryTick-1 || got[len(got)-1] != gaugeAt(wyvernMaxMeal-(hungryTick-1)*wyvernRideMealInNormal) {
		t.Fatalf("gauges before hunger = %v, want %d ending at meal %d", got, hungryTick-1, wyvernMaxMeal-(hungryTick-1)*wyvernRideMealInNormal)
	}
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatal("mount ate before its gauge fell below the auto-feed limit")
		}
	}

	frames = h.advanceTicks(t, 1)
	var seq []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSetupGauge, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage:
			seq = append(seq, f[0])
		}
	}
	want := []byte{serverpackets.OpcodeSetupGauge, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSetupGauge, serverpackets.OpcodeSystemMessage}
	if string(seq) != string(want) {
		t.Fatalf("hungry tick frames = % x, want % x", seq, want)
	}
	wantGauges(t, "hungry tick", feedGauges(t, frames), gaugeAt(wyvernMaxMeal-hungryTick*wyvernRideMealInNormal), fullGauge())
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeMagicSkillUse:
			r := wire.NewReader(f[1:])
			if caster, target, skill, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != h.ownerID || target != h.ownerID || skill != wolfFeedSkill || level != 1 {
				t.Fatalf("feed MagicSkillUse = %d->%d skill %d-%d, want %d->%d skill %d-1", caster, target, skill, level, h.ownerID, h.ownerID, wolfFeedSkill)
			}
		case serverpackets.OpcodeSystemMessage:
			r := wire.NewReader(f[1:])
			if id, params, kind, item := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != serverpackets.SystemMessagePetTookS1BecauseHeWasHungry || params != 1 || kind != serverpackets.SystemMessageParamItemName || item != wolfFoodID {
				t.Fatalf("hungry message = id %d params %d type %d item %d, want %d with item %d", id, params, kind, item, serverpackets.SystemMessagePetTookS1BecauseHeWasHungry, wolfFoodID)
			}
		}
	}
	if got := h.ownerItemCount(t, wolfFoodID); got != 1 {
		t.Fatalf("wolf food left = %d, want 1 after one auto-feed", got)
	}
}

// TestStarvedWyvernThrowsRiderToTown pins FeedTask's out-of-feed branch: the
// tick that finds no more than one meal left empties the gauge, dismounts
// (empty gauge, SkillList, the Ride dismount, UserInfo), tells the rider
// OUT_OF_FEED_MOUNT_CANCELED and, since the wyvern was flying, takes the
// rider to the nearest town. The feed task is gone after that.
func TestStarvedWyvernThrowsRiderToTown(t *testing.T) {
	t.Parallel()
	h, _ := bootWyvernRider(t)
	drainFrames(t, h.client)
	if r := h.rider(t); !r.Mounted() || !r.Flying() {
		t.Fatal("rider not flying its wyvern after the collar")
	}

	// 508 - 10*50 = 8 is no more than one meal: tick 51 starves.
	const starveTick = 51
	frames := h.advanceTicks(t, starveTick-1)
	if got := feedGauges(t, frames); len(got) != starveTick-1 {
		t.Fatalf("gauges before starving = %d, want %d", len(got), starveTick-1)
	}
	if !h.rider(t).Mounted() {
		t.Fatal("rider thrown before the starving tick")
	}

	frames = h.advanceTicks(t, 1)
	var seq []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSetupGauge, serverpackets.OpcodeSkillList, serverpackets.OpcodeRide,
			serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeTeleportToLocation:
			seq = append(seq, f[0])
		}
	}
	want := []byte{
		serverpackets.OpcodeSetupGauge, serverpackets.OpcodeSetupGauge, serverpackets.OpcodeSkillList,
		serverpackets.OpcodeRide, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeTeleportToLocation,
	}
	if len(seq) < len(want) || string(seq[:len(want)]) != string(want) {
		t.Fatalf("starving tick frames = % x, want % x first", seq, want)
	}
	wantGauges(t, "starving tick", feedGauges(t, frames), gaugeAt(0), feedGauge{0, 0})
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeRide:
			r := wire.NewReader(f[1:])
			if id, action, kind, class := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != h.ownerID || action != 0 || kind != 0 || class != 1_000_000 {
				t.Fatalf("dismount Ride = %d/%d/%d/%d, want %d/0/0/1000000", id, action, kind, class, h.ownerID)
			}
		case serverpackets.OpcodeSystemMessage:
			assertStaticSystemMessage(t, f, serverpackets.SystemMessageOutOfFeedMountCanceled)
		case serverpackets.OpcodeTeleportToLocation:
			r := wire.NewReader(f[1:])
			if id, x, y := r.ReadInt32(), int(r.ReadInt32()), int(r.ReadInt32()); id != h.ownerID || abs(x-mountRestartTown.X) > 25 || abs(y-mountRestartTown.Y) > 25 {
				t.Fatalf("teleport = %d to (%d,%d), want %d near the town (%d,%d)", id, x, y, h.ownerID, mountRestartTown.X, mountRestartTown.Y)
			}
		}
	}
	if r := h.rider(t); r.Mounted() || r.Flying() {
		t.Fatalf("rider after starving: mounted %v flying %v, want neither", r.Mounted(), r.Flying())
	}
	if got := feedGauges(t, h.advanceTicks(t, 2)); len(got) != 0 {
		t.Fatalf("feed gauges after the dismount = %v, want none", got)
	}
}

// TestRiderDeathStopsFeedAndReviveRefills pins Player.doDie's stopFeed and
// doRevive's startFeed: a dead rider's mount stops eating, and the revive
// fills its gauge again (startFeed with no summon sets the max meal), shows
// it twice and restarts the 10-second task.
func TestRiderDeathStopsFeedAndReviveRefills(t *testing.T) {
	t.Parallel()
	h, _ := bootWyvernRider(t)
	drainFrames(t, h.client)
	wantGauges(t, "first period", feedGauges(t, h.advanceTicks(t, 1)), gaugeAt(wyvernMaxMeal-wyvernRideMealInNormal))

	if !h.rider(t).Die(nil) {
		t.Fatal("Die() = false for the living rider")
	}
	drainFrames(t, h.client)
	if got := feedGauges(t, h.advanceTicks(t, 3)); len(got) != 0 {
		t.Fatalf("feed gauges while dead = %v, want none", got)
	}

	h.client.Send(encodeRequestRestartPoint(0))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "restart teleport")
	wantGauges(t, "revive", feedGauges(t, frames), fullGauge(), fullGauge())
	if !h.rider(t).Mounted() {
		t.Fatal("revived rider lost its mount")
	}
	drainFrames(t, h.client)
	wantGauges(t, "first period after the revive", feedGauges(t, h.advanceTicks(t, 1)), gaugeAt(wyvernMaxMeal-wyvernRideMealInNormal))
}

// TestMountedPlayerKeepsMoveType pins RequestChangeMoveType's and
// RequestActionUse action 1's mounted gate: a rider's run/walk toggle
// changes nothing and sends nothing, while the same toggles before mounting
// still switch the stance.
func TestMountedPlayerKeepsMoveType(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), wyvernTemplate()})),
	}, seedItem{TemplateID: wyvernCollarID, Count: 1})

	h.client.Send(encodeRequestChangeMoveType(false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeMoveType, "unmounted walk ChangeMoveType")
	h.client.Send(encodeRequestActionUse(1, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeMoveType, "unmounted run ChangeMoveType")
	drainUntilQuiet(t, h.client)
	if !h.rider(t).Running() {
		t.Fatal("owner not running after the unmounted toggles")
	}

	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeRide, "mount Ride")
	readUntilOpcode(t, h.client, serverpackets.OpcodeUserInfo, "mounted UserInfo")
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeRequestChangeMoveType(false))
	h.client.Send(encodeRequestActionUse(1, false))
	h.syncOnSkillList(t)
	if extra := drainFrames(t, h.client); len(extra) != 0 {
		t.Fatalf("mounted toggles sent %d frames (first opcode %#x), want none", len(extra), extra[0][0])
	}
	if !h.rider(t).Running() {
		t.Fatal("mounted toggles changed the rider's stance")
	}
}

func encodeRequestChangeMoveType(run bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeMoveType)
	w.WriteInt32(wire.BoolInt32(run))
	return w.Bytes()
}

func encodeRequestRestartPoint(requestType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRestartPoint)
	w.WriteInt32(requestType)
	return w.Bytes()
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
