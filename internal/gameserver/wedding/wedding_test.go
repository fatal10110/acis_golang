package wedding

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// fakeSpouse is a player whose adena and request marks the test sets.
type fakeSpouse struct {
	id     int32
	female bool
	adena  int
	// payFails makes Pay fail although Adena reports enough: the adena
	// left between the manager's check and the payment.
	payFails bool
	// leaving refuses WhileAttached: the player is leaving the world.
	leaving  bool
	paid     int
	refunded int
}

func (s *fakeSpouse) ObjectID() int32         { return s.id }
func (s *fakeSpouse) Female() bool            { return s.female }
func (s *fakeSpouse) Adena() int              { return s.adena }
func (s *fakeSpouse) WearingFormalWear() bool { return true }

func (s *fakeSpouse) Pay(count int) bool {
	if s.payFails || s.adena < count {
		return false
	}
	s.adena -= count
	s.paid += count
	return true
}

func (s *fakeSpouse) Refund(count int) {
	s.adena += count
	s.refunded += count
}

func (s *fakeSpouse) WhileAttached(fn func()) bool {
	if s.leaving {
		return false
	}
	fn()
	return true
}

// fakeIDs hands out next, or err when set, and records released ids.
type fakeIDs struct {
	next     int32
	err      error
	released []int32
}

func (f *fakeIDs) NextID() (int32, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.next++
	return f.next, nil
}

func (f *fakeIDs) ReleaseID(id int32) { f.released = append(f.released, id) }

const testPrice = 1_000_000

func testConfig() Config { return Config{Price: testPrice, FormalWear: true} }

// pair returns a requester and a partner of opposite sex, each holding the
// price.
func pair() (requester, partner *fakeSpouse) {
	return &fakeSpouse{id: 100, adena: testPrice}, &fakeSpouse{id: 200, female: true, adena: testPrice}
}

// asked has requester ask partner on m, failing the test on a refusal.
func asked(t *testing.T, m *Manager, requester, partner *fakeSpouse) {
	t.Helper()
	if page := m.Ask(requester, partner, true); page != "" {
		t.Fatalf("Ask refused with %q", page)
	}
}

func onlineOf(players ...*fakeSpouse) func(int32) (Spouse, bool) {
	return func(id int32) (Spouse, bool) {
		for _, p := range players {
			if p.id == id {
				return p, true
			}
		}
		return nil, false
	}
}

func TestManagerGivesSharedPlayerToLowestCoupleAndReindexesOnDivorce(t *testing.T) {
	ids := &fakeIDs{}
	m := NewManager(testConfig(), ids, []Couple{
		{ID: 7, RequesterID: 1, PartnerID: 2},
		{ID: 9, RequesterID: 4, PartnerID: 5},
		{ID: 5, RequesterID: 3, PartnerID: 1},
	}, nil)
	for _, tc := range []struct{ player, couple, partner int32 }{
		{1, 5, 3}, {2, 7, 1}, {3, 5, 1}, {4, 9, 5}, {5, 9, 4},
	} {
		if got := m.CoupleID(tc.player); got != tc.couple {
			t.Errorf("CoupleID(%d) = %d, want %d", tc.player, got, tc.couple)
		}
		if got := m.PartnerID(tc.player); got != tc.partner {
			t.Errorf("PartnerID(%d) = %d, want %d", tc.player, got, tc.partner)
		}
	}

	c, ok := m.Divorce(1)
	if !ok || c != (Couple{ID: 5, RequesterID: 3, PartnerID: 1}) {
		t.Fatalf("Divorce(1) = %+v, %v; want couple 5", c, ok)
	}
	if !slices.Equal(ids.released, []int32{5}) {
		t.Errorf("released ids = %v, want [5]", ids.released)
	}
	for _, tc := range []struct{ player, couple, partner int32 }{
		{1, 7, 2}, {2, 7, 1}, {3, 0, 0}, {4, 9, 5},
	} {
		if got := m.CoupleID(tc.player); got != tc.couple {
			t.Errorf("after divorce CoupleID(%d) = %d, want %d", tc.player, got, tc.couple)
		}
		if got := m.PartnerID(tc.player); got != tc.partner {
			t.Errorf("after divorce PartnerID(%d) = %d, want %d", tc.player, got, tc.partner)
		}
	}
	want := []Couple{{ID: 7, RequesterID: 1, PartnerID: 2}, {ID: 9, RequesterID: 4, PartnerID: 5}}
	if got := m.Couples(); !slices.Equal(got, want) {
		t.Errorf("Couples() = %v, want %v", got, want)
	}
	if _, ok := m.Divorce(3); ok {
		t.Error("Divorce(3) of an unmarried player reported a couple")
	}
}

// pending fails the test unless each of players has a pending request
// exactly as want says.
func pending(t *testing.T, m *Manager, want bool, players ...*fakeSpouse) {
	t.Helper()
	for _, p := range players {
		if got := m.Pending(p.id); got != want {
			t.Errorf("Pending(%d) = %v, want %v", p.id, got, want)
		}
	}
}

// An answer finding its requester offline ends the request for both, so
// neither stays waiting on it (#5100).
func TestAnswerWithRequesterOfflineEndsRequest(t *testing.T) {
	m := NewManager(testConfig(), &fakeIDs{}, nil, nil)
	requester, partner := pair()
	asked(t, m, requester, partner)

	out := m.Answer(partner, true, onlineOf(partner))
	if out.Outcome != AnswerIgnored {
		t.Fatalf("Outcome = %v, want AnswerIgnored", out.Outcome)
	}
	pending(t, m, false, requester, partner)
	if requester.paid != 0 || partner.paid != 0 || len(m.Couples()) != 0 {
		t.Errorf("ignored answer paid %d/%d or made couples %v", requester.paid, partner.paid, m.Couples())
	}
	if out := m.Answer(partner, true, onlineOf(requester, partner)); out.Outcome != AnswerIgnored {
		t.Fatalf("Outcome of a second answer = %v, want AnswerIgnored", out.Outcome)
	}
}

// A request lapses after RequestTimeout: a late answer finds none, and both
// players may ask again.
func TestRequestTimesOut(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	m := NewManager(testConfig(), &fakeIDs{}, nil, func() time.Time { return now })
	requester, partner := pair()
	asked(t, m, requester, partner)
	now = now.Add(RequestTimeout - time.Nanosecond)
	pending(t, m, true, requester, partner)
	now = now.Add(time.Nanosecond)
	pending(t, m, false, requester, partner)
	if out := m.Answer(partner, true, onlineOf(requester, partner)); out.Outcome != AnswerIgnored {
		t.Fatalf("Outcome of a late answer = %v, want AnswerIgnored", out.Outcome)
	}
	asked(t, m, partner, requester)
}

// A player under a request can neither ask nor be asked, and only the
// partner of a request answers it.
func TestOneRequestPerPlayer(t *testing.T) {
	m := NewManager(testConfig(), &fakeIDs{}, nil, nil)
	requester, partner := pair()
	rival := &fakeSpouse{id: 300, adena: testPrice}
	other := &fakeSpouse{id: 400, female: true, adena: testPrice}
	asked(t, m, requester, partner)
	if page := m.Ask(rival, partner, true); page != PageWaiting {
		t.Errorf("Ask of an asked partner = %q, want %q", page, PageWaiting)
	}
	if page := m.Ask(requester, other, true); page != PageWaiting {
		t.Errorf("Ask by a waiting requester = %q, want %q", page, PageWaiting)
	}
	pending(t, m, false, rival, other)
	if out := m.Answer(requester, true, onlineOf(requester, partner)); out.Outcome != AnswerIgnored {
		t.Errorf("the requester's own answer = %v, want AnswerIgnored", out.Outcome)
	}
	pending(t, m, true, requester, partner)
	m.Leave(requester.id)
	pending(t, m, false, requester, partner)
}

// A player leaving the world can neither ask nor be asked.
func TestAskInvolvingLeavingPlayerFindsNobody(t *testing.T) {
	for _, requesterLeaves := range []bool{true, false} {
		m := NewManager(testConfig(), &fakeIDs{}, nil, nil)
		requester, partner := pair()
		requester.leaving, partner.leaving = requesterLeaves, !requesterLeaves
		if page := m.Ask(requester, partner, true); page != PageNotFound {
			t.Errorf("requester leaving=%v: Ask = %q, want %q", requesterLeaves, page, PageNotFound)
		}
		pending(t, m, false, requester, partner)
	}
}

func TestAnswerVoidWithoutCoupleIDClearsRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  IDs
	}{
		{"nil ids", nil},
		{"NextID error", &fakeIDs{err: errors.New("exhausted")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(testConfig(), tc.ids, nil, nil)
			requester, partner := pair()
			asked(t, m, requester, partner)

			out := m.Answer(partner, true, onlineOf(requester, partner))
			if out.Outcome != AnswerVoid || out.Requester != Spouse(requester) {
				t.Fatalf("Answer = %+v, want AnswerVoid from the requester", out)
			}
			pending(t, m, false, requester, partner)
			if requester.paid != 0 || partner.paid != 0 || len(m.Couples()) != 0 {
				t.Errorf("void answer paid %d/%d or made couples %v", requester.paid, partner.paid, m.Couples())
			}
		})
	}
}

func TestAnswerAcceptedChargesBothThenMarries(t *testing.T) {
	ids := &fakeIDs{next: 40}
	m := NewManager(testConfig(), ids, nil, nil)
	requester, partner := pair()
	asked(t, m, requester, partner)

	out := m.Answer(partner, true, onlineOf(requester, partner))
	if out.Outcome != AnswerMarried {
		t.Fatalf("Outcome = %v, want AnswerMarried", out.Outcome)
	}
	if requester.paid != testPrice || partner.paid != testPrice {
		t.Errorf("paid %d/%d, want %d each", requester.paid, partner.paid, testPrice)
	}
	if got := m.Couples(); !slices.Equal(got, []Couple{{ID: 41, RequesterID: 100, PartnerID: 200}}) {
		t.Errorf("Couples() = %v", got)
	}
}

// A spouse whose adena leaves between the check and the payment (a drop
// or trade on its own packet queue while the partner's answer runs) ends
// the request unmarried, and nobody keeps a payment.
func TestAnswerPaymentFailureMarriesNobodyAndRefunds(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		requesterFails, partnerFails bool
	}{
		{"requester adena gone", true, false},
		{"partner adena gone", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := &fakeIDs{next: 40}
			m := NewManager(testConfig(), ids, nil, nil)
			requester, partner := pair()
			asked(t, m, requester, partner)
			requester.payFails, partner.payFails = tc.requesterFails, tc.partnerFails

			out := m.Answer(partner, true, onlineOf(requester, partner))
			if out.Outcome != AnswerUnpaid || out.RequesterShort != tc.requesterFails || out.PartnerShort != tc.partnerFails {
				t.Fatalf("Answer = %+v, want AnswerUnpaid naming the failed spouse", out)
			}
			if len(m.Couples()) != 0 || m.CoupleID(requester.id) != 0 || m.CoupleID(partner.id) != 0 {
				t.Errorf("unpaid answer made couples %v", m.Couples())
			}
			if requester.adena != testPrice || partner.adena != testPrice {
				t.Errorf("adena after unpaid answer %d/%d, want %d each", requester.adena, partner.adena, testPrice)
			}
			if !slices.Equal(ids.released, []int32{41}) {
				t.Errorf("released ids = %v, want [41]", ids.released)
			}
			pending(t, m, false, requester, partner)
		})
	}
}

func TestFormatNumber(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1,000"},
		{9999, "9,999"},
		{12345, "12,345"},
		{999999, "999,999"},
		{1000000, "1,000,000"},
		{2147483647, "2,147,483,647"},
		{9223372036854775807, "9,223,372,036,854,775,807"},
		{-1, "-1"},
		{-1234, "-1,234"},
		{-1000000, "-1,000,000"},
	} {
		if got := formatNumber(tc.n); got != tc.want {
			t.Errorf("formatNumber(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// TestAnswerToLeavingRequesterIsIgnored: a requester leaving the world is
// answered as if offline, accepted or not: nobody pays, no couple forms,
// and the request ends.
func TestAnswerToLeavingRequesterIsIgnored(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		m := NewManager(testConfig(), &fakeIDs{next: 1}, nil, nil)
		requester, partner := pair()
		asked(t, m, requester, partner)
		requester.leaving = true
		if out := m.Answer(partner, accepted, onlineOf(requester, partner)); out.Outcome != AnswerIgnored {
			t.Fatalf("accepted=%v: outcome %v, want ignored", accepted, out.Outcome)
		}
		if requester.paid != 0 || partner.paid != 0 || m.CoupleID(requester.id) != 0 {
			t.Fatalf("accepted=%v: paid %d/%d, couple %d; want nothing", accepted, requester.paid, partner.paid, m.CoupleID(requester.id))
		}
		pending(t, m, false, requester, partner)
	}
}
