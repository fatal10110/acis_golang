package network

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// decodeSystemMessage reads a SystemMessage frame as its id and params, a
// param written as "type:value".
func decodeSystemMessage(t *testing.T, frame []byte) (int32, []string) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("frame %x is not a SystemMessage", frame)
	}
	r := wire.NewReader(frame[1:])
	id := r.ReadInt32()
	var params []string
	for range r.ReadInt32() {
		typ := r.ReadInt32()
		if typ == serverpackets.SystemMessageParamText {
			params = append(params, fmt.Sprintf("%d:%s", typ, r.ReadString()))
		} else {
			params = append(params, fmt.Sprintf("%d:%d", typ, r.ReadInt32()))
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
	return id, params
}

// TestPartyLootNoticeFrame pins the line the other members get for each
// kind of loot: a counted stack (299, swept 608), an enchanted single item
// (376, its enchant level before the item), and a plain single item (300,
// swept 609). A swept item never names an enchant level.
func TestPartyLootNoticeFrame(t *testing.T) {
	const looter, itemID = "Looter", int32(30)
	name, itemParam := "0:Looter", "3:30"
	for _, tc := range []struct {
		what           string
		spoil          bool
		count, enchant int
		wantID         int32
		wantParams     []string
	}{
		{"stack", false, 5, 0, serverpackets.SystemMessageS1ObtainedS3S2, []string{name, itemParam, "6:5"}},
		{"swept stack", true, 5, 0, serverpackets.SystemMessageS1SweptUpS3S2, []string{name, itemParam, "6:5"}},
		{"enchanted", false, 1, 7, serverpackets.SystemMessageS1ObtainedS2S3, []string{name, "1:7", itemParam}},
		{"plain", false, 1, 0, serverpackets.SystemMessageS1ObtainedS2, []string{name, itemParam}},
		{"swept enchanted", true, 1, 7, serverpackets.SystemMessageS1SweptUpS2, []string{name, itemParam}},
	} {
		capture := &testsupport.FrameCapture{}
		capture.Send(partyLootNoticeFrame(looter, tc.spoil, itemID, tc.count, tc.enchant))
		id, params := decodeSystemMessage(t, capture.Frames()[0])
		if id != tc.wantID || !slices.Equal(params, tc.wantParams) {
			t.Errorf("%s: message %d %v, want %d %v", tc.what, id, params, tc.wantID, tc.wantParams)
		}
	}
}

// slotLimit is a fixed inventory slot limit with no weight limit.
type slotLimit int

func (l slotLimit) InventoryLimit() int { return int(l) }
func (slotLimit) WeightLimit() int      { return math.MaxInt32 }

// TestPartyLootEligible pins who may take an item under the random and
// by-turn rules: a member alive, with room for one more unit, and within
// party range of the origin. A dead member, a full one and a far one are
// passed over.
func TestPartyLootEligible(t *testing.T) {
	l := &GameClientLink{playerConfig: PlayerConfig{PartyRange: 1500}}
	capture := &testsupport.FrameCapture{}
	origin := newTestLivePlayer(t, 1, capture)
	alive := newTestLivePlayer(t, 2, capture)
	dead := newTestLivePlayer(t, 3, capture)
	dead.MarkDead()
	full := newTestLivePlayer(t, 4, capture)
	full.Inventory().SetLimiter(slotLimit(0))
	far := newTestLivePlayer(t, 100, capture) // 9900 from the origin

	eligible := l.partyLootEligible(20, origin)
	for _, tc := range []struct {
		what string
		m    *livePlayer
		want bool
	}{
		{"alive, with room, in range", alive, true},
		{"dead", dead, false},
		{"full", full, false},
		{"out of range", far, false},
	} {
		if got := eligible(tc.m); got != tc.want {
			t.Errorf("%s: eligible = %v, want %v", tc.what, got, tc.want)
		}
	}

	l.playerConfig.PartyRange = -1
	if !l.partyLootEligible(20, origin)(far) {
		t.Error("a far member is passed over with an unlimited party range")
	}
}

// TestShareAdenaSkipsMembersAtTheCap: a member whose adena is already at
// its cap is left out of the split and hears nothing; the others share
// the whole amount, and a share below one adena is still announced as 0.
func TestShareAdenaSkipsMembersAtTheCap(t *testing.T) {
	templates := testItemTemplates()
	capped := &testsupport.FrameCapture{}
	rich := newEquipTestLivePlayer(t, 1, capped, templates, []*item.Instance{
		{ObjectID: 500, OwnerID: 1, TemplateID: item.AdenaID, Count: math.MaxInt32, Location: item.LocationInventory},
	})
	first, second := &testsupport.FrameCapture{}, &testsupport.FrameCapture{}
	a := newEquipTestLivePlayer(t, 2, first, templates, nil)
	b := newEquipTestLivePlayer(t, 3, second, templates, nil)
	l := &GameClientLink{log: zerolog.Nop(), ids: &sequentialIDs{next: 9000}, playerConfig: PlayerConfig{PartyRange: 1500}}
	members := []*livePlayer{rich, a, b}

	l.shareAdena(members, 1, rich)
	if n := len(capped.Frames()); n != 0 {
		t.Fatalf("the capped member got %d frames, want none", n)
	}
	for i, c := range []*testsupport.FrameCapture{first, second} {
		frames := c.Frames()
		if len(frames) != 1 {
			t.Fatalf("member %d frames = %d, want one EARNED_S1_ADENA", i, len(frames))
		}
		id, params := decodeSystemMessage(t, frames[0])
		if id != serverpackets.SystemMessageEarnedS1Adena || !slices.Equal(params, []string{"1:0"}) {
			t.Fatalf("member %d: message %d %v, want EARNED_S1_ADENA 0", i, id, params)
		}
	}
	if got := a.Inventory().Adena() + b.Inventory().Adena(); got != 0 {
		t.Fatalf("a 0 share paid out %d adena", got)
	}

	l.shareAdena(members, 11, rich)
	if got := rich.Inventory().Adena(); got != math.MaxInt32 {
		t.Fatalf("the capped member holds %d adena, want it unchanged", got)
	}
	for i, m := range []*livePlayer{a, b} {
		if got := m.Inventory().Adena(); got != 5 {
			t.Fatalf("member %d holds %d adena, want 5 of 11 split two ways", i, got)
		}
	}
}
