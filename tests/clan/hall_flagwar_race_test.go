package clan

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	latecomersClanID = 9100
	latecomerID      = 9101
	purpleFlagID     = 35427
)

// TestBanditStrongholdLastSlotRace: Holders owns the Bandit Stronghold and
// three clans hold flags of its war. After the close, Holders registered,
// one of its five slots is left. The Attacker, leading Besiegers, and
// Rival, leading Latecomers, each with a Trophy of Alliance, ask for it at
// once, each on its own connection, so the two registrations run on two
// queues together. One clan takes it under the purple flag and pays its
// trophy; the other is told the war is full and keeps its trophy: the war
// never counts six attackers nor gives two clans one flag.
func TestBanditStrongholdLastSlotRace(t *testing.T) {
	t.Parallel()
	seed := []string{
		`CREATE TEMPORARY TABLE rival SELECT * FROM characters WHERE char_name = 'Attacker'`,
		`UPDATE rival SET obj_Id = ` + strconv.Itoa(latecomerID) + `, account_name = 'rival', char_name = 'Rival', clanid = ` + strconv.Itoa(latecomersClanID),
		`INSERT INTO characters SELECT * FROM rival`,
		`DROP TEMPORARY TABLE rival`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (` + strconv.Itoa(latecomersClanID) + `, 'Rivals', 5, ` + strconv.Itoa(latecomerID) + `)`,
	}
	for i := range int32(3) {
		id := strconv.Itoa(int(9001 + i))
		seed = append(seed,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level) VALUES (`+id+`, 'Seeded`+id+`', 5)`,
			`INSERT INTO clanhall_flagwar_attackers (clanhall_id, flag, npc, clan_id) VALUES (35, `+strconv.Itoa(int(redFlagID+i))+`, 0, `+id+`)`)
	}
	w := bootFlagWar(t, flagWarSetup{owned: true, seed: seed, items: map[int32]int32{trophyID: 1}})
	for i := range int32(3) {
		// Stored registrations bring back the war's flags, not its
		// attackers.
		if !w.siege.AddAttacker(9001 + i) {
			t.Fatalf("clan %d did not join the attackers", 9001+i)
		}
	}
	w.srv.GiveItem(t, latecomerID, trophyID, 1)
	rival := w.srv.DialClient(t, "rival", 1)
	startInWorld(t, rival)
	messenger := w.messenger(t)
	w.srv.Advance(t, time.Duration(w.date-w.srv.Halls.Now()-3_600_000)*time.Millisecond)
	if !w.siege.WaitingBattle() {
		t.Fatal("the registrations did not close")
	}
	if got := w.siege.Attackers(); len(got) != 4 || !slices.Contains(got, holdersClanID) {
		t.Fatalf("attackers before the race = %v, want the three seeded and Holders", got)
	}

	leaders := []struct {
		name   string
		c      *testsupport.ScriptedClient
		objID  int32
		clanID int32
	}{
		{"the Attacker", w.c, w.objID, besiegersClanID},
		{"Rival", rival, latecomerID, latecomersClanID},
	}
	for _, l := range leaders {
		drainFrames(t, l.c)
		on := &flagWarWorld{srv: w.srv, c: l.c}
		on.talk(t, messenger)
		wantPage(t, l.name+" asks to register", on.bypass(t, "register"), "agit_oel_mahum_messeger_1.htm", messenger.ObjectID())
	}
	for _, l := range leaders {
		l.c.Send(encodeBypass("Quest " + banditScriptName + " register_clan"))
	}
	w.srv.Settle(t)

	full := strings.ReplaceAll(banditPages(t)["script/siegablehall/BanditStronghold/agit_oel_mahum_messeger_21.htm"], "%objectId%", strconv.Itoa(int(messenger.ObjectID())))
	var winner int32
	for _, l := range leaders {
		html := page(t, l.name+" registers", drainFrames(t, l.c))
		trophies := int32(-1)
		w.srv.RunScript(t, l.objID, banditScriptName, func(_ *script.Script, p *script.Player) { trophies = p.ItemCount(trophyID) })
		switch {
		case strings.TrimSpace(html) == strings.TrimSpace(full):
			if trophies != 1 {
				t.Errorf("%s, refused for a full war, holds %d trophies, want 1", l.name, trophies)
			}
		case winner == 0:
			winner = l.clanID
			if trophies != 0 {
				t.Errorf("%s, registered, holds %d trophies, want 0", l.name, trophies)
			}
		default:
			t.Errorf("%s registered too: %q", l.name, html)
		}
	}
	if winner == 0 {
		t.Fatal("neither clan took the last slot")
	}
	if got := w.siege.Attackers(); len(got) != 5 || !slices.Contains(got, winner) {
		t.Fatalf("attackers = %v, want Holders, the three seeded and %d", got, winner)
	}
	want := []string{
		row(35, royalFlagID, 0, int64(holdersClanID)), row(35, redFlagID, 0, 9001), row(35, redFlagID+1, 0, 9002),
		row(35, redFlagID+2, 0, 9003), row(35, purpleFlagID, 0, int64(winner)),
	}
	if got := w.flagWarClans(t); !slices.Equal(got, want) {
		t.Fatalf("flag war clans = %v, want %v", got, want)
	}
}
