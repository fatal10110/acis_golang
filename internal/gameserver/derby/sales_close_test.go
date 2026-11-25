package derby

import "testing"

// purse pays ticket prices from amount, recording each payment; onPay runs
// as the payment is asked for.
type purse struct {
	noTickets
	amount int
	paid   []int
	onPay  func()
}

func (p *purse) Pay(price int) bool {
	if p.onPay != nil {
		p.onPay()
	}
	if price > p.amount {
		return false
	}
	p.amount -= price
	p.paid = append(p.paid, price)
	return true
}

// TestBuyTicketStakesBeforeTheSalesClose pins #5119: the sales-open check,
// the payment and the stake share one hold of the track's lock. A closing
// tick started while a purchase is being paid for waits for it, so the
// stake makes the odds; once closed, a purchase is refused without paying.
func TestBuyTicketStakesBeforeTheSalesClose(t *testing.T) {
	store := &fakeStore{}
	track := newTestTrack(t, store)
	for range 900 {
		track.Tick()
	}

	picks := Picks{Lane: 2, Price: 3}
	closed := make(chan struct{})
	buyer := &purse{amount: 1000}
	buyer.onPay = func() {
		go func() {
			track.Tick()
			close(closed)
		}()
	}
	reply := track.Bypass(noPages{}, 30995, 1, &picks, buyer, "BuyTicket 21")
	<-closed
	if reply.Buy == nil || *reply.Buy != (Purchase{Race: 1, Lane: 2, Price: 1000}) || !reply.Chat {
		t.Fatalf("purchase as the sales close = %+v, want a 1000-adena ticket on lane 2", reply)
	}
	if buyer.amount != 0 || picks != (Picks{}) {
		t.Fatalf("after buying: purse %d, picks %+v, want 0 and none", buyer.amount, picks)
	}
	if got := track.Stakes(); got != ([Lanes]int64{1: 1000}) {
		t.Fatalf("stakes = %v, want 1000 on lane 2", got)
	}
	if want := []float64{0, 1.25, 0, 0, 0, 0, 0, 0}; len(track.odds) != len(want) || track.odds[1] != want[1] {
		t.Fatalf("odds = %v, want %v: the stake sold before the close is counted", track.odds, want)
	}

	late := &purse{amount: 1000}
	picks = Picks{Lane: 3, Price: 1}
	if reply := track.Bypass(noPages{}, 30995, 1, &picks, late, "BuyTicket 21"); reply != (Reply{Notice: MsgTicketsNotAvailable, Chat: true}) {
		t.Fatalf("purchase once closed = %+v, want the tickets-not-available notice", reply)
	}
	if late.amount != 1000 || len(late.paid) != 0 {
		t.Fatalf("purchase once closed paid %v, want nothing", late.paid)
	}
	if got := track.Stakes(); got != ([Lanes]int64{1: 1000}) {
		t.Fatalf("stakes after a refused purchase = %v, want 1000 on lane 2 only", got)
	}
}
