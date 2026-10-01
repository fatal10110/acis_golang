package pets

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// User command ids the client sends for /mount and /dismount.
const (
	cmdMount    = 61
	cmdDismount = 62
)

func encodeUserCommand(id int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestUserCommand)
	w.WriteInt32(id)
	return w.Bytes()
}

// mountCommand has the rider run user command id and returns its answer.
func (h *petWorld) mountCommand(t *testing.T, id int32) [][]byte {
	t.Helper()
	h.client.Send(encodeUserCommand(id))
	return drainFrames(t, h.client)
}

// requireDismounted fails unless frames show the rider getting off its
// mount (the emptied green gauge first, then the dismount Ride) and the
// rider is no longer mounted.
func (h *petWorld) requireDismounted(t *testing.T, what string, frames [][]byte) {
	t.Helper()
	if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeSetupGauge {
		t.Fatalf("%s = %x, want the emptied feed gauge first", what, frameOpcodes(frames))
	}
	if g := feedGauges(t, frames[:1]); len(g) != 1 || g[0] != (feedGauge{}) {
		t.Fatalf("%s gauge = %v, want an empty green gauge", what, g)
	}
	i := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeRide })
	if i < 0 {
		t.Fatalf("%s = %x, want a Ride", what, frameOpcodes(frames))
	}
	r := wire.NewReader(frames[i][1:])
	if id, action := r.ReadInt32(), r.ReadInt32(); id != h.ownerID || action != 0 {
		t.Fatalf("%s Ride = %d action %d, want %d dismounting", what, id, action, h.ownerID)
	}
	if h.rider(t).Mounted() {
		t.Fatalf("%s left the rider mounted", what)
	}
}

// TestDismountCommand pins /dismount (Dismount.useUserCommand): a rider
// gets off at once, with none of /mount's checks, here even off a hungry
// wyvern. Off a mount it does nothing.
func TestDismountCommand(t *testing.T) {
	t.Parallel()
	h, _ := bootWyvernRider(t)
	drainFrames(t, h.client)
	// 508 - 26*10 = 248 is below the hungry limit of 254.
	h.advanceTicks(t, 26)
	h.requireDismounted(t, "/dismount", h.mountCommand(t, cmdDismount))
	if frames := h.mountCommand(t, cmdDismount); len(frames) != 0 {
		t.Fatalf("/dismount off a mount = %x, want silence", frameOpcodes(frames))
	}
}

// TestMountCommandDismounts pins /mount on a mount (Player.mountPlayer's
// dismount branch): a fed wyvern rider over safe ground gets off.
func TestMountCommandDismounts(t *testing.T) {
	t.Parallel()
	h, _ := bootWyvernRider(t)
	drainFrames(t, h.client)
	h.requireDismounted(t, "/mount on a wyvern", h.mountCommand(t, cmdMount))
	if frames := h.mountCommand(t, cmdMount); len(frames) != 0 {
		t.Fatalf("/mount with no mount and no pet = %x, want silence", frameOpcodes(frames))
	}
}

// TestMountCommandHungryRefusal pins mountPlayer's hunger check: a rider
// whose mount is fed below its hungry limit (half of 508) is told a hungry
// mount cannot be dismounted (1008) and stays on.
func TestMountCommandHungryRefusal(t *testing.T) {
	t.Parallel()
	h, _ := bootWyvernRider(t)
	drainFrames(t, h.client)
	h.advanceTicks(t, 26)
	frames := h.mountCommand(t, cmdMount)
	if len(frames) != 1 {
		t.Fatalf("/mount on a hungry wyvern = %x, want one refusal", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageHungryStriderNotMount)
	if !h.rider(t).Mounted() {
		t.Fatal("hungry refusal dismounted the rider")
	}
}

// TestMountCommandNoLandingRefusal pins mountPlayer's wyvern zone check: in
// a no-landing zone the rider is told it cannot dismount here (1385) and
// stays on.
func TestMountCommandNoLandingRefusal(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(-1000000, 1000000, -1000000, 1000000, -100000, 100000)
	if err != nil {
		t.Fatalf("build zone form: %v", err)
	}
	zones := zone.NewIndex()
	zones.Add(zone.NewNoLanding(1, form))
	h, _ := bootWyvernRiderOpts(t, []gameservertest.Option{gameservertest.WithZones(zones)})
	drainFrames(t, h.client)
	frames := h.mountCommand(t, cmdMount)
	if len(frames) != 1 {
		t.Fatalf("/mount in a no-landing zone = %x, want one refusal", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNoDismountHere)
	if !h.rider(t).Mounted() {
		t.Fatal("no-landing refusal dismounted the rider")
	}
}

// dropGeo puts the ground drop units below whatever height is asked about.
type dropGeo struct {
	gameservertest.Geo
	drop int
}

func (g dropGeo) Height(_, _, z int) int16 { return int16(z - g.drop) }

// TestMountCommandElevationRefusal pins mountPlayer's wyvern fall check: a
// rider higher above the ground than the class may fall unhurt (250 for
// the fixture's male body) is told it cannot dismount from this height
// (1158) and stays on; 200 above the ground it gets off.
func TestMountCommandElevationRefusal(t *testing.T) {
	t.Parallel()
	t.Run("too high", func(t *testing.T) {
		t.Parallel()
		h, _ := bootWyvernRiderOpts(t, []gameservertest.Option{gameservertest.WithGeo(dropGeo{drop: 251})})
		drainFrames(t, h.client)
		frames := h.mountCommand(t, cmdMount)
		if len(frames) != 1 {
			t.Fatalf("/mount 251 above the ground = %x, want one refusal", frameOpcodes(frames))
		}
		assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageCannotDismountFromElevation)
		if !h.rider(t).Mounted() {
			t.Fatal("elevation refusal dismounted the rider")
		}
	})
	t.Run("safe", func(t *testing.T) {
		t.Parallel()
		h, _ := bootWyvernRiderOpts(t, []gameservertest.Option{gameservertest.WithGeo(dropGeo{drop: 250})})
		drainFrames(t, h.client)
		h.requireDismounted(t, "/mount 250 above the ground", h.mountCommand(t, cmdMount))
	})
}

// striderNPCID is a strider's npc id: one of the pets a player can ride.
const striderNPCID = 12526

// TestMountCommandStriderNotPortedYet pins /mount beside a summoned strider
// pet, before strider riding is ported (#3210): the client is released with
// ActionFailed and the rider stays on foot. A pet that cannot be ridden
// gets no answer.
func TestMountCommandStriderNotPortedYet(t *testing.T) {
	t.Parallel()
	t.Run("strider", func(t *testing.T) {
		t.Parallel()
		strider := wolfTemplate()
		strider.ID = striderNPCID
		items, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: wolfCollarID, NPCID: striderNPCID, SummonType: 1}})
		if err != nil {
			t.Fatalf("summon items: %v", err)
		}
		h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
			gameservertest.WithNPCs(npc.NewTable([]*npc.Template{strider, treeTemplate()})),
			gameservertest.WithSummonItems(items),
		})
		h.spawnWolf(t)
		frames := h.mountCommand(t, cmdMount)
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("/mount beside a strider = %x, want ActionFailed", frameOpcodes(frames))
		}
		if h.rider(t).Mounted() {
			t.Fatal("rider mounted a strider")
		}
	})
	t.Run("wolf", func(t *testing.T) {
		t.Parallel()
		h := bootOwnerWithCollar(t)
		h.spawnWolf(t)
		if frames := h.mountCommand(t, cmdMount); len(frames) != 0 {
			t.Fatalf("/mount beside a wolf = %x, want silence", frameOpcodes(frames))
		}
	})
}
