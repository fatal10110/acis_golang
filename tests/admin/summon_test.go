package admin

import (
	"bytes"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: AdminSummon.useAdminCommand (//ride, //unride, //unsummon,
// //summon food|inventory|level), Player.mount(int, int), Player.dismount(),
// Summon.unSummon, Pet.setCurrentFed, GMViewItemList, and
// PlayableStatus.addExp(long)/removeExp(long) with PetStatus.addLevel and
// setLevel: //summon level hands a long, so the experience moves without
// the pet's exp message or status refresh.

const (
	summonStriderID = 12526
	// summonPetLevelUpAction is the level-up SocialAction id.
	summonPetLevelUpAction = 15
)

// summonPetTemplate is infoPetTemplate with a growth row for every level
// up to 81, as the datapack's pets have: level n starts at 100*(n-1)
// experience and holds 390+10*n HP. The pet is called at level 10.
func summonPetTemplate() *npc.Template {
	tmpl := infoPetTemplate()
	row := tmpl.Pet.Levels[10]
	levels := map[int]npc.PetLevelStats{}
	for level := 1; level <= 81; level++ {
		r := row
		r.MaxExp = int64(100 * (level - 1))
		r.MaxHP = float64(390 + 10*level)
		levels[level] = r
	}
	tmpl.Pet.Levels = levels
	return tmpl
}

// summonAdminOptions boots the pet, its collar and the skill the collar
// casts.
func summonAdminOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	collars, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: infoCollarID, NPCID: infoPetID, SummonType: 1}})
	if err != nil {
		t.Fatal(err)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: infoSummonSkil, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON_CREATURE", StaticHitTime: true, StaticReuse: true,
	}}), gamesql.NewCharacterSkillStore(db))
	return []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{summonPetTemplate()})),
		gameservertest.WithSummonItems(collars),
		gameservertest.WithSkills(skills),
	}
}

// callPet has ownerID, playing c, use collar and returns the pet it calls.
func callPet(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, ownerID, collar int32) *summon.Actor {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(collar)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	var pet *summon.Actor
	srv.AdvanceUntil(t, "pet in the world", func() bool {
		obj, ok := srv.State.Summon(ownerID)
		if ok {
			pet, ok = obj.(*summon.Actor)
		}
		return ok
	})
	srv.Settle(t)
	drain(t, c)
	return pet
}

// addSummoner seeds a second account's player holding a collar, enters the
// world with it and returns its client, object id and collar.
func addSummoner(t *testing.T, srv *gameservertest.Server, account, name string) (*testsupport.ScriptedClient, int32, int32) {
	t.Helper()
	ch := srv.SeedCharacterFor(t, account, name, 1, 0)
	collar := srv.GiveItem(t, ch.ID, infoCollarID, 1)
	c := srv.DialClient(t, account, 1)
	enterWorld(t, c)
	return c, ch.ID, collar
}

// liveCharacter is the online character objID.
func liveCharacter(t *testing.T, srv *gameservertest.Server, objID int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not online", objID)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d is %T", objID, obj)
	}
	return c
}

// onlinePlayer is the world object of the online player objID, what a
// click selects.
func onlinePlayer(t *testing.T, srv *gameservertest.Server, objID int32) world.Tracked {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not online", objID)
	}
	return obj
}

// onQueue runs fn on objID's queue and waits for it.
func onQueue(t *testing.T, srv *gameservertest.Server, objID int32, fn func()) {
	t.Helper()
	done := make(chan struct{})
	srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		fn()
	})
	<-done
}

// frameBytes is fr as a client reads it, past its length header.
func frameBytes(fr wire.Frame) []byte { return fr.Bytes()[2:] }

// mountOrder is a mount no collar called, as the rider reads it: the
// disarm's ActionFailed and UserInfo, the SkillList, the mounting Ride and
// the speed refresh; the fixture holds no pet data for the mount, so no
// feed gauge follows.
var mountOrder = []byte{
	serverpackets.OpcodeActionFailed, serverpackets.OpcodeUserInfo,
	serverpackets.OpcodeSkillList, serverpackets.OpcodeRide, serverpackets.OpcodeUserInfo,
}

// dismountOrder is a dismount as the rider reads it: the gauge emptied, the
// SkillList, the dismounting Ride and the speed refresh.
var dismountOrder = []byte{
	serverpackets.OpcodeSetupGauge, serverpackets.OpcodeSkillList, serverpackets.OpcodeRide, serverpackets.OpcodeUserInfo,
}

// assertMounted requires frames to be want, the Ride at rideAt mounting
// npcID, and gmID riding npcID with no collar.
func assertMounted(t *testing.T, srv *gameservertest.Server, frames [][]byte, want []byte, rideAt int, gmID, npcID int32) {
	t.Helper()
	if got := testsupport.FrameOpcodes(frames); !bytes.Equal(got, want) {
		t.Fatalf("frames = %x, want %x", got, want)
	}
	if !bytes.Equal(frames[rideAt], frameBytes(serverpackets.FrameRide(gmID, npcID))) {
		t.Fatalf("Ride = %x, want mounting %d", frames[rideAt], npcID)
	}
	c := liveCharacter(t, srv, gmID)
	if c.MountNPCID() != npcID || c.MountObjectID() != 0 || c.MountType() != player.MountTypeOf(npcID) {
		t.Fatalf("mount = %d collar %d type %d, want %d with no collar", c.MountNPCID(), c.MountObjectID(), c.MountType(), npcID)
	}
}

// TestAdminRideRefusals pins //ride's answers to a missing and an unknown
// mount: each named, and no mount.
func TestAdminRideRefusals(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("ride")), "You must enter a parameter for that command.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ride horse")), "Parameter 'horse' isn't recognized for that command.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("ride 3")), "Parameter '3' isn't recognized for that command.")
	if c := liveCharacter(t, srv, gmID); c.Mounted() {
		t.Fatalf("mounted on %d after refusals", c.MountNPCID())
	}
}

// TestAdminRideCursedWeaponHolder pins //ride refused to a cursed weapon
// holder, whatever the mount asked.
func TestAdminRideCursedWeaponHolder(t *testing.T) {
	t.Parallel()
	srv, gmID := bootCursedAdmin(t, 0)
	gm := srv.Client
	enterWorld(t, gm)
	exchange(t, gm, encodeBuildCmd("cw set 8190"))
	srv.Settle(t)
	drain(t, gm)
	if !liveCharacter(t, srv, gmID).CursedWeaponEquipped() {
		t.Fatal("game master holds no cursed weapon")
	}

	for _, cmd := range []string{"ride 1", "ride wyvern", "ride"} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd(cmd)), "You can't use //ride owning a Cursed Weapon.")
	}
	if c := liveCharacter(t, srv, gmID); c.Mounted() {
		t.Fatalf("cursed weapon holder mounted on %d", c.MountNPCID())
	}
}

// TestAdminRideAndUnride pins //ride strider, //ride wyvern from the
// strider (the strider left first), and //unride, which shows the dismount
// even off a mount; a //unride followed by anything does nothing.
func TestAdminRideAndUnride(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)

	assertMounted(t, srv, exchange(t, gm, encodeBuildCmd("ride strider")), mountOrder, 3, gmID, summonStriderID)

	frames := exchange(t, gm, encodeBuildCmd("ride 2"))
	assertMounted(t, srv, frames, slices.Concat(dismountOrder, mountOrder), len(dismountOrder)+3, gmID, player.WyvernNPCID)
	if !bytes.Equal(frames[2], frameBytes(serverpackets.FrameDismount(gmID))) {
		t.Fatalf("first Ride = %x, want the dismount", frames[2])
	}

	if frames := exchange(t, gm, encodeBuildCmd("unride extra")); len(frames) != 0 {
		t.Fatalf("//unride extra frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	if !liveCharacter(t, srv, gmID).Mounted() {
		t.Fatal("//unride extra dismounted")
	}

	for range 2 {
		frames := exchange(t, gm, encodeBuildCmd("unride"))
		if got := testsupport.FrameOpcodes(frames); !bytes.Equal(got, dismountOrder) {
			t.Fatalf("//unride frames = %x, want %x", got, dismountOrder)
		}
		if !bytes.Equal(frames[2], frameBytes(serverpackets.FrameDismount(gmID))) {
			t.Fatalf("Ride = %x, want the dismount", frames[2])
		}
		if c := liveCharacter(t, srv, gmID); c.Mounted() {
			t.Fatalf("still mounted on %d", c.MountNPCID())
		}
	}
}

// TestAdminRideSendsSummonAway pins //ride with a pet out: the pet leaves
// before the mount.
func TestAdminRideSendsSummonAway(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, summonAdminOptions(t)...)
	collar := srv.GiveItem(t, gmID, infoCollarID, 1)
	gm := srv.Client
	enterWorld(t, gm)
	pet := callPet(t, srv, gm, gmID, collar)

	frames := exchange(t, gm, encodeBuildCmd("ride 1"))
	gone := slices.Index(testsupport.FrameOpcodes(frames), serverpackets.OpcodePetDelete)
	ride := slices.Index(testsupport.FrameOpcodes(frames), serverpackets.OpcodeRide)
	if gone < 0 || ride < 0 || gone > ride {
		t.Fatalf("frames = %x, want PetDelete before the Ride", testsupport.FrameOpcodes(frames))
	}
	if _, ok := srv.State.Summon(gmID); ok {
		t.Fatal("pet still out after //ride")
	}
	if c := liveCharacter(t, srv, gmID); c.MountNPCID() != summonStriderID {
		t.Fatalf("mount = %d, want the strider", c.MountNPCID())
	}
	if pet.Dead() {
		t.Fatal("pet died instead of leaving")
	}
}

// TestAdminUnsummon pins //unsummon: without a summon the target is
// invalid; it sends away gm's own pet with nothing selected, a selected
// pet, and the pet of a selected player.
func TestAdminUnsummon(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, summonAdminOptions(t)...)
	collar := srv.GiveItem(t, gmID, infoCollarID, 1)
	gm := srv.Client
	enterWorld(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("unsummon"))
	if len(frames) != 1 {
		t.Fatalf("frames = %x, want INVALID_TARGET alone", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)

	callPet(t, srv, gm, gmID, collar)
	frames = exchange(t, gm, encodeBuildCmd("unsummon"))
	if !slices.Contains(testsupport.FrameOpcodes(frames), serverpackets.OpcodePetDelete) {
		t.Fatalf("frames = %x, want the pet deleted", testsupport.FrameOpcodes(frames))
	}
	if _, ok := srv.State.Summon(gmID); ok {
		t.Fatal("own pet still out")
	}

	// A selected pet is its owner's to send away.
	first, firstID, firstCollar := addSummoner(t, srv, "summoner1", "SummonerOne")
	pet := callPet(t, srv, first, firstID, firstCollar)
	retarget(t, srv, gm, gmID, pet)
	exchange(t, gm, encodeBuildCmd("unsummon"))
	srv.AdvanceUntil(t, "selected pet gone", func() bool {
		_, ok := srv.State.Summon(firstID)
		return !ok
	})

	second, secondID, secondCollar := addSummoner(t, srv, "summoner2", "SummonerTwo")
	callPet(t, srv, second, secondID, secondCollar)
	retarget(t, srv, gm, gmID, onlinePlayer(t, srv, secondID))
	exchange(t, gm, encodeBuildCmd("unsummon"))
	srv.AdvanceUntil(t, "selected player's pet gone", func() bool {
		_, ok := srv.State.Summon(secondID)
		return !ok
	})
}

// TestAdminSummonRefusals pins //summon's refusals: no pet makes the target
// invalid; a missing, unknown or unreadable subcommand gets the usage; a
// level the growth table has no row for is invalid.
func TestAdminSummonRefusals(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, summonAdminOptions(t)...)
	collar := srv.GiveItem(t, gmID, infoCollarID, 1)
	gm := srv.Client
	enterWorld(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("summon food"))
	if len(frames) != 1 {
		t.Fatalf("frames = %x, want INVALID_TARGET alone", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)

	pet := callPet(t, srv, gm, gmID, collar)
	const usage = "Usage: //summon food|inventory|level>"
	for _, cmd := range []string{"summon", "summon feed", "summon level", "summon level ten", "summon level 2147483648"} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd(cmd)), usage)
	}
	for _, cmd := range []string{"summon level 0", "summon level 82", "summon level -1"} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd(cmd)), "Invalid level for //summon level.")
	}
	if pet.Level() != 10 || pet.Exp() != 900 {
		t.Fatalf("pet level %d exp %d, want 10 and 900 untouched", pet.Level(), pet.Exp())
	}
}

// TestAdminSummonFood pins //summon food on a selected player's pet: its
// meal filled to its level's most, silently.
func TestAdminSummonFood(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, summonAdminOptions(t)...)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID, userCollar := addSummoner(t, srv, "feeder", "Feeder")
	pet := callPet(t, srv, user, userID, userCollar)
	onQueue(t, srv, userID, func() { pet.AddFed(-2000) })
	if pet.Fed() > 1000 {
		t.Fatalf("fed = %d, want it lowered", pet.Fed())
	}
	retarget(t, srv, gm, gmID, onlinePlayer(t, srv, userID))

	if frames := exchange(t, gm, encodeBuildCmd("summon food")); len(frames) != 0 {
		t.Fatalf("frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	srv.AdvanceUntil(t, "pet fed", func() bool { return pet.Fed() == 3000 })
}

// TestAdminSummonInventory pins //summon inventory: gm sees the pet's
// inventory, an unnamed pet showing no name.
func TestAdminSummonInventory(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, summonAdminOptions(t)...)
	collar := srv.GiveItem(t, gmID, infoCollarID, 1)
	gm := srv.Client
	enterWorld(t, gm)
	pet := callPet(t, srv, gm, gmID, collar)
	objID := srv.NewObjectID()
	onQueue(t, srv, gmID, func() {
		if pet.PetInventory().AddNew(item.AdenaID, 77, objID) == nil {
			t.Error("adena not added to the pet")
		}
	})
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("summon inventory"))
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeGMViewItemList {
		t.Fatalf("frames = %x, want GMViewItemList alone", testsupport.FrameOpcodes(frames))
	}
	r := wire.NewReader(frames[0][1:])
	if name := r.ReadString(); name != "" {
		t.Fatalf("name = %q, want none", name)
	}
	if limit := r.ReadInt32(); int(limit) != pet.InventoryLimit() {
		t.Fatalf("limit = %d, want %d", limit, pet.InventoryLimit())
	}
	if show, n := r.ReadUint16(), r.ReadUint16(); show != 1 || n != 1 {
		t.Fatalf("show %d count %d, want 1 and 1", show, n)
	}
	r.ReadUint16() // category
	if gotObj, gotID, count := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); gotObj != objID || gotID != item.AdenaID || count != 77 {
		t.Fatalf("item = %d/%d x%d, want %d/%d x77", gotObj, gotID, count, objID, item.AdenaID)
	}
	r.ReadUint16() // subcategory
	r.ReadUint16() // custom type 1
	if equipped := r.ReadUint16(); equipped != 0 {
		t.Fatalf("equipped = %d, want 0", equipped)
	}
	r.ReadInt32()  // body slot
	r.ReadUint16() // enchant
	r.ReadUint16() // custom type 2
	if aug, mana := r.ReadInt32(), r.ReadInt32(); aug != 0 || mana != -1 {
		t.Fatalf("augmentation %d mana %d, want 0 and -1", aug, mana)
	}
	if r.Remaining() != 0 {
		t.Fatalf("%d trailing bytes", r.Remaining())
	}
}

// TestAdminSummonLevel pins //summon level: up, the pet takes the level
// with full vitals, its collar's enchant follows and it plays the level-up
// animation; down, the level and collar follow with no animation. Neither
// tells the owner of experience earned. Level 81 moves the experience but
// not the level.
func TestAdminSummonLevel(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, summonAdminOptions(t)...)
	collar := srv.GiveItem(t, gmID, infoCollarID, 1)
	gm := srv.Client
	enterWorld(t, gm)
	pet := callPet(t, srv, gm, gmID, collar)

	collarEnchant := func() int {
		t.Helper()
		var level int
		onQueue(t, srv, gmID, func() {
			inst := liveCharacter(t, srv, gmID).Inventory().ItemByObjectID(collar)
			level = inst.Snapshot().EnchantLevel
		})
		return level
	}
	levelUps := func(frames [][]byte) int {
		n := 0
		for _, f := range frames {
			if f[0] == serverpackets.OpcodeSocialAction {
				r := wire.NewReader(f[1:])
				if r.ReadInt32() == pet.ObjectID() && r.ReadInt32() == summonPetLevelUpAction {
					n++
				}
			}
			if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == serverpackets.SystemMessagePetEarnedS1Exp {
				t.Fatalf("frames = %x carry the exp-earned message", testsupport.FrameOpcodes(frames))
			}
		}
		return n
	}

	frames := exchange(t, gm, encodeBuildCmd("summon level 12"))
	srv.Settle(t)
	frames = append(frames, gm.ReadQueued()...)
	if pet.Level() != 12 || pet.Exp() != 1100 {
		t.Fatalf("pet level %d exp %d, want 12 and 1100", pet.Level(), pet.Exp())
	}
	if pet.HP() != pet.MaxHPValue() || pet.MaxHPValue() < 510 {
		t.Fatalf("pet HP %v/%v, want full at the level 12 row", pet.HP(), pet.MaxHPValue())
	}
	if got := levelUps(frames); got != 1 {
		t.Fatalf("level-up animations = %d in %x, want 1", got, testsupport.FrameOpcodes(frames))
	}
	if got := collarEnchant(); got != 12 {
		t.Fatalf("collar enchant = %d, want 12", got)
	}

	frames = exchange(t, gm, encodeBuildCmd("summon level 11"))
	srv.Settle(t)
	frames = append(frames, gm.ReadQueued()...)
	if pet.Level() != 11 || pet.Exp() != 1000 {
		t.Fatalf("pet level %d exp %d, want 11 and 1000", pet.Level(), pet.Exp())
	}
	if got := levelUps(frames); got != 0 {
		t.Fatalf("level-up animations = %d, want none going down", got)
	}
	if got := collarEnchant(); got != 11 {
		t.Fatalf("collar enchant = %d, want 11", got)
	}

	// Level 81 is past the highest a pet holds: the experience moves, the
	// level stays and nothing is shown.
	if frames := exchange(t, gm, encodeBuildCmd("summon level 81")); len(frames) != 0 {
		t.Fatalf("level 81 frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	srv.Settle(t)
	if pet.Level() != 11 || pet.Exp() != 8000 {
		t.Fatalf("pet level %d exp %d, want 11 and 8000", pet.Level(), pet.Exp())
	}

	// Back to level 11's start: the level is unchanged, so only the
	// experience moves, silently.
	if frames := exchange(t, gm, encodeBuildCmd("summon level 11")); len(frames) != 0 {
		t.Fatalf("same level frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	srv.Settle(t)
	if pet.Level() != 11 || pet.Exp() != 1000 {
		t.Fatalf("pet level %d exp %d, want 11 and 1000", pet.Level(), pet.Exp())
	}
}
