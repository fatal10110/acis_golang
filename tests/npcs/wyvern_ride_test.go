package npcs

import (
	"bytes"
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: WyvernManagerNpc.onBypassFeedback RideWyvern
// (WyvernManagerNpc.java:34-71) and ClanHallDoorman.onBypassFeedback
// wyvern_ride (ClanHallDoorman.java:57-97). Past each NPC's own gates (the
// manager: the leader of the clan owning its castle, else page 2; the
// doorman: an Aden hall, the owning clan, its leader, else nothing), the
// ride answers page 3 while Dusk owns the Seal of Strife; on no strider
// YOU_MAY_ONLY_RIDE_WYVERN_WHILE_RIDING_STRIDER then page 1 (the doorman:
// page 8 alone while the mount level is short of RequiredStriderLevel);
// page 6 for a strider of too low a level; page 5 after NOT_ENOUGH_ITEMS
// for a talker short of RequiredCrystalsNumber B-grade crystals; else the
// crystals go (S2_S1_DISAPPEARED), Player.dismount() takes the talker off
// its strider, Player.mount(12621, 0) puts it on a wyvern no collar
// called, and page 4 opens. Every page is sendHtm's: the page, then
// ActionFailed, before the dispatcher's own ActionFailed.

const (
	windStriderID         = 12526
	msgOnlyWyvernStrider  = serverpackets.SystemMessageOnlyRideWyvernFromStrider
	msgNotEnoughItems     = serverpackets.SystemMessageNotEnoughItems
	msgS2S1Disappeared    = serverpackets.SystemMessageS2S1Disappeared
	rideCrystals          = 10
	rideStriderLevel      = 55
	wyvernRiderCollarItem = 77
)

// wyvernRideCatalog is the shared catalog plus the B-grade crystal a ride
// is paid in.
func wyvernRideCatalog() gameservertest.Option {
	templates := append([]*item.Template(nil), gameservertest.ItemTemplates().All()...)
	templates = append(templates, &item.Template{
		ID: item.CrystalB.ItemID(), Name: "Crystal: B-Grade", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Depositable: true,
		EtcItem: &item.EtcItemDetail{},
	})
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// wyvernRidePages are the ride pages of dir, each naming itself and the
// placeholders a wyvern page fills.
func wyvernRidePages(dir string, vals ...string) gameservertest.Option {
	pages := map[string]string{}
	for _, val := range vals {
		file := dir + "/wyvernmanager-" + val + ".htm"
		pages[file] = commandPage(file)
	}
	return gameservertest.WithHTMLPages(pages)
}

// duskStrife seeds Dusk as the owner of the Seal of Strife.
func duskStrife(t *testing.T) gameservertest.Option {
	return gameservertest.WithSevenSignsSeed(func(store *gamesql.SevenSignsStore) {
		ctx := context.Background()
		row, found, err := store.LoadStatus(ctx)
		if err != nil || !found {
			t.Fatalf("load seven signs status: found=%v err=%v", found, err)
		}
		row.SealOwners[2] = sevensigns.Dusk
		if err := store.SaveStatus(ctx, row); err != nil {
			t.Fatalf("seed seven signs status: %v", err)
		}
	})
}

// rider is the talker's live character.
func (w *folkWorld) rider(t *testing.T) *player.Character {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("talker missing from the world")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("talker is %T", obj)
	}
	return c
}

// onRiderQueue runs step on the talker's queue and drains what it sent.
func (w *folkWorld) onRiderQueue(t *testing.T, step func(c *player.Character)) {
	t.Helper()
	c := w.rider(t)
	done := make(chan struct{})
	w.srv.PlayerQueue(t, w.player).Post(func() {
		defer close(done)
		step(c)
	})
	<-done
	drainUntilQuiet(t, w.c)
}

// holdCrystals hands the talker n B-grade crystals.
func (w *folkWorld) holdCrystals(t *testing.T, n int) {
	t.Helper()
	id := w.srv.NewObjectID()
	w.onRiderQueue(t, func(c *player.Character) {
		if c.Inventory().AddNew(item.CrystalB.ItemID(), n, id) == nil {
			t.Error("crystals not added")
		}
	})
}

// mountPet seats the talker on the pet npcID of level level, called by a
// collar.
func (w *folkWorld) mountPet(t *testing.T, npcID int32, level int) {
	t.Helper()
	w.onRiderQueue(t, func(c *player.Character) {
		if !c.MountPet(npcID, wyvernRiderCollarItem, level) {
			t.Error("the mount did not change")
		}
	})
}

// frameBody is fr as a client reads it, past its length header.
func frameBody(fr wire.Frame) []byte { return fr.Bytes()[2:] }

// wyvernRideMessage is the answer: msg, then page, then ActionFailed twice.
var wyvernRideMessage = []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}

// assertWyvernMessage checks frames are msg, then page of f, then
// ActionFailed twice.
func assertWyvernMessage(t *testing.T, frames [][]byte, msg int32, f *npc.Folk, page string) {
	t.Helper()
	assertAnswer(t, frames, wyvernRideMessage, f, page)
	if got := systemMessageID(frames[0]); got != msg {
		t.Fatalf("system message = %d, want %d", got, msg)
	}
}

// assertStillOn checks the talker still rides npcID at level, holding
// crystals.
func (w *folkWorld) assertStillOn(t *testing.T, npcID int32, level, crystals int) {
	t.Helper()
	c := w.rider(t)
	if c.MountNPCID() != npcID || c.MountLevel() != level {
		t.Fatalf("mount = %d level %d, want %d level %d", c.MountNPCID(), c.MountLevel(), npcID, level)
	}
	if got := w.srv.PlayerItemCount(t, w.player, item.CrystalB.ItemID()); got != crystals {
		t.Fatalf("crystals = %d, want %d", got, crystals)
	}
}

// wyvernRideOrder is the ride as the rider reads it: the crystals named;
// the strider left (gauge cleared, SkillList, the dismounting Ride,
// UserInfo); the wyvern mounted (the disarm's ActionFailed and UserInfo,
// SkillList, the mounting Ride, UserInfo; the fixture holds no wyvern pet
// data, so no feed gauge follows); then page 4 and ActionFailed twice.
var wyvernRideOrder = []byte{
	serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodeSetupGauge, serverpackets.OpcodeSkillList, serverpackets.OpcodeRide, serverpackets.OpcodeUserInfo,
	serverpackets.OpcodeActionFailed, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSkillList, serverpackets.OpcodeRide, serverpackets.OpcodeUserInfo,
	serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed,
}

// assertWyvernRide checks frames are the ride in wyvernRideOrder, naming
// the crystals taken and page, and that the talker flies a wyvern no
// collar called at its own level, holding left crystals.
func (w *folkWorld) assertWyvernRide(t *testing.T, frames [][]byte, f *npc.Folk, page string, left int) {
	t.Helper()
	assertAnswer(t, frames, wyvernRideOrder, f, page)
	if got := systemMessageID(frames[0]); got != msgS2S1Disappeared {
		t.Fatalf("system message = %d, want S2_S1_DISAPPEARED", got)
	}
	if !bytes.Equal(frames[1], frameBody(serverpackets.FrameSetupGauge(serverpackets.GaugeGreen, 0, 0))) {
		t.Fatalf("SetupGauge = %x, want the green gauge cleared", frames[1])
	}
	if !bytes.Equal(frames[3], frameBody(serverpackets.FrameDismount(w.player))) {
		t.Fatalf("first Ride = %x, want the dismount", frames[3])
	}
	if !bytes.Equal(frames[8], frameBody(serverpackets.FrameRide(w.player, player.WyvernNPCID))) {
		t.Fatalf("second Ride = %x, want the wyvern mount", frames[8])
	}
	c := w.rider(t)
	if c.MountNPCID() != player.WyvernNPCID || c.MountObjectID() != 0 || c.MountLevel() != playerLevel || c.MountType() != player.MountTypeWyvern {
		t.Fatalf("mount = %d collar %d level %d type %d, want the wyvern, no collar, level %d",
			c.MountNPCID(), c.MountObjectID(), c.MountLevel(), c.MountType(), playerLevel)
	}
	if got := w.srv.PlayerItemCount(t, w.player, item.CrystalB.ItemID()); got != left {
		t.Fatalf("crystals = %d, want %d", got, left)
	}
}

// TestWyvernManagerRideWyvern walks RideWyvern at a castle wyvern manager
// through each refusal and the ride.
func TestWyvernManagerRideWyvern(t *testing.T) {
	t.Parallel()
	page := func(f *npc.Folk, val string) string {
		return filled("wyvernmanager/wyvernmanager-"+val+".htm", f, wyvernPairs...)
	}
	boot := func(t *testing.T, setup residenceSetup, extra ...gameservertest.Option) (*folkWorld, *npc.Folk) {
		t.Helper()
		extra = append(extra, wyvernRideCatalog(), wyvernRidePages("wyvernmanager", "4", "5", "6"))
		w := bootResidenceCommands(t, commandSetup{residenceSetup: setup, extra: extra})
		return w, w.spawnKeeper(t, "WyvernManagerNpc", wyvernManagerID, 30)
	}
	for _, c := range []struct {
		name  string
		setup residenceSetup
	}{{"member", residenceSetup{rank: 5}}, {"rival leader", residenceSetup{rival: true}}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, f := boot(t, c.setup)
			w.holdCrystals(t, rideCrystals)
			w.mountPet(t, windStriderID, rideStriderLevel)
			assertAnswer(t, w.command(t, f, "RideWyvern"), chatThenFinal, f, page(f, "2"))
			w.assertStillOn(t, windStriderID, rideStriderLevel, rideCrystals)
		})
	}
	t.Run("dusk owns strife", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, residenceSetup{strife: sevensigns.Dusk})
		w.holdCrystals(t, rideCrystals)
		w.mountPet(t, windStriderID, rideStriderLevel)
		assertAnswer(t, w.command(t, f, "RideWyvern"), chatThenFinal, f, page(f, "3"))
		w.assertStillOn(t, windStriderID, rideStriderLevel, rideCrystals)
	})
	t.Run("on foot", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, residenceSetup{})
		w.holdCrystals(t, rideCrystals)
		assertWyvernMessage(t, w.command(t, f, "RideWyvern"), msgOnlyWyvernStrider, f, page(f, "1"))
		w.assertStillOn(t, 0, 0, rideCrystals)
	})
	t.Run("strider too young", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, residenceSetup{})
		w.holdCrystals(t, rideCrystals)
		w.mountPet(t, windStriderID, rideStriderLevel-1)
		assertAnswer(t, w.command(t, f, "RideWyvern"), chatThenFinal, f, page(f, "6"))
		w.assertStillOn(t, windStriderID, rideStriderLevel-1, rideCrystals)
	})
	t.Run("short of crystals", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, residenceSetup{})
		w.holdCrystals(t, rideCrystals-1)
		w.mountPet(t, windStriderID, rideStriderLevel)
		assertWyvernMessage(t, w.command(t, f, "RideWyvern"), msgNotEnoughItems, f, page(f, "5"))
		w.assertStillOn(t, windStriderID, rideStriderLevel, rideCrystals-1)
	})
	t.Run("ride", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, residenceSetup{})
		w.holdCrystals(t, rideCrystals+2)
		w.mountPet(t, windStriderID, rideStriderLevel)
		w.assertWyvernRide(t, w.command(t, f, "RideWyvern"), f, page(f, "4"), 2)
	})
}

// TestClanHallDoormanWyvernRide walks wyvern_ride at an Aden clan hall
// doorman through each refusal and the ride; a bystander sees the rider
// leave its strider and take wing.
func TestClanHallDoormanWyvernRide(t *testing.T) {
	t.Parallel()
	page := func(f *npc.Folk, val string) string {
		return filled("clanHallDoormen/wyvernmanager-"+val+".htm", f, wyvernPairs...)
	}
	boot := func(t *testing.T, setup hallManagerSetup, extra ...gameservertest.Option) (*hallManagerWorld, *npc.Folk) {
		t.Helper()
		setup.hall, setup.manager = goldenChamber, goldenManagerID
		setup.extra = append(extra, hallDoormanCommandPages(), wyvernRideCatalog(), wyvernRidePages("clanHallDoormen", "3", "4", "5", "6", "8"))
		w := bootHallManager(t, setup)
		return w, w.spawnKeeper(t, "ClanHallDoorman", goldenDoormanID, 40)
	}
	t.Run("member", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, hallManagerSetup{rank: 5})
		w.holdCrystals(t, rideCrystals)
		w.mountPet(t, windStriderID, rideStriderLevel)
		assertAnswer(t, w.command(t, f, "wyvern_ride"), releaseOnly, f, "")
		w.assertStillOn(t, windStriderID, rideStriderLevel, rideCrystals)
	})
	t.Run("dusk owns strife", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, hallManagerSetup{}, duskStrife(t))
		w.holdCrystals(t, rideCrystals)
		w.mountPet(t, windStriderID, rideStriderLevel)
		assertAnswer(t, w.command(t, f, "wyvern_ride"), chatThenFinal, f, page(f, "3"))
		w.assertStillOn(t, windStriderID, rideStriderLevel, rideCrystals)
	})
	t.Run("on foot", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, hallManagerSetup{})
		w.holdCrystals(t, rideCrystals)
		assertAnswer(t, w.command(t, f, "wyvern_ride"), chatThenFinal, f, page(f, "8"))
		w.assertStillOn(t, 0, 0, rideCrystals)
	})
	t.Run("on a grown wyvern", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, hallManagerSetup{})
		w.holdCrystals(t, rideCrystals)
		w.mountPet(t, player.WyvernNPCID, rideStriderLevel)
		assertWyvernMessage(t, w.command(t, f, "wyvern_ride"), msgOnlyWyvernStrider, f, page(f, "1"))
		w.assertStillOn(t, player.WyvernNPCID, rideStriderLevel, rideCrystals)
	})
	t.Run("strider too young", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, hallManagerSetup{})
		w.holdCrystals(t, rideCrystals)
		w.mountPet(t, windStriderID, rideStriderLevel-1)
		assertAnswer(t, w.command(t, f, "wyvern_ride"), chatThenFinal, f, page(f, "6"))
		w.assertStillOn(t, windStriderID, rideStriderLevel-1, rideCrystals)
	})
	t.Run("short of crystals", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, hallManagerSetup{})
		w.mountPet(t, windStriderID, rideStriderLevel)
		assertWyvernMessage(t, w.command(t, f, "wyvern_ride"), msgNotEnoughItems, f, page(f, "5"))
		w.assertStillOn(t, windStriderID, rideStriderLevel, 0)
	})
	t.Run("ride", func(t *testing.T) {
		t.Parallel()
		w, f := boot(t, hallManagerSetup{})
		w.holdCrystals(t, rideCrystals)
		w.mountPet(t, windStriderID, rideStriderLevel)
		drainUntilQuiet(t, w.bystander)
		w.assertWyvernRide(t, w.command(t, f, "wyvern_ride"), f, page(f, "4"), 0)
		seen := drainFrames(t, w.bystander)
		var rides [][]byte
		for _, fr := range seen {
			if fr[0] == serverpackets.OpcodeRide {
				rides = append(rides, fr)
			}
		}
		want := [][]byte{frameBody(serverpackets.FrameDismount(w.player)), frameBody(serverpackets.FrameRide(w.player, player.WyvernNPCID))}
		if !slices.EqualFunc(rides, want, bytes.Equal) {
			t.Fatalf("bystander Rides = %x, want the dismount then the wyvern", rides)
		}
	})
}
