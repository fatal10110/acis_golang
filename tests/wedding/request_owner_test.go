package wedding

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// carol brings Carol, a man holding twice the price and on Bobby's friend
// list, into the chapel.
func (c *chapel) carol(t *testing.T) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	id := c.srv.SeedCharacterFor(t, "player3", "Carol", 20, 0).ID
	c.srv.GiveItem(t, id, item.AdenaID, 2_000_000)
	client := c.srv.DialClient(t, "player3", 1)
	enter(t, c.srv, client)
	c.srv.Relations.AddFriend(id, c.bobbyID)
	drainFrames(t, c.alice)
	drainFrames(t, c.bobby)
	return client, id
}

// ask has client ask name from the request form, failing the test on a
// refusal page or unless the request dialog reaches to.
func (c *chapel) ask(t *testing.T, client *testsupport.ScriptedClient, name string, to *testsupport.ScriptedClient) {
	t.Helper()
	c.talk(t, client)
	if frames := c.bypass(t, client, "AskWedding "+name); len(only(frames, serverpackets.OpcodeNpcHtmlMessage)) != 0 {
		t.Fatalf("asking %s answered %x, want no page", name, opcodes(frames))
	}
	if got := only(drainFrames(t, to), serverpackets.OpcodeConfirmDlg); len(got) != 1 {
		t.Fatalf("%s got %d request dialogs, want 1", name, len(got))
	}
}

// logout takes client's character objID out of the world.
func (c *chapel) logout(t *testing.T, client *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	client.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	c.srv.AdvanceUntil(t, "player leaves the world", func() bool {
		_, ok := c.srv.State.Player(objID)
		return !ok
	})
}

// assertGreeting checks the page client's talk to the manager opens.
func (c *chapel) assertGreeting(t *testing.T, what string, client *testsupport.ScriptedClient, waiting bool) {
	t.Helper()
	want := c.page(t, "start.htm", "1,000,000", "won't")
	if waiting {
		want = c.page(t, "waitforpartner.htm", "", "")
	}
	c.assertPage(t, what, c.talk(t, client), want)
}

// assertIgnored sends Bobby's yes to a request that is gone and checks
// that nobody is told anything or shown a wedding, and nobody marries or
// pays.
func (c *chapel) assertIgnored(t *testing.T, what string, others ...*testsupport.ScriptedClient) {
	t.Helper()
	answer(t, c.bobby, true)
	for _, client := range append([]*testsupport.ScriptedClient{c.bobby}, others...) {
		if frames := drainFrames(t, client); len(only(frames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeMagicSkillUse)) != 0 {
			t.Fatalf("%s: frames %x, want no message or wedding skill", what, opcodes(frames))
		}
	}
	if c.srv.Couples.CoupleID(c.bobbyID) != 0 || c.adena(t, c.bobbyID) != 2_000_000 {
		t.Fatalf("%s married or charged Bobby", what)
	}
}

// TestWeddingSecondSuitorCannotRedirectAnswer: while Bobby has Alice's
// request to answer, Carol's request to Bobby is refused with the waiting
// page and reaches nobody, so Bobby's yes marries Alice and Carol pays
// nothing (#5100).
func TestWeddingSecondSuitorCannotRedirectAnswer(t *testing.T) {
	c := bootChapel(t, defaultChapel())
	carol, carolID := c.carol(t)
	c.ask(t, c.alice, "Bobby", c.bobby)

	c.talk(t, carol)
	c.assertPage(t, "Carol asking an asked partner", c.bypass(t, carol, "AskWedding Bobby"), c.page(t, "waitforpartner.htm", "", ""))
	if frames := only(drainFrames(t, c.bobby), serverpackets.OpcodeConfirmDlg); len(frames) != 0 {
		t.Fatal("a second request reached Bobby")
	}
	c.assertGreeting(t, "Carol after the refusal", carol, false)
	c.assertGreeting(t, "Bobby while asked", c.bobby, true)

	answer(t, c.bobby, true)
	drainFrames(t, c.alice)
	drainFrames(t, c.bobby)
	if got := c.srv.Couples.PartnerID(c.bobbyID); got != c.aliceID {
		t.Fatalf("Bobby's yes married %d, want Alice %d", got, c.aliceID)
	}
	if c.srv.Couples.CoupleID(carolID) != 0 || c.adena(t, carolID) != 2_000_000 {
		t.Fatal("Carol was married or charged")
	}
}

// TestWeddingLogoutEndsRequest: a requester or a partner leaving the world
// ends the request for both, so the one left behind is greeted with the
// request form, a late answer does nothing, and a fresh request goes out
// and marries (#5100).
func TestWeddingLogoutEndsRequest(t *testing.T) {
	t.Run("requester leaves", func(t *testing.T) {
		c := bootChapel(t, defaultChapel())
		carol, carolID := c.carol(t)
		c.ask(t, c.alice, "Bobby", c.bobby)
		c.logout(t, c.alice, c.aliceID)
		drainFrames(t, c.bobby)
		drainFrames(t, carol)

		c.assertGreeting(t, "Bobby after Alice left", c.bobby, false)
		c.assertIgnored(t, "Bobby's yes to a requester gone", carol)

		c.ask(t, carol, "Bobby", c.bobby)
		answer(t, c.bobby, true)
		drainFrames(t, c.bobby)
		drainFrames(t, carol)
		if got := c.srv.Couples.PartnerID(c.bobbyID); got != carolID {
			t.Fatalf("Bobby's yes married %d, want Carol %d", got, carolID)
		}
	})
	t.Run("partner leaves", func(t *testing.T) {
		c := bootChapel(t, defaultChapel())
		carol, _ := c.carol(t)
		c.ask(t, carol, "Bobby", c.bobby)
		c.logout(t, c.bobby, c.bobbyID)
		drainFrames(t, c.alice)
		drainFrames(t, carol)

		c.assertGreeting(t, "Carol after Bobby left", carol, false)
		if c.srv.Couples.Pending(c.bobbyID) {
			t.Fatal("Bobby left still under the request")
		}
	})
}

// TestWeddingRequestLapses: a request unanswered for wedding.RequestTimeout
// lapses: both are greeted with the request form, Bobby's late yes does
// nothing, and Alice's fresh request goes out (#5100).
func TestWeddingRequestLapses(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Unix(1_000_000, 0).UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	opts := defaultChapel()
	opts.extra = []gameservertest.Option{gameservertest.WithTradeClock(now)}
	c := bootChapel(t, opts)
	c.ask(t, c.alice, "Bobby", c.bobby)

	clock.Add(int64(wedding.RequestTimeout - time.Nanosecond))
	c.assertGreeting(t, "Bobby just before the request lapses", c.bobby, true)
	clock.Add(int64(time.Nanosecond))
	c.assertGreeting(t, "Alice after the request lapsed", c.alice, false)
	c.assertGreeting(t, "Bobby after the request lapsed", c.bobby, false)
	c.assertIgnored(t, "Bobby's yes to a lapsed request", c.alice)

	c.ask(t, c.alice, "Bobby", c.bobby)
	answer(t, c.bobby, true)
	drainFrames(t, c.alice)
	drainFrames(t, c.bobby)
	if got := c.srv.Couples.PartnerID(c.bobbyID); got != c.aliceID {
		t.Fatalf("Bobby's yes to the fresh request married %d, want Alice %d", got, c.aliceID)
	}
}
