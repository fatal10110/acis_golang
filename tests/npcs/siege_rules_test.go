package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// gludioViewpoint is Gludio castle's viewpoint 620 (observerGroups.xml),
// 500 adena a seat.
var gludioViewpoint = location.Location{X: -18347, Y: 114000, Z: -2360}

const gludioViewpointFee = 500

// TestBypassTeleportToBesiegedCastleRefused pins a destination tied to a
// castle (Npc.teleport, Npc.java:1415-1423): while that castle's siege
// runs it is refused with CANNOT_PORT_VILLAGE_IN_SIEGE alone, released
// only by the dispatcher, nothing paid and nobody moved; before the siege
// the trip is paid and taken.
func TestBypassTeleportToBesiegedCastleRefused(t *testing.T) {
	t.Parallel()
	teleports := travel.TeleportTable{
		gatekeeperID: {
			{Location: paidSpot, Description: "Gludio Castle", Kind: travel.KindStandard, PriceID: int(item.AdenaID), PriceCount: tripPrice, CastleID: gludioCastleID},
		},
	}
	pages := dialogPages()
	pages["gatekeeper/30080.htm"] = gatekeeperPage
	for _, tt := range []struct {
		name    string
		started bool
	}{
		{name: "during the siege", started: true},
		{name: "before the siege"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := bootBesiegedWorld(t, tt.started, 2*tripPrice,
				gameservertest.WithHTMLPages(pages),
				gameservertest.WithItemTemplates(travelTemplates()),
				gameservertest.WithTeleports(teleports, nil, false, newTeleportClock().now),
				noBypassReuse,
			)
			w := b.folk()
			gk := w.spawnFolk(t, folkTemplate("Gatekeeper", gatekeeperID), 50)
			w.openAnyNpcPage(t)
			frames := w.tripFrames(t, npcCommand(gk, "teleport 0"))
			if !tt.started {
				assertFrames(t, "teleport 0", frames, trip(sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(tripPrice)))...)
				assertLandedNear(t, landing(t, frames), paidSpot)
				return
			}
			assertFrames(t, "teleport 0", frames, sysMsg(serverpackets.SystemMessageCannotPortVillageInSiege), []byte{serverpackets.OpcodeActionFailed})
			if got := w.held(t, item.AdenaID); got != 2*tripPrice {
				t.Fatalf("adena held = %d, want %d", got, 2*tripPrice)
			}
		})
	}
}

// TestObserveCastleViewpointDuringSiege pins a castle viewpoint while its
// castle's siege runs (Npc.java:1276-1282): it opens like any other, the
// fee taken and the player sent to watch from it.
func TestObserveCastleViewpointDuringSiege(t *testing.T) {
	t.Parallel()
	b := bootBesiegedWorld(t, true, 1000,
		gameservertest.WithObserverGroups(shippedObserverGroups(t)),
		gameservertest.WithItemTemplates(travelTemplates()),
		gameservertest.WithHTMLPages(dialogPages()),
		noBypassReuse,
	)
	w := b.folk()
	tower := w.spawnFolk(t, folkTemplate("Folk", broadcastingTowerID), 50)
	tower.SetObserverGroups([]int{612})
	w.talkTo(t, tower)
	w.observeFrames(t, npcCommand(tower, "observe_group 612"))
	frames := w.observeFrames(t, npcCommand(tower, "observe 620"))
	if got := frames[0]; string(got) != string(sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(gludioViewpointFee))) {
		t.Fatalf("observe 620 first frame = %x, want the fee taken", got)
	}
	start, ok := firstOpcode(frames, serverpackets.OpcodeObserverStart)
	if !ok {
		t.Fatalf("observe 620 = %x, want ObserverStart", opcodes(frames))
	}
	if want := observerStart(gludioViewpoint, 49152, 0); string(start) != string(want) {
		t.Fatalf("ObserverStart = %x, want %x", start, want)
	}
	if !w.character(t).ObserverMode() {
		t.Fatal("the player is not observing")
	}
	if got := w.held(t, item.AdenaID); got != 1000-gludioViewpointFee {
		t.Fatalf("adena held = %d, want %d", got, 1000-gludioViewpointFee)
	}
}
