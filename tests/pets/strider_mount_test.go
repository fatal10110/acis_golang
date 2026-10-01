package pets

import (
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// striderNPCID is the Wind Strider, a pet a player can ride.
const striderNPCID = 12526

// actionMountDismount is the action-bar Mount/Dismount command.
const actionMountDismount = int32(38)

// dryadRootSkill is Dryad Root, a skill that roots its target.
const dryadRootSkill = modelskill.ID(1201)

// striderRow is one Wind Strider pet data row, as aCis_datapack ships it
// (data/xml/npcs/12000-12999.xml, npc 12526): the meals, the ride speeds
// (speedOnRide base and water) and pAtkOnRide (equal to mAtkOnRide).
type striderRow struct {
	level, maxMeal, mealInBattle, mealInNormal int
	exp                                        int64
	rideMeal, rideBattleMeal                   int
	run, swim                                  int
	rideAtk                                    float64
}

// The rows the scenarios summon the strider at, for a level 1 rider: 4,
// 6 and 20 levels above it. Row 21 rides faster than the lower ones, so a
// mount that read the wrong row would show it.
var (
	striderRow5  = striderRow{5, 932, 11, 2, 357, 1, 3, 130, 70, 7.6102756048229}
	striderRow7  = striderRow{7, 1200, 16, 3, 1481, 1, 5, 130, 70, 8.94296804305048}
	striderRow21 = striderRow{21, 1908, 25, 5, 177595, 1, 8, 140, 70, 25.2538835633486}
)

// striderTemplate is the Wind Strider with the rows above and its
// shipped hungry limit, radius and height. It eats the fixture wolf food.
func striderTemplate() *npc.Template {
	tpl := wolfTemplate()
	tpl.ID, tpl.TemplateID, tpl.Name = striderNPCID, striderNPCID, "Wind Strider"
	tpl.CollisionRadius, tpl.CollisionHeight = 23, 31
	levels := map[int]npc.PetLevelStats{}
	for _, r := range []striderRow{striderRow5, striderRow7, striderRow21} {
		levels[r.level] = npc.PetLevelStats{
			MaxExp: r.exp, MaxHP: wolfMaxHP, MaxMP: wolfMaxMP, PAtk: 100, PDef: 90, MAtk: 20, MDef: 40, SSCount: 1,
			MaxMeal: r.maxMeal, MealInBattle: r.mealInBattle, MealInNormal: r.mealInNormal,
			MountMealInNormal: r.rideMeal, MountMealInBattle: r.rideBattleMeal,
			MountBaseSpeed: r.run, MountWaterSpeed: r.swim, MountAtkSpd: 350,
			MountPAtk: r.rideAtk, MountMAtk: r.rideAtk,
		}
	}
	tpl.Pet = &npc.PetData{
		Food1: int(wolfFoodID), AutoFeedLimit: 0.55, HungryLimit: 0.5, UnsummonLimit: 0.4,
		Levels: levels,
	}
	return tpl
}

// bootStriderOwner boots a level 1 owner whose collar calls the strider,
// saved at row's level with fed meal, and summons it beside the owner.
func bootStriderOwner(t *testing.T, row striderRow, fed int) (*petWorld, *summon.Actor) {
	t.Helper()
	items, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: wolfCollarID, NPCID: striderNPCID, SummonType: 1}})
	if err != nil {
		t.Fatalf("summon items: %v", err)
	}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{striderTemplate(), treeTemplate()})),
		gameservertest.WithSummonItems(items),
		gameservertest.WithRestartPoints(mountRestartTable()),
	})
	if !h.srv.DrivesClock() {
		t.Skip("counting 10-second feed ticks needs the driven clock")
	}
	if err := h.srv.Pets.Save(petCtx(), h.collarID, pet.State{
		Level: row.level, Exp: row.exp, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: fed,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	strider, _ := h.spawnWolf(t)
	if got := strider.Level(); got != row.level {
		t.Fatalf("strider level = %d, want %d", got, row.level)
	}
	drainUntilQuiet(t, h.client)
	return h, strider
}

// mountStrider sends request and returns its answer: every frame up to and including
// the PetDelete of the pet that left, and any that follow.
func (h *petWorld) mountStrider(t *testing.T, request []byte) [][]byte {
	t.Helper()
	h.client.Send(request)
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "strider PetDelete")
	return append(frames, drainFrames(t, h.client)...)
}

// opcodeAfter is the index of the first frame with opcode op at or after
// from, or -1.
func opcodeAfter(frames [][]byte, from int, op byte) int {
	i := slices.IndexFunc(frames[from:], func(f []byte) bool { return f[0] == op })
	if i < 0 {
		return -1
	}
	return from + i
}

// TestStriderMountCommand pins Player.mountPlayer's strider branch and
// mount(Summon) (Player.java:3517-3540, 3570-3615) through /mount: the
// rider's skill list, then the feed gauge started from the pet's own meal
// (setCurrentFeed, then startFeed's own), the Ride of mount type 1, the
// rider's new UserInfo, and last the pet leaves (PetDelete) with its row
// saved. The rider then rides the strider.
func TestStriderMountCommand(t *testing.T) {
	t.Parallel()
	h, strider := bootStriderOwner(t, striderRow5, striderRow5.maxMeal)
	fed := strider.Fed()
	frames := h.mountStrider(t, encodeUserCommand(cmdMount))

	skills := opcodeAfter(frames, 0, serverpackets.OpcodeSkillList)
	if skills < 0 {
		t.Fatalf("mount frames = %x, want a SkillList", frameOpcodes(frames))
	}
	ride := opcodeAfter(frames, skills, serverpackets.OpcodeRide)
	if ride < 0 {
		t.Fatalf("mount frames = %x, want a Ride after the SkillList", frameOpcodes(frames))
	}
	scale := 10000 / int32(striderRow5.rideMeal)
	want := feedGauge{int32(fed) * scale, int32(striderRow5.maxMeal) * scale}
	if g := feedGauges(t, frames[skills:ride]); len(g) != 2 || g[0] != want || g[1] != want {
		t.Fatalf("gauges between SkillList and Ride = %v, want %v twice", g, want)
	}
	r := wire.NewReader(frames[ride][1:])
	if id, action, kind, class := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != h.ownerID || action != 1 || kind != 1 || class != striderNPCID+1_000_000 {
		t.Fatalf("Ride = %d action %d type %d class %d, want %d mounting type 1 class %d", id, action, kind, class, h.ownerID, striderNPCID+1_000_000)
	}
	info := opcodeAfter(frames, ride, serverpackets.OpcodeUserInfo)
	gone := opcodeAfter(frames, ride, serverpackets.OpcodePetDelete)
	if info < 0 || gone < info {
		t.Fatalf("mount frames = %x, want the UserInfo and then the PetDelete after the Ride", frameOpcodes(frames))
	}

	rider := h.character(t)
	if !rider.Mounted() || rider.MountNPCID() != striderNPCID || rider.MountType() != 1 || rider.MountObjectID() != h.collarID {
		t.Fatalf("rider mount = npc %d type %d collar %d, want the strider on collar %d", rider.MountNPCID(), rider.MountType(), rider.MountObjectID(), h.collarID)
	}
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("the strider stayed summoned")
	}
	if got := h.savedPetState(t).Fed; got != fed {
		t.Fatalf("pet row fed after the mount = %d, want the pet's %d", got, fed)
	}
}

// TestStriderMountTakesThePetsLevel pins mount(Summon)'s setMount and
// _petData at the pet's level (Player.java:3526-3529) through the mounted
// UserInfo: the run and swim speeds are the strider row's speedOnRide,
// halved when the strider is more than 9 levels above its rider, and P.Atk.
// and M.Atk. its pAtkOnRide / mAtkOnRide through POWER_ATTACK and
// MAGIC_ATTACK, scaled by 0.5 - (min(gap, 10) - 5) * 0.05 when it is more
// than 4 levels above (PlayerStatus.java:985-1031). The server moves the
// rider at the run speed the UserInfo shows.
func TestStriderMountTakesThePetsLevel(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		row       striderRow
		run, swim int32
		atkMul    float64
	}{
		{"4 levels above", striderRow5, 130, 70, 1},
		{"6 levels above", striderRow7, 130, 70, 0.45},
		{"20 levels above", striderRow21, 70, 35, 0.25},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, _ := bootStriderOwner(t, tt.row, tt.row.maxMeal)
			frames := h.mountStrider(t, encodeUserCommand(cmdMount))
			info := opcodeAfter(frames, opcodeAfter(frames, 0, serverpackets.OpcodeRide), serverpackets.OpcodeUserInfo)
			speeds := decodeRiderUserInfo(t, frames[info])
			if speeds.run != tt.run || speeds.swim != tt.swim {
				t.Fatalf("mounted UserInfo run/swim = %d/%d, want %d/%d", speeds.run, speeds.swim, tt.run, tt.swim)
			}
			rider := h.character(t)
			if got, want := rider.Move().Speed(), float64(float32(float64(tt.run)*speeds.moveMult)); math.Abs(got-want) > 1e-3 {
				t.Fatalf("simulated move speed = %v, want %v", got, want)
			}
			lvlMod := (100.0 - 11 + float64(rider.Level())) / 100.0
			intMod := statbonus.INTBonus[rider.INT()]
			base := tt.row.rideAtk * tt.atkMul
			want := userInfoAtk{
				pAtk:    int32(base * statbonus.STRBonus[rider.STR()] * lvlMod),
				mAtk:    int32(base * ((lvlMod * lvlMod) * (intMod * intMod))),
				castSpd: int32(333 * statbonus.WITBonus[rider.WIT()]),
			}
			if got := decodeUserInfoAtk(t, frames[info]); got != want {
				t.Fatalf("mounted UserInfo P.Atk./M.Atk./cast speed = %+v, want %+v", got, want)
			}
		})
	}
}

// TestStriderFeedGoesBackToThePet pins startFeed's summon branch and
// storePetFood (Player.java:3667-3705, 3755-3776): the rider's gauge starts
// at the strider's own meal and drains by its ride meal every 10 seconds;
// once below hungryLimit of its max meal the rider's cast speed base halves
// and /mount refuses the dismount (1008); /dismount gets off anyway, and
// the meal left is written back to the strider's row.
func TestStriderFeedGoesBackToThePet(t *testing.T) {
	t.Parallel()
	// 932 * 0.5 = 466: 467 rides, two ride meals of 1 later it is hungry.
	h, strider := bootStriderOwner(t, striderRow5, 467)
	fed := strider.Fed()
	if fed != 467 {
		t.Fatalf("strider fed before the mount = %d, want the saved 467", fed)
	}
	h.mountStrider(t, encodeUserCommand(cmdMount))
	rider := h.character(t)
	scale := int32(10000 / striderRow5.rideMeal)
	wantGauges(t, "two periods", feedGauges(t, h.advanceTicks(t, 2)),
		feedGauge{466 * scale, int32(striderRow5.maxMeal) * scale}, feedGauge{465 * scale, int32(striderRow5.maxMeal) * scale})
	if got, want := rider.MagicAttackSpeed(), int(166.5*statbonus.WITBonus[rider.WIT()]); got != want {
		t.Fatalf("hungry strider cast speed = %d, want %d (half the 333 base)", got, want)
	}

	frames := h.mountCommand(t, cmdMount)
	if len(frames) != 1 {
		t.Fatalf("/mount on a hungry strider = %x, want one refusal", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageHungryStriderNotMount)

	h.requireDismounted(t, "/dismount", h.mountCommand(t, cmdDismount))
	if got := h.savedPetState(t).Fed; got != 465 {
		t.Fatalf("pet row fed after the dismount = %d, want the 465 the rider left", got)
	}
}

// TestStriderFeedPatchesQueuedPetSave dismounts while the strider's row
// save from the mount is still queued: summoned again before the lane
// runs, the strider restores the meal its rider left, not the meal it had
// when it was mounted.
func TestStriderFeedPatchesQueuedPetSave(t *testing.T) {
	t.Parallel()
	h, strider := bootStriderOwner(t, striderRow5, striderRow5.maxMeal)
	fed := strider.Fed()
	h.srv.HoldPersistenceLane(t, h.collarID)
	h.mountStrider(t, encodeUserCommand(cmdMount))
	h.advanceTicks(t, 3)
	h.requireDismounted(t, "/dismount", h.mountCommand(t, cmdDismount))

	again, _ := h.spawnWolf(t)
	if got, want := again.Fed(), fed-3*striderRow5.rideMeal; got != want {
		t.Fatalf("strider summoned again fed = %d, want the %d its rider left", got, want)
	}
}

// TestStriderRiderLogoutStoresFeed pins Player.deleteMe's dismount: a rider
// who leaves gets off first, so the meal its strider had left is written
// back to the strider's row.
func TestStriderRiderLogoutStoresFeed(t *testing.T) {
	t.Parallel()
	h, strider := bootStriderOwner(t, striderRow5, striderRow5.maxMeal)
	fed := strider.Fed()
	h.mountStrider(t, encodeUserCommand(cmdMount))
	h.advanceTicks(t, 4)

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	readUntilOpcode(t, h.client, serverpackets.OpcodeLeaveWorld, "LeaveWorld")
	h.srv.AdvanceUntil(t, "owner left world", func() bool {
		_, ok := h.srv.State.Player(h.ownerID)
		return !ok
	})
	if got, want := h.savedPetState(t).Fed, fed-4*striderRow5.rideMeal; got != want {
		t.Fatalf("pet row fed after the logout = %d, want the %d its rider left", got, want)
	}
}

// TestStriderMountRefusals pins the strider branch's checks, each with its
// message, in the order mountPlayer runs them; the rider stays on foot and
// the strider stays out. An owner in combat puts its strider in combat
// too, so it is refused as a strider in battle (1011).
func TestStriderMountRefusals(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		fed   int
		setup func(t *testing.T, h *petWorld, strider *summon.Actor)
		want  int
	}{
		{"dead rider", striderRow5.maxMeal, func(t *testing.T, h *petWorld, _ *summon.Actor) {
			if !h.rider(t).Die(nil) {
				t.Fatal("Die() = false for the living rider")
			}
		}, serverpackets.SystemMessageStriderCantBeRiddenWhileDead},
		{"dead strider", striderRow5.maxMeal, func(t *testing.T, h *petWorld, strider *summon.Actor) {
			obj, _ := h.srv.State.Player(h.ownerID)
			strider.ReduceHP(strider.HP()+1, obj.(attackable.Combatant), modelskill.Definition{})
			h.srv.AdvanceUntil(t, "strider dead", strider.Dead)
		}, serverpackets.SystemMessageDeadStriderCantBeRidden},
		// A root on the strider alone, its owner out of combat, refuses it
		// as a strider in battle.
		{"rooted strider", striderRow5.maxMeal, func(t *testing.T, h *petWorld, strider *summon.Actor) {
			landFearPetEffect(t, strider, strider, "Root", dryadRootSkill, 1, 30)
			if !strider.Rooted() {
				t.Fatal("Rooted() = false after Root landed on the strider")
			}
			if h.character(t).InCombat() {
				t.Fatal("rider in combat; the case needs only the strider held")
			}
		}, serverpackets.SystemMessageStriderInBattleCantBeRidden},
		{"rider in combat", striderRow5.maxMeal, func(t *testing.T, h *petWorld, _ *summon.Actor) {
			h.srv.SetPlayerInCombat(t, h.ownerID, true)
		}, serverpackets.SystemMessageStriderInBattleCantBeRidden},
		{"sitting rider", striderRow5.maxMeal, func(t *testing.T, h *petWorld, _ *summon.Actor) {
			h.client.Send(encodeRequestActionUse(0, false))
			h.srv.AdvanceUntil(t, "rider sitting", func() bool { return !h.character(t).Standing() })
		}, serverpackets.SystemMessageStriderCanBeRiddenOnlyWhileStanding},
		{"fishing rider", striderRow5.maxMeal, func(t *testing.T, h *petWorld, _ *summon.Actor) {
			h.character(t).SetFishing(true)
		}, serverpackets.SystemMessageCannotDoWhileFishing2},
		{"strider out of reach", striderRow5.maxMeal, func(t *testing.T, h *petWorld, strider *summon.Actor) {
			// 200 past both radii (a player's and the strider's 23) is in
			// reach; this is well past it.
			x, y, z := h.srv.PlayerPosition(t, h.ownerID)
			placePet(t, strider, location.Location{X: x + 300, Y: y, Z: z})
		}, serverpackets.SystemMessageTooFarAwayFromStriderToMount},
		// 465 is below 932 * 0.5.
		{"hungry strider", 465, func(*testing.T, *petWorld, *summon.Actor) {}, serverpackets.SystemMessageHungryStriderNotMount},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, strider := bootStriderOwner(t, striderRow5, tt.fed)
			tt.setup(t, h, strider)
			drainUntilQuiet(t, h.client)
			frames := h.mountCommand(t, cmdMount)
			if len(frames) != 1 {
				t.Fatalf("/mount = %x, want one refusal", frameOpcodes(frames))
			}
			assertStaticSystemMessage(t, frames[0], tt.want)
			if h.rider(t).Mounted() {
				t.Fatal("refused rider mounted")
			}
			if _, ok := h.srv.State.Summon(h.ownerID); !ok {
				t.Fatal("refused mount sent the strider away")
			}
		})
	}
}

// TestActionMountDismount pins RequestActionUse action 38: after the
// action pre-checks it runs mountPlayer, mounting a summoned strider and
// then taking the rider off it. With nothing to mount or dismount, and for
// a dead player before the strider's own checks, the client is released
// with ActionFailed.
func TestActionMountDismount(t *testing.T) {
	t.Parallel()
	t.Run("mount and dismount", func(t *testing.T) {
		t.Parallel()
		h, _ := bootStriderOwner(t, striderRow5, striderRow5.maxMeal)
		h.mountStrider(t, encodeRequestActionUse(actionMountDismount, false))
		if !h.rider(t).Mounted() {
			t.Fatal("action 38 left the owner on foot")
		}
		h.client.Send(encodeRequestActionUse(actionMountDismount, false))
		h.requireDismounted(t, "action 38 on the strider", drainFrames(t, h.client))
		h.client.Send(encodeRequestActionUse(actionMountDismount, false))
		frames := drainFrames(t, h.client)
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("action 38 with no pet and no mount = %x, want ActionFailed", frameOpcodes(frames))
		}
	})
	t.Run("dead player", func(t *testing.T) {
		t.Parallel()
		h, _ := bootStriderOwner(t, striderRow5, striderRow5.maxMeal)
		if !h.rider(t).Die(nil) {
			t.Fatal("Die() = false for the living rider")
		}
		drainUntilQuiet(t, h.client)
		h.client.Send(encodeRequestActionUse(actionMountDismount, false))
		frames := drainFrames(t, h.client)
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("action 38 by a dead player = %x, want ActionFailed", frameOpcodes(frames))
		}
	})
}
