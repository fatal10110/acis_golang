package clan

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: BanditStronghold.onAdvEvent / getClanRegisterStatus /
// checkAndRegisterClan and WildBeastReserve's: the flag war messengers'
// refusals.

// inventorySlotsOption sizes the Attacker's inventory: n slots, or the
// shipped ones for 0.
func inventorySlotsOption(n int) gameservertest.Option {
	if n == 0 {
		return gameservertest.WithInventorySlots(player.DefaultInventorySlots.NoDwarf, player.DefaultInventorySlots.Dwarf)
	}
	return gameservertest.WithInventorySlots(n, n)
}

// refusalHall is one flag war hall the refusals run on.
type refusalHall struct {
	name string
	// id is the hall's id and scriptName its script's.
	id         int
	scriptName string
	// pages names a messenger page by its number.
	pages func(n int) string
	// payment is what a registration by trophy takes; extra is another item
	// the Attacker may hold.
	payment, extra int32
	boot           func(t *testing.T, s flagWarSetup) *flagWarWorld
	bypass         func(w *flagWarWorld, t *testing.T, event string) [][]byte
	messenger      func(w *flagWarWorld, t *testing.T) flagWarNPC
	wantPage       func(t *testing.T, what string, frames [][]byte, file string, objectID int32)
}

func refusalHalls() []refusalHall {
	return []refusalHall{{
		name: "Bandit Stronghold", id: int(bandit), scriptName: banditScriptName,
		pages:   func(n int) string { return fmt.Sprintf("agit_oel_mahum_messeger_%d.htm", n) },
		payment: trophyID, extra: tarlkAmuletID,
		boot:      bootFlagWar,
		bypass:    (*flagWarWorld).bypass,
		messenger: (*flagWarWorld).messenger,
		wantPage:  nil,
	}, {
		name: "Wild Beast Reserve", id: beastFarm, scriptName: beastScriptName,
		pages:   func(n int) string { return fmt.Sprintf("farm_kel_mahum_messenger_%d.htm", n) },
		payment: trainerLicenseID, extra: crystalOfPurityID,
		boot:      bootBeastFarm,
		bypass:    (*flagWarWorld).beastBypass,
		messenger: (*flagWarWorld).beastMessenger,
		wantPage:  wantBeastPage,
	}}
}

// closeRegistrations runs the clock to the registrations' close, the
// owner registered.
func closeRegistrations(t *testing.T, w *flagWarWorld) {
	t.Helper()
	w.srv.Advance(t, time.Duration(w.date-w.srv.Halls.Now()-3_600_000)*time.Millisecond)
	if !w.siege.WaitingBattle() {
		t.Fatal("the registrations did not close")
	}
	drainFrames(t, w.c)
}

// expect fails unless frames show page n of the hall's messenger.
func (h refusalHall) expect(t *testing.T, what string, frames [][]byte, n int, messenger flagWarNPC) {
	t.Helper()
	if h.wantPage != nil {
		h.wantPage(t, what, frames, h.pages(n), messenger.ObjectID())
		return
	}
	wantPage(t, what, frames, h.pages(n), messenger.ObjectID())
}

// TestFlagWarMessengerRefusals runs the refusals of both flag war halls'
// messengers: a registration link asked of an overweight player answers
// INVENTORY_LESS_THAN_80_PERCENT and no page; once the registrations closed,
// a clan registers no more when five attack, the hall's owner member is told
// its clan owns the hall (page 22), and a player with no clan that paid for
// a registration is told it cannot (page 3), its payment taken.
func TestFlagWarMessengerRefusals(t *testing.T) {
	t.Parallel()
	for _, h := range refusalHalls() {
		t.Run(h.name+"/overweight", func(t *testing.T) {
			t.Parallel()
			// Three slots hold the adena and the two items: 100% full.
			w := h.boot(t, flagWarSetup{slots: 3, items: map[int32]int32{57: 1, h.payment: 1, h.extra: 1}})
			messenger := h.messenger(w, t)
			w.talk(t, messenger)
			frames := h.bypass(w, t, "register")
			if got := sysMsgParams(t, frames); !slices.Equal(flat(got), []string{"1118"}) {
				t.Fatalf("register while overweight: messages %v, want INVENTORY_LESS_THAN_80_PERCENT", got)
			}
			if _, shown := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); shown {
				t.Fatalf("register while overweight showed a page: opcodes %x", opcodes(frames))
			}
		})
		t.Run(h.name+"/five clans attack", func(t *testing.T) {
			t.Parallel()
			// Holders owns the hall and is registered at the close; four
			// other clans attack beside it: five.
			var seed []string
			var fillers []int32
			for i := range 4 {
				id := holdersClanID + 1 + int32(i)
				fillers = append(fillers, id)
				seed = append(seed, fmt.Sprintf("INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (%d, 'Filler%d', 5)", id, i))
			}
			w := h.boot(t, flagWarSetup{owned: true, seed: seed})
			messenger := h.messenger(w, t)
			closeRegistrations(t, w)
			for _, id := range fillers {
				if !w.siege.AddAttacker(id) {
					t.Fatalf("clan %#x did not join the attackers", id)
				}
			}
			if got := w.siege.Attackers(); len(got) != 5 {
				t.Fatalf("attackers after the close = %#x, want five", got)
			}
			w.talk(t, messenger)
			h.bypass(w, t, "register")
			h.expect(t, "register_clan with five attackers", h.bypass(w, t, "register_clan"), 21, messenger)
			if got := w.flagWarClans(t); len(got) != 1 {
				t.Fatalf("flag war clans after the refusal = %v, want Holders alone", got)
			}
			if got := w.siege.Attackers(); len(got) != 5 || w.siege.Registered(besiegersClanID) {
				t.Fatalf("attackers after the refusal = %#x, want the five, Besiegers not among them", got)
			}
		})
		t.Run(h.name+"/owner member", func(t *testing.T) {
			t.Parallel()
			w := h.boot(t, flagWarSetup{seed: []string{fmt.Sprintf("UPDATE clanhall SET ownerId = %d WHERE id = %d", besiegersClanID, h.id)}})
			messenger := h.messenger(w, t)
			closeRegistrations(t, w)
			w.talk(t, messenger)
			h.bypass(w, t, "register")
			h.expect(t, "register_clan by the owner's clan", h.bypass(w, t, "register_clan"), 22, messenger)
		})
		t.Run(h.name+"/no clan", func(t *testing.T) {
			t.Parallel()
			w := h.boot(t, flagWarSetup{items: map[int32]int32{h.payment: 1}, seed: []string{"UPDATE characters SET clanid = 0 WHERE char_name = 'Attacker'"}})
			messenger := h.messenger(w, t)
			closeRegistrations(t, w)
			w.talk(t, messenger)
			h.bypass(w, t, "register")
			h.expect(t, "register_clan with no clan", h.bypass(w, t, "register_clan"), 3, messenger)
			w.srv.RunScript(t, w.objID, h.scriptName, func(_ *script.Script, p *script.Player) {
				if n := p.ItemCount(h.payment); n != 0 {
					t.Errorf("payment items after the refusal = %d, want it taken", n)
				}
			})
			if got := w.flagWarClans(t); len(got) != 0 {
				t.Fatalf("flag war clans after the refusal = %v, want none", got)
			}
		})
	}
}
