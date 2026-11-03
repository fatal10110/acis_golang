package npcs

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: ClanHallDoorman.onBypassFeedback (ClanHallDoorman.java:30-103)
// and its door overrides (148-187). wyvern_info and wyvern_help open
// clanHallDoormen/wyvernmanager-1.htm and -7.htm, then ActionFailed, for a
// member of the clan owning an Aden hall (36-41), and answer nothing
// elsewhere; open_doors and close_doors (Doorman.java:27-56) open or close
// every gate of the hall for any member of the owning clan, then show
// doormen-opened.htm or doormen-closed.htm alone, and answer nothing to
// anyone else; Chat <n> reopens the doorman's first page
// (ClanHallDoorman.java:142-146).

func hallDoormanCommandPages() gameservertest.Option {
	pages := map[string]string{}
	for _, file := range []string{
		"clanHallDoormen/doormen.htm", "clanHallDoormen/doormen_wyvern.htm", "clanHallDoormen/doormen-opened.htm", "clanHallDoormen/doormen-closed.htm",
		"clanHallDoormen/wyvernmanager-1.htm", "clanHallDoormen/wyvernmanager-7.htm",
	} {
		pages[file] = commandPage(file)
	}
	return gameservertest.WithHTMLPages(pages)
}

// TestClanHallDoormanDoors pins the doorman's gates: any member of the clan
// owning the hall opens then closes both, each followed by its page alone;
// a stranger is answered nothing and the gates stay shut.
func TestClanHallDoormanDoors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		setup hallManagerSetup
	}{
		{"leader", hallManagerSetup{}},
		{"member without rights", hallManagerSetup{rank: 5}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.setup.extra = []gameservertest.Option{moonstoneGates(), hallDoormanCommandPages()}
			w := bootHallManager(t, c.setup)
			f := w.spawnKeeper(t, "ClanHallDoorman", hallDoormanID, 40)
			for _, d := range []struct {
				command, page string
				open          bool
			}{{"open_doors", "clanHallDoormen/doormen-opened.htm", true}, {"close_doors", "clanHallDoormen/doormen-closed.htm", false}} {
				frames := w.command(t, f, d.command)
				ids, open, last := gateStates(frames)
				if !slices.Equal(ids, []int32{moonstoneGateA, moonstoneGateB}) || open[0] != d.open || open[1] != d.open {
					t.Fatalf("%s: gate updates %v open %v", d.command, ids, open)
				}
				rest := frames[last+1:]
				assertAnswer(t, rest, pageThenFinal, f, filled(d.page, f))
			}
		})
	}
	t.Run("stranger", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{foreign: true, extra: []gameservertest.Option{moonstoneGates(), hallDoormanCommandPages()}})
		f := w.spawnKeeper(t, "ClanHallDoorman", hallDoormanID, 40)
		assertGates(t, w.command(t, f, "open_doors"), nil, true, true)
	})
}

// TestClanHallDoormanWyvernPages pins the wyvern pages at an Aden hall for a
// member of the owning clan, each naming the ride's settings, then
// ActionFailed; a stranger, and an owner at a hall offering no wyverns, are
// answered nothing, the ride included. TestClanHallDoormanWyvernRide covers
// the ride.
func TestClanHallDoormanWyvernPages(t *testing.T) {
	t.Parallel()
	release := []byte{serverpackets.OpcodeActionFailed}
	t.Run("aden owner", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{hall: goldenChamber, manager: goldenManagerID, extra: []gameservertest.Option{hallDoormanCommandPages()}})
		f := w.spawnKeeper(t, "ClanHallDoorman", goldenDoormanID, 40)
		assertAnswer(t, w.command(t, f, "wyvern_info"), chatThenFinal, f, filled("clanHallDoormen/wyvernmanager-1.htm", f, wyvernPairs...))
		assertAnswer(t, w.command(t, f, "wyvern_help"), chatThenFinal, f, filled("clanHallDoormen/wyvernmanager-7.htm", f, wyvernPairs...))
		assertAnswer(t, w.command(t, f, "Chat 3"), releasedChat, f, filled("clanHallDoormen/doormen_wyvern.htm", f))
	})
	t.Run("aden stranger", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{hall: goldenChamber, manager: goldenManagerID, foreign: true, extra: []gameservertest.Option{hallDoormanCommandPages()}})
		f := w.spawnKeeper(t, "ClanHallDoorman", goldenDoormanID, 40)
		assertAnswer(t, w.command(t, f, "wyvern_info"), release, f, "")
		assertAnswer(t, w.command(t, f, "wyvern_ride"), release, f, "")
	})
	t.Run("owner elsewhere", func(t *testing.T) {
		t.Parallel()
		w := bootHallManager(t, hallManagerSetup{extra: []gameservertest.Option{hallDoormanCommandPages()}})
		f := w.spawnKeeper(t, "ClanHallDoorman", hallDoormanID, 40)
		assertAnswer(t, w.command(t, f, "wyvern_info"), release, f, "")
		assertAnswer(t, w.command(t, f, "wyvern_ride"), release, f, "")
	})
}
