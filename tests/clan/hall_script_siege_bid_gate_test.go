package clan

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// bidOnMoonstone seeds clanID's standing bid on free Moonstone Hall's
// auction, which ends a week from now.
func bidOnMoonstone(clanID int32) []string {
	id := itoa(clanID)
	return []string{
		"INSERT INTO clanhall (id, endDate) VALUES (22, " + weekAhead() + ")",
		"INSERT INTO auctions (clanhall_id, bidder_name, clan_oid, clan_name, max_bid, time_bid) VALUES (22, 'Bidder', " + id + ", 'Bidder', 21000000, 1)",
		"UPDATE clan_data SET auction_bid_at = 22 WHERE clan_id = " + id,
	}
}

// TestBanditStrongholdBiddingClanMayNotRegister: Besiegers bids on
// Moonstone Hall and holds the trophy that registers it for the Bandit
// Stronghold's flag war. The messenger refuses it with the page for a clan
// owning a hall: it could win both halls (owner-approved divergence,
// #5110). Nothing is taken, the quest stays, nothing is registered, and the
// siege itself refuses the bidder as an attacker.
func TestBanditStrongholdBiddingClanMayNotRegister(t *testing.T) {
	t.Parallel()
	items := map[int32]int32{trophyID: 2, tarlkAmuletID: 31, contestCertificateID: 1}
	w := bootFlagWar(t, flagWarSetup{q504: true, items: items, seed: bidOnMoonstone(besiegersClanID)})
	messenger := w.messenger(t)
	w.srv.Advance(t, time.Duration(w.date-w.srv.Halls.Now()-3_600_000)*time.Millisecond)
	if !w.siege.WaitingBattle() {
		t.Fatal("the registrations did not close")
	}
	drainFrames(t, w.c)

	w.talk(t, messenger)
	wantPage(t, "register after the close", w.bypass(t, "register"), "agit_oel_mahum_messeger_1.htm", messenger.ObjectID())
	wantPage(t, "the bidder's registration", w.bypass(t, "register_clan"), "azit_messenger_q0504_10.htm", messenger.ObjectID())
	w.srv.RunScript(t, w.objID, banditScriptName, func(_ *script.Script, p *script.Player) {
		for id, want := range items {
			if n := p.ItemCount(id); n != want {
				t.Errorf("item %d after the refusal = %d, want %d", id, n, want)
			}
		}
	})
	if got := w.rows(t, "SELECT COUNT(*) FROM character_quests WHERE name = '"+q504Name+"'"); slices.Equal(got, []string{"0"}) {
		t.Fatal("the refusal ended Q504")
	}
	if got := w.flagWarClans(t); len(got) != 0 {
		t.Fatalf("flag war clans = %v, want none", got)
	}
	if w.siege.AddAttacker(besiegersClanID) {
		t.Fatal("the siege took the bidder as an attacker")
	}
	if got := w.siege.Attackers(); len(got) != 0 {
		t.Fatalf("attackers = %#x, want none", got)
	}
}

// TestRainbowSpringsBiddingClanMayNotRegister: Springers bids on Moonstone
// Hall, so the messenger refuses its registration with the page for a clan
// owning a hall and keeps no decree (owner-approved divergence, #5110).
func TestRainbowSpringsBiddingClanMayNotRegister(t *testing.T) {
	t.Parallel()
	members := springersMembers()
	w := bootRainbow(t, rainbowSetup{date: 5 * time.Hour, heldDecrees: 7, members: members, seed: bidOnMoonstone(springersClanID)})
	messenger := w.npcOf(t, rainbowMessenger)
	w.talk(t, w.c, messenger)
	wantRainbowPage(t, "the bidder's registration", w.link(t, w.c, "register"), "messenger_yetti012.htm")
	if got := w.storedDecrees(t); len(got) != 0 {
		t.Fatalf("stored decrees = %v, want none", got)
	}
	if n := w.heldDecrees(t); n != 7 {
		t.Fatalf("decrees held = %d, want 7", n)
	}
}

// TestRainbowSpringsDropsAClanThatBidSinceItRegistered: Springers (4
// decrees) and Bathers (3) are registered, then Bathers bids on Moonstone
// Hall. At the date Bathers is passed over like a dissolving clan, so
// Springers is alone and the siege is called off (owner-approved
// divergence, #5110).
func TestRainbowSpringsDropsAClanThatBidSinceItRegistered(t *testing.T) {
	t.Parallel()
	w := bootRainbow(t, rainbowSetup{
		date:    2 * time.Hour,
		decrees: map[int32]int{springersClanID: 4, bathersClanID: 3},
		seed:    bidOnMoonstone(bathersClanID),
	})
	w.srv.Advance(t, time.Hour)
	drainFrames(t, w.c)
	w.srv.Advance(t, time.Hour)
	if got := sysMsgIDs(t, drainFrames(t, w.c)); !slices.Equal(got, []string{"1823", "1841", "856 62"}) {
		t.Fatalf("the Attacker got %v, want REGISTRATION_PERIOD_ENDED, CLANHALL_WAR_CANCELLED, SIEGE_S1_DRAW", got)
	}
	if got := w.siege.Attackers(); len(got) != 0 {
		t.Fatalf("attackers = %#x, want none", got)
	}
	if w.siege.Status() != siege.StatusRegistrationOpened {
		t.Fatalf("status = %v, want registration opened", w.siege.Status())
	}
}

// TestWildBeastReserveBiddingClanMayNotRegister: Besiegers bids on
// Moonstone Hall and holds the licenses and crystals that register it for
// the Wild Beast Reserve's flag war. The messenger refuses it with the page
// for a clan owning a hall before taking its license: it could win both
// halls (owner-approved divergence, #5110). Nothing is taken, the quest
// stays, nothing is registered, and the siege itself refuses the bidder as
// an attacker.
func TestWildBeastReserveBiddingClanMayNotRegister(t *testing.T) {
	t.Parallel()
	items := map[int32]int32{trainerLicenseID: 2, crystalOfPurityID: 10}
	w := bootBeastFarm(t, flagWarSetup{items: items, seed: bidOnMoonstone(besiegersClanID)})
	messenger := w.beastMessenger(t)
	w.srv.Advance(t, time.Duration(w.date-w.srv.Halls.Now()-3_600_000)*time.Millisecond)
	if !w.siege.WaitingBattle() {
		t.Fatal("the registrations did not close")
	}
	drainFrames(t, w.c)

	w.talk(t, messenger)
	wantBeastPage(t, "register after the close", w.beastBypass(t, "register"), "farm_kel_mahum_messenger_1.htm", messenger.ObjectID())
	wantBeastPage(t, "the bidder's registration", w.beastBypass(t, "register_clan"), "farm_messenger_q0655_04.htm", messenger.ObjectID())
	w.srv.RunScript(t, w.objID, beastScriptName, func(_ *script.Script, p *script.Player) {
		for id, want := range items {
			if n := p.ItemCount(id); n != want {
				t.Errorf("item %d after the refusal = %d, want %d", id, n, want)
			}
		}
	})
	if got := w.rows(t, "SELECT COUNT(*) FROM character_quests WHERE name = '"+q655Name+"'"); slices.Equal(got, []string{"0"}) {
		t.Fatal("the refusal ended Q655")
	}
	if got := w.flagWarClans(t); len(got) != 0 {
		t.Fatalf("flag war clans = %v, want none", got)
	}
	if w.siege.AddAttacker(besiegersClanID) {
		t.Fatal("the siege took the bidder as an attacker")
	}
	if got := w.siege.Attackers(); len(got) != 0 {
		t.Fatalf("attackers = %#x, want none", got)
	}
}
