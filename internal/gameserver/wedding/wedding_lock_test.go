package wedding

import (
	"sync"
	"testing"
	"time"
)

// deadlockBound is how long a call may take before the test reads it as a
// lock-order deadlock.
const deadlockBound = 30 * time.Second

// hookedSpouse is a fakeSpouse safe for concurrent use whose Pay first runs
// onPay, as a payment reaching other code would.
type hookedSpouse struct {
	mu sync.Mutex
	fakeSpouse
	onPay func()
}

func (s *hookedSpouse) Adena() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeSpouse.Adena()
}

func (s *hookedSpouse) Pay(count int) bool {
	if s.onPay != nil {
		s.onPay()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeSpouse.Pay(count)
}

func (s *hookedSpouse) Refund(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fakeSpouse.Refund(count)
}

func (s *hookedSpouse) charged() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paid - s.refunded
}

func onlineSpouses(players ...Spouse) func(int32) (Spouse, bool) {
	return func(id int32) (Spouse, bool) {
		for _, p := range players {
			if p.ObjectID() == id {
				return p, true
			}
		}
		return nil, false
	}
}

// answerWithin runs Answer on its own goroutine and fails the test when it
// has not returned within deadlockBound.
func answerWithin(t *testing.T, m *Manager, partner Spouse, online func(int32) (Spouse, bool)) Answer {
	t.Helper()
	done := make(chan Answer, 1)
	go func() { done <- m.Answer(partner, true, online) }()
	select {
	case a := <-done:
		return a
	case <-time.After(deadlockBound):
		t.Fatal("Answer did not return: the manager's lock is held across a payment that calls back into it")
		return Answer{}
	}
}

// A payment runs outside the manager's lock: whatever it reaches may read
// and change the manager's requests and couples.
func TestPaymentMayCallBackIntoTheManager(t *testing.T) {
	m := NewManager(testConfig(), &fakeIDs{}, []Couple{{ID: 900, RequesterID: 300, PartnerID: 400}}, nil)
	requester := &hookedSpouse{fakeSpouse: fakeSpouse{id: 100, adena: testPrice}}
	partner := &hookedSpouse{fakeSpouse: fakeSpouse{id: 200, female: true, adena: testPrice}}
	if page := m.Ask(requester, partner, true); page != "" {
		t.Fatalf("Ask refused with %q", page)
	}
	other, otherPartner := &fakeSpouse{id: 500, adena: testPrice}, &fakeSpouse{id: 600, female: true, adena: testPrice}
	var askedDuringPay string
	partner.onPay = func() {
		m.Leave(requester.id)
		m.Pending(partner.id)
		m.Greeting(partner)
		m.CoupleID(requester.id)
		m.PartnerID(partner.id)
		m.Couples()
		m.Divorce(300)
		askedDuringPay = m.Ask(other, otherPartner, true)
	}

	got := answerWithin(t, m, partner, onlineSpouses(requester, partner))
	if got.Outcome != AnswerMarried {
		t.Fatalf("Answer outcome = %v, want married", got.Outcome)
	}
	if askedDuringPay != "" {
		t.Fatalf("Ask during the payment refused with %q", askedDuringPay)
	}
	if m.PartnerID(requester.id) != partner.id || m.CoupleID(300) != 0 {
		t.Fatalf("couples = %+v, want the new couple and the divorced one gone", m.Couples())
	}
	if !m.Pending(other.id) {
		t.Fatal("the request asked during the payment is not pending")
	}
}

// A requester whose answered request is still paying may ask again; a
// second marriage answered meanwhile waits for the first and finds the
// requester married, so it charges nobody.
func TestConcurrentAnswersForOneRequesterMarryOnce(t *testing.T) {
	m := NewManager(testConfig(), &fakeIDs{}, nil, nil)
	requester := &hookedSpouse{fakeSpouse: fakeSpouse{id: 100, adena: 3 * testPrice}}
	first := &hookedSpouse{fakeSpouse: fakeSpouse{id: 200, female: true, adena: testPrice}}
	second := &hookedSpouse{fakeSpouse: fakeSpouse{id: 300, female: true, adena: testPrice}}
	online := onlineSpouses(requester, first, second)
	if page := m.Ask(requester, first, true); page != "" {
		t.Fatalf("Ask refused with %q", page)
	}
	secondDone := make(chan Answer, 1)
	var asked string
	first.onPay = func() {
		first.onPay = nil
		// The first request is answered and dropped, so the requester is
		// free to ask; the second answer runs while the first pays.
		asked = m.Ask(requester, second, true)
		go func() { secondDone <- m.Answer(second, true, online) }()
	}

	if got := answerWithin(t, m, first, online); got.Outcome != AnswerMarried {
		t.Fatalf("first Answer outcome = %v, want married", got.Outcome)
	}
	if asked != "" {
		t.Fatalf("second Ask refused with %q", asked)
	}
	var got Answer
	select {
	case got = <-secondDone:
	case <-time.After(deadlockBound):
		t.Fatal("second Answer did not return")
	}
	if got.Outcome != AnswerVoid {
		t.Fatalf("second Answer outcome = %v, want void", got.Outcome)
	}
	if n := len(m.Couples()); n != 1 {
		t.Fatalf("couples = %+v, want one", m.Couples())
	}
	if requester.charged() != testPrice || first.charged() != testPrice || second.charged() != 0 {
		t.Fatalf("charged requester %d, first %d, second %d; want %d, %d, 0",
			requester.charged(), first.charged(), second.charged(), testPrice, testPrice)
	}
}
