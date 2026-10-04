package network

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/lottery"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// noIDs is an exhausted object id factory.
type noIDs struct{}

func (noIDs) NextID() (int32, error) { return 0, errors.New("id factory exhausted") }

// drawnRoundStore holds round 0 drawn with draw, and nothing else.
type drawnRoundStore struct{ draw lottery.Draw }

func (s drawnRoundStore) LoadRounds(context.Context) ([]lottery.StoredRound, error) {
	return []lottery.StoredRound{{ID: 0, Finished: true, Draw: s.draw}}, nil
}
func (drawnRoundStore) InsertRound(context.Context, int32, int64, int32) error { return nil }
func (drawnRoundStore) SavePrize(context.Context, int32, int32) error          { return nil }
func (drawnRoundStore) FinishRound(context.Context, int32, int32, int32, lottery.Draw) error {
	return nil
}

func (drawnRoundStore) Tickets(context.Context, int32) ([]lottery.Numbers, error) { return nil, nil }

func lotteryTestTemplates() *item.Table {
	etc := func(id int32, name string, stackable bool) *item.Template {
		return &item.Template{
			ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, Stackable: stackable,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true,
			EtcItem: &item.EtcItemDetail{},
		}
	}
	return item.NewTable([]*item.Template{etc(item.AdenaID, "Adena", true), etc(lottery.TicketID, "Lottery Ticket", false)})
}

func firstFivePicks() lottery.Picks {
	var p lottery.Picks
	for n := 1; n <= 5; n++ {
		p.Press(n)
	}
	return p
}

// TestLotteryPurchaseWithoutObjectIDChargesNothing: when no object id can be
// had for the ticket, the purchase is refused before the price is taken or
// the jackpot raised, and nothing is said to the buyer.
func TestLotteryPurchaseWithoutObjectIDChargesNothing(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, lotteryTestTemplates(), []*item.Instance{
		{ObjectID: 500, OwnerID: 1, TemplateID: item.AdenaID, Count: 10000, Location: item.LocationInventory},
	})
	live.lottoPicks = firstFivePicks()
	lot := lottery.New(lottery.DefaultConfig(), drawnRoundStore{}, nil, nil, idleQueue(), zerolog.Nop())
	l := &GameClientLink{log: zerolog.Nop(), ids: noIDs{}, lottery: lot}
	prize := lot.Status().Prize

	if l.buyLotteryTicket(live) {
		t.Fatal("a purchase without a ticket id went through")
	}
	if got := live.Inventory().Adena(); got != 10000 {
		t.Fatalf("adena = %d after a refused purchase, want 10000", got)
	}
	if got := lot.Status().Prize; got != prize {
		t.Fatalf("jackpot = %d after a refused purchase, want %d", got, prize)
	}
	if n := len(live.Inventory().ItemsByTemplateID(lottery.TicketID)); n != 0 {
		t.Fatalf("%d tickets held after a refused purchase", n)
	}
	if n := len(capture.Frames()); n != 0 {
		t.Fatalf("a refused purchase sent %d frames, want none", n)
	}
}

// TestLotteryClaimWithoutObjectIDKeepsTicket: when no object id can be had
// for a winning ticket's payout, the ticket is kept rather than destroyed
// for nothing, so it can be claimed again later.
func TestLotteryClaimWithoutObjectIDKeepsTicket(t *testing.T) {
	picks := firstFivePicks()
	numbers, _ := picks.Numbers()
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, lotteryTestTemplates(), []*item.Instance{
		{
			ObjectID: 600, OwnerID: 1, TemplateID: lottery.TicketID, Count: 1, Location: item.LocationInventory,
			CustomType1: 0, EnchantLevel: int(numbers.Low), CustomType2: int(numbers.High),
		},
	})
	lot := lottery.New(lottery.DefaultConfig(), drawnRoundStore{draw: lottery.Draw{Numbers: numbers, Prize1: 5000}}, nil, nil, idleQueue(), zerolog.Nop())
	if err := lot.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, adena := lot.Check(0, numbers); adena != 5000 {
		t.Fatalf("the ticket wins %d, want the 5000 first prize", adena)
	}
	l := &GameClientLink{log: zerolog.Nop(), ids: noIDs{}, lottery: lot}

	l.claimLotteryTicket(live, 600)
	if live.Inventory().ItemByObjectID(600) == nil {
		t.Fatal("the winning ticket was destroyed without a payout")
	}
	if got := live.Inventory().Adena(); got != 0 {
		t.Fatalf("adena = %d, want 0", got)
	}
	if n := len(capture.Frames()); n != 0 {
		t.Fatalf("a refused claim sent %d frames, want none", n)
	}

	l.ids = &sequentialIDs{next: 9000}
	l.claimLotteryTicket(live, 600)
	if live.Inventory().ItemByObjectID(600) != nil {
		t.Fatal("the ticket survived its claim")
	}
	if got := live.Inventory().Adena(); got != 5000 {
		t.Fatalf("adena = %d after the claim, want 5000", got)
	}
}

// countingIDs hands out ids from next, counts every id taken and tracks
// those not yet given back.
type countingIDs struct {
	next        int32
	taken       int
	outstanding map[int32]bool
}

func (c *countingIDs) NextID() (int32, error) {
	c.taken++
	c.next++
	if c.outstanding == nil {
		c.outstanding = map[int32]bool{}
	}
	c.outstanding[c.next] = true
	return c.next, nil
}

func (c *countingIDs) ReleaseID(id int32) { delete(c.outstanding, id) }

// TestLotteryUnpaidPurchaseHoldsNoObjectID: a purchase refused for want of
// adena gives back the ticket id it took, however often it is repeated, so
// resending the form's buy link cannot drain the id factory.
func TestLotteryUnpaidPurchaseHoldsNoObjectID(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, lotteryTestTemplates(), []*item.Instance{
		{ObjectID: 500, OwnerID: 1, TemplateID: item.AdenaID, Count: 1, Location: item.LocationInventory},
	})
	live.lottoPicks = firstFivePicks()
	lot := lottery.New(lottery.DefaultConfig(), drawnRoundStore{}, nil, nil, idleQueue(), zerolog.Nop())
	ids := &countingIDs{next: 9000}
	l := &GameClientLink{log: zerolog.Nop(), ids: ids, lottery: lot}
	prize := lot.Status().Prize

	for range 100 {
		if l.buyLotteryTicket(live) {
			t.Fatal("an unpaid purchase went through")
		}
	}
	// The production allocator never moves its cursor back, so a taken id
	// is used up even when released: an unpaid purchase must take none.
	if ids.taken != 0 {
		t.Fatalf("%d object ids taken by 100 unpaid purchases, want none", ids.taken)
	}
	if n := len(ids.outstanding); n != 0 {
		t.Fatalf("%d object ids held after 100 unpaid purchases, want none", n)
	}
	// The capture records payloads, without the 2-byte length prefix.
	refusal := serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena).Bytes()[2:]
	frames := capture.Frames()
	if len(frames) != 100 {
		t.Fatalf("100 unpaid purchases sent %d frames, want 100", len(frames))
	}
	for i, f := range frames {
		if !bytes.Equal(f, refusal) {
			t.Fatalf("frame %d of the unpaid purchases = % x, want not enough adena % x", i, f, refusal)
		}
	}
	if got := live.Inventory().Adena(); got != 1 {
		t.Fatalf("adena = %d after unpaid purchases, want 1", got)
	}
	if got := lot.Status().Prize; got != prize {
		t.Fatalf("jackpot = %d after unpaid purchases, want %d", got, prize)
	}

	if live.Inventory().AddNew(item.AdenaID, int(lottery.DefaultConfig().TicketPrice), 501) == nil {
		t.Fatal("could not give the buyer the ticket price")
	}
	if !l.buyLotteryTicket(live) {
		t.Fatal("a paid purchase was refused")
	}
	if ids.taken != 1 {
		t.Fatalf("%d object ids taken by one paid purchase, want the ticket's one", ids.taken)
	}
	if n := len(ids.outstanding); n != 1 {
		t.Fatalf("%d object ids held after one paid purchase, want the ticket's one", n)
	}
}
