package wedding

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
)

// The answers of WeddingManagerNpc (WeddingManagerNpc.java). An interact
// opens its page alone, with no talk animation and no ActionFailed after
// the page (sendHtmlMessage sends none); the interact's own ActionFailed
// and MoveToPawn come first. A command answers with its page, if any, then
// the dispatcher's ActionFailed (RequestBypassToServer.java:118).
var (
	greetingOrder = []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeNpcHtmlMessage}
	refusalOrder  = []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
)

// greetingFrames keeps the frames of an interact's answer.
func greetingFrames(frames [][]byte) [][]byte {
	return only(frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction, serverpackets.OpcodeNpcHtmlMessage)
}

// TestWeddingMarriesAskedFriend walks the whole wedding: the request form,
// the request (a ConfirmDlg 1983 naming the requester to the partner, and
// the pending page to both), the accept (each pays the price, is
// congratulated, shows the wedding march and fireworks, and everyone hears
// the announcement), the married menu, then a divorce. The couple reaches
// mods_wedding on save.
func TestWeddingMarriesAskedFriend(t *testing.T) {
	c := bootChapel(t, defaultChapel())

	frames := greetingFrames(c.talk(t, c.alice))
	if got := opcodes(frames); string(got) != string(greetingOrder) {
		t.Fatalf("greeting = %x, want %x", got, greetingOrder)
	}
	c.assertPage(t, "request form", frames, c.page(t, "start.htm", "1,000,000", "won't"))

	frames = c.bypass(t, c.alice, "AskWedding Bobby")
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("request answer to Alice = %x, want ActionFailed alone", got)
	}
	dialogs := only(drainFrames(t, c.bobby), serverpackets.OpcodeConfirmDlg)
	if len(dialogs) != 1 {
		t.Fatalf("Bobby received %d ConfirmDlg, want 1", len(dialogs))
	}
	want := serverpackets.FrameConfirmDlgEngageRequest("Alice asked you to marry. Do you want to start a new relationship ?")
	// Frame bytes carry the two-byte length the client frames strip.
	if string(dialogs[0]) != string(want.Bytes()[2:]) {
		t.Fatalf("ConfirmDlg = %x, want %x", dialogs[0], want.Bytes()[2:])
	}
	want.Release()
	c.assertPage(t, "Alice while asking", c.talk(t, c.alice), c.page(t, "waitforpartner.htm", "", ""))
	c.assertPage(t, "Bobby while asked", c.talk(t, c.bobby), c.page(t, "waitforpartner.htm", "", ""))

	answer(t, c.bobby, true)
	aliceFrames, bobbyFrames := drainFrames(t, c.alice), drainFrames(t, c.bobby)
	coupleID := c.srv.Couples.CoupleID(c.aliceID)
	if coupleID == 0 || c.srv.Couples.CoupleID(c.bobbyID) != coupleID || c.srv.Couples.PartnerID(c.bobbyID) != c.aliceID {
		t.Fatalf("couple ids = %d/%d, want one couple", coupleID, c.srv.Couples.CoupleID(c.bobbyID))
	}
	if a, b := c.adena(t, c.aliceID), c.adena(t, c.bobbyID); a != 1_000_000 || b != 1_000_000 {
		t.Fatalf("adena after the wedding = %d/%d, want 1000000 each", a, b)
	}
	announcement := "Congratulations to Alice and Bobby! They have been married."
	for _, tc := range []struct {
		who    string
		frames [][]byte
		text   string
	}{
		{"Alice", aliceFrames, "Congratulations, you are now married with Bobby !"},
		{"Bobby", bobbyFrames, "Congratulations, you are now married with Alice !"},
	} {
		if got := systemMessageIDs(tc.frames); !slices.Equal(got, []int32{serverpackets.SystemMessageS1DisappearedAdena, serverpackets.SystemMessageS1}) {
			t.Fatalf("%s system messages = %v, want the adena spent, then the congratulation", tc.who, got)
		}
		if got := texts(t, tc.frames); !slices.Equal(got, []string{tc.text}) {
			t.Fatalf("%s texts = %q, want %q", tc.who, got, tc.text)
		}
		var casts [][2]int32
		for _, f := range only(tc.frames, serverpackets.OpcodeMagicSkillUse) {
			r := wire.NewReader(f[1:])
			caster, target, skill, level, hit, reuse := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			if caster != target || level != 1 || hit != 1 || reuse != 0 {
				t.Fatalf("%s MagicSkillUse = %d->%d skill %d level %d hit %d reuse %d", tc.who, caster, target, skill, level, hit, reuse)
			}
			casts = append(casts, [2]int32{caster, skill})
		}
		wantCasts := [][2]int32{{c.aliceID, 2230}, {c.bobbyID, 2230}, {c.aliceID, 2025}, {c.bobbyID, 2025}}
		if !slices.Equal(casts, wantCasts) {
			t.Fatalf("%s casts = %v, want %v", tc.who, casts, wantCasts)
		}
		says := only(tc.frames, serverpackets.OpcodeCreatureSay)
		if len(says) != 1 {
			t.Fatalf("%s CreatureSay count = %d, want 1", tc.who, len(says))
		}
		r := wire.NewReader(says[0][1:])
		if speaker, typ, name, text := r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadString(); speaker != 0 || typ != 10 || name != "" || text != announcement {
			t.Fatalf("%s announcement = %d %d %q %q, want 0 10 \"\" %q", tc.who, speaker, typ, name, text, announcement)
		}
		order := opcodes(only(tc.frames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeCreatureSay))
		wantOrder := []byte{
			serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage,
			serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeMagicSkillUse,
			serverpackets.OpcodeCreatureSay,
		}
		if string(order) != string(wantOrder) {
			t.Fatalf("%s wedding order = %x, want %x", tc.who, order, wantOrder)
		}
	}

	c.srv.SaveCouples(t)
	stored, err := gamesql.NewCoupleStore(c.srv.DB).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []wedding.Couple{{ID: coupleID, RequesterID: c.aliceID, PartnerID: c.bobbyID}}; !slices.Equal(stored, want) {
		t.Fatalf("mods_wedding = %+v, want %+v", stored, want)
	}
	// A restart reads the couple back: both spouses are married again.
	reloaded := wedding.NewManager(wedding.DefaultConfig(), nil, stored)
	if reloaded.CoupleID(c.aliceID) != coupleID || reloaded.PartnerID(c.aliceID) != c.bobbyID || reloaded.PartnerID(c.bobbyID) != c.aliceID {
		t.Fatal("the reloaded couples lost Alice and Bobby")
	}

	frames = greetingFrames(c.talk(t, c.bobby))
	if got := opcodes(frames); string(got) != string(greetingOrder) {
		t.Fatalf("married greeting = %x, want %x", got, greetingOrder)
	}
	c.assertPage(t, "married menu", frames, c.page(t, "start2.htm", "", ""))

	frames = c.bypass(t, c.bobby, "Divorce")
	if got := texts(t, frames); !slices.Equal(got, []string{"You are now divorced."}) {
		t.Fatalf("Bobby after the divorce = %q", got)
	}
	if got := opcodes(frames); got[len(got)-1] != serverpackets.OpcodeActionFailed {
		t.Fatalf("divorce answer = %x, want it to end with ActionFailed", got)
	}
	if got := texts(t, drainFrames(t, c.alice)); !slices.Equal(got, []string{"You are now divorced."}) {
		t.Fatalf("Alice after the divorce = %q", got)
	}
	if c.srv.Couples.CoupleID(c.aliceID) != 0 || c.srv.Couples.CoupleID(c.bobbyID) != 0 {
		t.Fatal("still married after the divorce")
	}
	c.srv.SaveCouples(t)
	if stored, err := gamesql.NewCoupleStore(c.srv.DB).Load(context.Background()); err != nil || len(stored) != 0 {
		t.Fatalf("mods_wedding after the divorce = %+v, %v; want none", stored, err)
	}
	c.assertPage(t, "divorced greeting", c.talk(t, c.alice), c.page(t, "start.htm", "1,000,000", "won't"))
}

// TestWeddingDeclined pins the refusal of a request: each side is told,
// nothing is paid, and both are free to ask again.
func TestWeddingDeclined(t *testing.T) {
	c := bootChapel(t, defaultChapel())
	c.talk(t, c.alice)
	c.bypass(t, c.alice, "AskWedding bobby")
	drainFrames(t, c.bobby)

	answer(t, c.bobby, false)
	if got := texts(t, drainFrames(t, c.bobby)); !slices.Equal(got, []string{"You declined your partner's marriage request."}) {
		t.Fatalf("Bobby after declining = %q", got)
	}
	if got := texts(t, drainFrames(t, c.alice)); !slices.Equal(got, []string{"Your partner declined your marriage request."}) {
		t.Fatalf("Alice after the decline = %q", got)
	}
	if c.srv.Couples.CoupleID(c.aliceID) != 0 || c.adena(t, c.aliceID) != 2_000_000 || c.adena(t, c.bobbyID) != 2_000_000 {
		t.Fatal("a declined request married or charged")
	}
	c.assertPage(t, "Alice after the decline", c.talk(t, c.alice), c.page(t, "start.htm", "1,000,000", "won't"))

	// The request is spent: answering it again does nothing.
	drainFrames(t, c.bobby)
	answer(t, c.bobby, true)
	if frames := drainFrames(t, c.bobby); len(frames) != 0 {
		t.Fatalf("a second answer = %x, want nothing", opcodes(frames))
	}
	if c.srv.Couples.CoupleID(c.aliceID) != 0 {
		t.Fatal("a spent request married")
	}
}

// TestWeddingRequestRefusals pins weddingConditions' order and pages: an
// unknown or offline name, oneself, the same sex, a stranger, a partner
// already married, missing formal wear and a short purse each open their
// page and leave nobody under a request.
func TestWeddingRequestRefusals(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    func(*chapelOptions)
		command string
		page    string
		price   string
	}{
		{name: "no name", command: "AskWedding", page: "notfound.htm"},
		{name: "offline", command: "AskWedding Nobody", page: "notfound.htm"},
		{name: "oneself", command: "AskWedding Alice", page: "error_wrongtarget.htm"},
		{name: "same sex", opts: func(o *chapelOptions) { o.bobbySex = player.SexMale }, command: "AskWedding Bobby", page: "error_sex.htm"},
		{name: "stranger", opts: func(o *chapelOptions) { o.friends = false }, command: "AskWedding Bobby", page: "error_friendlist.htm"},
		{name: "no formal wear", opts: func(o *chapelOptions) { o.cfg.FormalWear = true }, command: "AskWedding Bobby", page: "error_noformal.htm"},
		{name: "partner short", opts: func(o *chapelOptions) { o.bobbyAdena = 999_999 }, command: "AskWedding Bobby", page: "error_adena.htm", price: "1,000,000"},
		{name: "requester short", opts: func(o *chapelOptions) { o.aliceAdena = 0 }, command: "AskWedding Bobby", page: "error_adena.htm", price: "1,000,000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := defaultChapel()
			if tc.opts != nil {
				tc.opts(&opts)
			}
			c := bootChapel(t, opts)
			c.talk(t, c.alice)
			frames := c.bypass(t, c.alice, tc.command)
			if got := opcodes(frames); string(got) != string(refusalOrder) {
				t.Fatalf("refusal = %x, want %x", got, refusalOrder)
			}
			c.assertPage(t, tc.name, frames, c.page(t, tc.page, tc.price, ""))
			if frames := only(drainFrames(t, c.bobby), serverpackets.OpcodeConfirmDlg); len(frames) != 0 {
				t.Fatal("a refused request reached Bobby")
			}
			if c.character(t, c.aliceID).UnderMarryRequest() || c.character(t, c.bobbyID).UnderMarryRequest() {
				t.Fatal("a refused request left a player under it")
			}
		})
	}
}

// TestWeddingFormalWearAndSameSexAllowed pins the two settings that lift a
// refusal: formal wear on both passes the formal wear check, and
// WeddingAllowSameSex lets two men marry.
func TestWeddingFormalWearAndSameSexAllowed(t *testing.T) {
	opts := defaultChapel()
	opts.cfg.FormalWear, opts.cfg.SameSex = true, true
	opts.bobbySex = player.SexMale
	opts.formalOnBoth = true
	c := bootChapel(t, opts)
	if !c.character(t, c.aliceID).WearingFormalWear() || !c.character(t, c.bobbyID).WearingFormalWear() {
		t.Fatal("the fixture spouses do not wear their formal wear")
	}
	c.assertPage(t, "request form", c.talk(t, c.alice), c.page(t, "start.htm", "1,000,000", "will"))
	c.bypass(t, c.alice, "AskWedding Bobby")
	answer(t, c.bobby, true)
	drainFrames(t, c.bobby)
	if c.srv.Couples.CoupleID(c.aliceID) == 0 {
		t.Fatal("two formally dressed men were not married with same-sex marriage allowed")
	}
}

// TestWeddingAlreadyMarriedPartner pins the refusal of a married partner.
func TestWeddingAlreadyMarriedPartner(t *testing.T) {
	c := bootChapel(t, defaultChapel())
	c.talk(t, c.alice)
	c.bypass(t, c.alice, "AskWedding Bobby")
	answer(t, c.bobby, true)
	drainFrames(t, c.alice)
	drainFrames(t, c.bobby)

	third := c.srv.SeedCharacterFor(t, "player3", "Carol", 20, 0).ID
	c.srv.GiveItem(t, third, 57, 2_000_000)
	carol := c.srv.DialClient(t, "player3", 1)
	enter(t, c.srv, carol)
	c.srv.Relations.AddFriend(third, c.bobbyID)
	drainFrames(t, c.alice)
	drainFrames(t, c.bobby)
	c.talk(t, carol)
	frames := c.bypass(t, carol, "AskWedding Bobby")
	c.assertPage(t, "married partner", frames, c.page(t, "error_alreadymarried.htm", "", ""))
	if c.character(t, c.bobbyID).UnderMarryRequest() {
		t.Fatal("a refused request left the married partner under it")
	}
}

// TestWeddingAcceptRechecksSpouses pins the two checks the accept repeats,
// so that a request cannot marry someone already married since, nor marry
// a spouse who spent the price meanwhile: nobody is married or charged,
// the spouse short of the price is told so, and both leave the request.
func TestWeddingAcceptRechecksSpouses(t *testing.T) {
	t.Run("price spent meanwhile", func(t *testing.T) {
		c := bootChapel(t, defaultChapel())
		c.talk(t, c.alice)
		c.bypass(t, c.alice, "AskWedding Bobby")
		drainFrames(t, c.bobby)
		inv := c.srv.PlayerInventory(t, c.aliceID)
		if inv.DestroyByTemplateID(57, 1_500_000) == nil {
			t.Fatal("could not spend Alice's adena")
		}
		drainFrames(t, c.alice)

		answer(t, c.bobby, true)
		if got := systemMessageIDs(drainFrames(t, c.alice)); !slices.Equal(got, []int32{serverpackets.SystemMessageYouNotEnoughAdena}) {
			t.Fatalf("Alice after the accept = %v, want not enough adena", got)
		}
		if got := drainFrames(t, c.bobby); len(only(got, serverpackets.OpcodeSystemMessage)) != 0 {
			t.Fatalf("Bobby after the accept = %x, want no message", opcodes(got))
		}
		if c.srv.Couples.CoupleID(c.aliceID) != 0 || c.adena(t, c.aliceID) != 500_000 || c.adena(t, c.bobbyID) != 2_000_000 {
			t.Fatal("an unpaid wedding married or charged")
		}
		if c.character(t, c.aliceID).UnderMarryRequest() || c.character(t, c.bobbyID).UnderMarryRequest() {
			t.Fatal("an unpaid wedding left a player under the request")
		}
	})
	t.Run("forged answer", func(t *testing.T) {
		c := bootChapel(t, defaultChapel())
		answer(t, c.bobby, true)
		if frames := drainFrames(t, c.bobby); len(frames) != 0 {
			t.Fatalf("an answer with no request = %x, want nothing", opcodes(frames))
		}
		// Asking puts the requester under a request it was not asked to
		// answer: its own answer does nothing either.
		c.talk(t, c.alice)
		c.bypass(t, c.alice, "AskWedding Bobby")
		answer(t, c.alice, true)
		if frames := drainFrames(t, c.alice); len(frames) != 0 {
			t.Fatalf("the requester's own answer = %x, want nothing", opcodes(frames))
		}
		if c.srv.Couples.CoupleID(c.aliceID) != 0 {
			t.Fatal("the requester married itself to its pending partner")
		}
	})
	t.Run("married meanwhile", func(t *testing.T) {
		c := bootChapel(t, defaultChapel())
		third := c.srv.SeedCharacterFor(t, "player3", "Carol", 20, 0).ID
		carol := c.srv.DialClient(t, "player3", 1)
		if _, err := c.srv.DB.ExecContext(context.Background(), "UPDATE characters SET sex = 1 WHERE obj_Id = ?", third); err != nil {
			t.Fatal(err)
		}
		c.srv.GiveItem(t, third, 57, 2_000_000)
		enter(t, c.srv, carol)
		c.srv.Relations.AddFriend(c.aliceID, third)
		drainFrames(t, c.alice)
		drainFrames(t, c.bobby)

		// Alice asks Bobby, then, from the request form still open, Carol.
		c.talk(t, c.alice)
		c.bypass(t, c.alice, "AskWedding Bobby")
		c.bypass(t, c.alice, "AskWedding Carol")
		answer(t, carol, true)
		drainFrames(t, carol)
		drainFrames(t, c.alice)
		drainFrames(t, c.bobby)
		if c.srv.Couples.PartnerID(c.aliceID) != third {
			t.Fatal("Alice did not marry Carol")
		}

		answer(t, c.bobby, true)
		drainFrames(t, c.alice)
		drainFrames(t, c.bobby)
		if c.srv.Couples.PartnerID(c.aliceID) != third || c.srv.Couples.CoupleID(c.bobbyID) != 0 {
			t.Fatal("a stale request married an already married requester")
		}
		if c.adena(t, c.bobbyID) != 2_000_000 {
			t.Fatal("a stale request charged its partner")
		}
		if c.character(t, c.bobbyID).UnderMarryRequest() {
			t.Fatal("a stale request left its partner under it")
		}
	})
}

// TestWeddingGoToLove pins the teleport to the spouse from the married
// menu: refused while the spouse is jailed, otherwise a teleport next to
// the spouse, and refused once the couple is gone.
func TestWeddingGoToLove(t *testing.T) {
	c := bootChapel(t, defaultChapel())
	c.talk(t, c.alice)
	c.bypass(t, c.alice, "AskWedding Bobby")
	answer(t, c.bobby, true)
	drainFrames(t, c.alice)
	drainFrames(t, c.bobby)
	c.assertPage(t, "married menu", c.talk(t, c.alice), c.page(t, "start2.htm", "", ""))

	c.character(t, c.bobbyID).SetPunishment(player.PunishJail, 0)
	frames := c.bypass(t, c.alice, "GoToLove")
	if got := texts(t, frames); !slices.Equal(got, []string{"Due to the current partner's status, the teleportation failed."}) {
		t.Fatalf("GoToLove to a jailed spouse = %q", got)
	}
	if got := only(frames, serverpackets.OpcodeTeleportToLocation); len(got) != 0 {
		t.Fatal("a refused GoToLove teleported")
	}
	c.character(t, c.bobbyID).SetPunishment(player.PunishNone, 0)
	drainFrames(t, c.bobby)

	frames = c.bypass(t, c.alice, "GoToLove")
	teleports := only(frames, serverpackets.OpcodeTeleportToLocation)
	if len(teleports) != 1 {
		t.Fatalf("GoToLove = %x, want one TeleportToLocation", opcodes(frames))
	}
	r := wire.NewReader(teleports[0][1:])
	mover, x, y := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	bx, by, _ := c.srv.PlayerPosition(t, c.bobbyID)
	if mover != c.aliceID || abs(int(x)-bx) > 20 || abs(int(y)-by) > 20 {
		t.Fatalf("TeleportToLocation = %d to (%d, %d), want Alice within 20 of Bobby at (%d, %d)", mover, x, y, bx, by)
	}
	drainFrames(t, c.bobby)

	// Bobby divorces while Alice keeps the married menu open.
	c.talk(t, c.bobby)
	c.bypass(t, c.bobby, "Divorce")
	drainFrames(t, c.alice)
	frames = c.bypass(t, c.alice, "GoToLove")
	if got := texts(t, frames); !slices.Equal(got, []string{"Your partner can't be found."}) {
		t.Fatalf("GoToLove once divorced = %q", got)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
