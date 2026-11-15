package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestTradeStartHidesSummonedPetCollar pins the owner's TradeStart list
// against the available-item filter (ItemInstance.isAvailable): the collar of
// the pet that is out is left out, so the owner's window opens empty. With
// the pet not called out, the same collar is listed.
func TestTradeStartHidesSummonedPetCollar(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		summoned bool
	}{{"pet out", true}, {"pet not out", false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithItemTemplates(discardableCollarItems())})
			buyer := h.joinSecondPlayer(t, "Buyer")
			if tc.summoned {
				h.spawnWolf(t)
			}

			h.client.Send(encodeTradeRequest(buyer.id))
			drainUntilQuiet(t, buyer.client)
			buyer.client.Send(encodeAnswerTradeRequest(1))
			frames := readUntilOpcode(t, h.client, serverpackets.OpcodeTradeStart, "owner TradeStart")
			r := wire.NewReader(frames[len(frames)-1][1:])
			r.ReadInt32() // partner id
			rows := r.ReadUint16()
			var listed []int32
			for range rows {
				r.ReadUint16() // category
				listed = append(listed, r.ReadInt32())
				r.ReadInt32()  // template id
				r.ReadInt32()  // count
				r.ReadUint16() // sub-category
				r.ReadUint16() // custom type 1
				r.ReadInt32()  // body slot
				r.ReadUint16() // enchant
				r.ReadUint16() // custom type 2
				r.ReadUint16()
			}
			if err := r.Err(); err != nil {
				t.Fatalf("decode owner TradeStart: %v", err)
			}
			want := []int32{h.collarID}
			if tc.summoned {
				want = nil
			}
			if len(listed) != len(want) || (len(want) == 1 && listed[0] != want[0]) {
				t.Fatalf("owner TradeStart rows = %v, want %v", listed, want)
			}
		})
	}
}
