package pets

import (
	"context"
	"database/sql"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: a pet's pickup runs the ground item's pickupMe for the pet,
// which tells the tutorial quest of the pet's owner (the acting player) of
// adena (CE57) and blue gemstones (CE6353), after the GetItem broadcast and
// before the item leaves the world. A partied owner's pet picks the item up
// the same way before the party shares it.

const blueGemstoneID = int32(6353)

// petTutorialLog records the events the owner's tutorial quest hears.
type petTutorialLog struct {
	mu     sync.Mutex
	events []string
}

// take returns the events heard since the last call.
func (l *petTutorialLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.events
	l.events = nil
	return out
}

// bootTutorialWolf boots the owner with a started tutorial quest, a wolf
// collar and the blue gemstone in the item catalog, then calls the wolf
// out. The tutorial quest records each event it hears and answers CE<n>
// by enabling client event n, so its answer shows in the frame order.
func bootTutorialWolf(t *testing.T) (*petWorld, *summon.Actor, *petTutorialLog) {
	t.Helper()
	heard := &petTutorialLog{}
	hook := func(_ *script.Script, e script.Event) string {
		heard.mu.Lock()
		heard.events = append(heard.events, e.Name)
		heard.mu.Unlock()
		if id, ok := strings.CutPrefix(e.Name, "CE"); ok {
			n, _ := strconv.Atoi(id)
			e.Player.EnableTutorialEvent(int32(n))
		}
		return ""
	}
	path := "script.feature.Tutorial"
	catalog := item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: blueGemstoneID, Name: "Blue Gemstone", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Dropable: true, Tradable: true, Destroyable: true,
	}))
	srv := bootPets(t,
		gameservertest.WithItemTemplates(catalog),
		gameservertest.WithScripts([]script.Listing{{Path: path}}, script.Catalog{
			path: func() script.Script { return script.Script{QuestID: -1, Hooks: script.Hooks{OnEvent: hook}} },
		}),
	)
	ownerID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(),
		"INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?)",
		ownerID, "Tutorial", questlog.KeyState, sql.NullString{String: "STARTED", Valid: true}); err != nil {
		t.Fatalf("seed the tutorial state: %v", err)
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	heard.take()
	return h, wolf, heard
}

// requireTutorialBetweenPickup checks the owner's frames carry the pet's
// GetItem for groundID, then the tutorial's answer to CE<id>, then the
// ground item's DeleteObject.
func requireTutorialBetweenPickup(t *testing.T, frames [][]byte, wolf *summon.Actor, groundID, id int32) {
	t.Helper()
	get := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeGetItem })
	if get < 0 || get+2 >= len(frames) {
		t.Fatalf("pickup frames %x: no GetItem followed by two frames", frameOpcodes(frames))
	}
	r := wire.NewReader(frames[get][1:])
	if picker, ground := r.ReadInt32(), r.ReadInt32(); picker != wolf.ObjectID() || ground != groundID {
		t.Fatalf("GetItem = picker %d item %d, want pet %d item %d", picker, ground, wolf.ObjectID(), groundID)
	}
	assertFrameOpcode(t, frames[get+1], serverpackets.OpcodeTutorialEnableClientEvent, "tutorial answer")
	if got := wire.NewReader(frames[get+1][1:]).ReadInt32(); got != id {
		t.Fatalf("tutorial answer enables event %d, want %d", got, id)
	}
	assertFrameOpcode(t, frames[get+2], serverpackets.OpcodeDeleteObject, "DeleteObject")
	if got := wire.NewReader(frames[get+2][1:]).ReadInt32(); got != groundID {
		t.Fatalf("DeleteObject id = %d, want ground item %d", got, groundID)
	}
}

// TestPetPickupTellsTheOwnersTutorial: the wolf picks up adena, then a blue
// gemstone; the owner's tutorial hears CE57, then CE6353, each between the
// pet's GetItem and the item's DeleteObject. A potion raises nothing.
func TestPetPickupTellsTheOwnersTutorial(t *testing.T) {
	t.Parallel()
	h, wolf, heard := bootTutorialWolf(t)

	for _, id := range []int32{item.AdenaID, blueGemstoneID} {
		ground := h.seedGroundNearOwner(t, id, 10)
		requireTutorialBetweenPickup(t, h.petPickup(t, ground), wolf, ground, id)
		if got, want := heard.take(), []string{"CE" + strconv.Itoa(int(id))}; !slices.Equal(got, want) {
			t.Fatalf("pickup of %d: heard %q, want %q", id, got, want)
		}
	}

	ground := h.seedGroundNearOwner(t, partyPotionID, 1)
	if frames := h.petPickup(t, ground); !slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeGetItem }) {
		t.Fatalf("potion pickup frames %x: no GetItem", frameOpcodes(frames))
	}
	if got := heard.take(); len(got) != 0 {
		t.Fatalf("potion pickup: heard %q, want nothing", got)
	}
}

// TestPartyPetPickupTellsTheOwnersTutorial: a partied owner's wolf picks up
// adena the party shares; the owner's tutorial still hears CE57 once,
// between the pet's GetItem and the item's DeleteObject.
func TestPartyPetPickupTellsTheOwnersTutorial(t *testing.T) {
	t.Parallel()
	h, wolf, heard := bootTutorialWolf(t)
	formPetParty(t, h, party.LootFindersKeepers)
	heard.take()

	ground := h.seedGroundNearOwner(t, item.AdenaID, 100)
	requireTutorialBetweenPickup(t, h.petPickup(t, ground), wolf, ground, item.AdenaID)
	if got := heard.take(); !slices.Equal(got, []string{"CE57"}) {
		t.Fatalf("party pickup: heard %q, want [CE57]", got)
	}
	if len(h.srv.GroundItems.Snapshots(nil)) != 0 {
		t.Fatal("the shared adena is still on the ground")
	}
}
