package clan

import (
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: RainbowSpringsChateau.onAdvEvent (portToArena, register,
// cancel). The caretaker's refusals game_manager006 to 012, 014 and 015 and
// the messenger's yetti008, 011, 012, 014 and 017. game_manager012 (the
// clan has fewer than five members while five of them stand in the party)
// cannot be reached: a party of five clan members is five members.

// joinParty has from invite the character toName, which to, its client,
// accepts.
func (w *rainbowWorld) joinParty(t *testing.T, from, to *testsupport.ScriptedClient, toName string) {
	t.Helper()
	req := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	req.WriteString(toName)
	req.WriteInt32(0)
	from.Send(req.Bytes())
	w.srv.Settle(t)
	answer := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	answer.WriteInt32(1)
	to.Send(answer.Bytes())
	w.srv.Settle(t)
	drainFrames(t, from)
	drainFrames(t, to)
	drainFrames(t, w.c)
}

// TestRainbowSpringsCaretakerRefusals has the Attacker, the leader of
// Springers, ask the caretaker to port its party to the arena with a party
// that does not qualify, and checks the page each refusal shows: it has no
// clan (009), it does not lead the party (006), a member of another clan stands in it (007),
// it is under five (008), Springers owns another hall (010; the registrations' close frees the siege's own), is under the hall's
// minimum clan level (011), is unregistered (014) or registered without
// having been chosen (015). None moves anybody.
func TestRainbowSpringsCaretakerRefusals(t *testing.T) {
	t.Parallel()
	fifthLeader := rainbowMember{castleRival: castleRival{0x7f001011, "fifth", "FifthLeader", fifthClanID}, leads: true}
	for _, tt := range []struct {
		name  string
		setup rainbowSetup
		// partied are the members of Springers the Attacker invites, foreign
		// the clan member of another clan it invites, leads that the first
		// member invites the Attacker instead.
		partied int
		foreign bool
		leads   bool
		page    string
	}{
		{"no clan", rainbowSetup{seed: []string{"UPDATE characters SET clanid = 0 WHERE char_name = 'Attacker'"}}, 0, false, false, "game_manager009.htm"},
		{"not the party leader", rainbowSetup{}, 1, false, true, "game_manager006.htm"},
		{"a member of another clan", rainbowSetup{}, 1, true, false, "game_manager007.htm"},
		{"party under five", rainbowSetup{}, 1, false, false, "game_manager008.htm"},
		{"clan owns another hall", rainbowSetup{seed: []string{fmt.Sprintf("INSERT INTO clanhall (id, ownerId, paidUntil, paid, endDate) VALUES (22, %d, %d, 1, 0)", springersClanID, hallPaidUntil)}}, 4, false, false, "game_manager010.htm"},
		{"clan under the minimum level", rainbowSetup{seed: []string{fmt.Sprintf("UPDATE clan_data SET clan_level = 3 WHERE clan_id = %d", springersClanID)}}, 4, false, false, "game_manager011.htm"},
		{"clan not registered", rainbowSetup{}, 4, false, false, "game_manager014.htm"},
		{"clan registered but not chosen", rainbowSetup{decrees: map[int32]int{springersClanID: 4}}, 4, false, false, "game_manager015.htm"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			members := springersMembers()
			tt.setup.date = 2 * time.Hour
			// Four rivals out-register Springers, so that, once the siege
			// date chose the clans, it is not among them but for the owner.
			decrees := map[int32]int{bathersClanID: 9, soakersClanID: 8, waders4ClanID: 7, fifthClanID: 6}
			maps.Copy(decrees, tt.setup.decrees)
			tt.setup.decrees = decrees
			tt.setup.members = append(append([]rainbowMember(nil), members...), fifthLeader)
			w := bootRainbow(t, tt.setup)
			var clients []*testsupport.ScriptedClient
			for _, m := range members[:tt.partied] {
				clients = append(clients, w.member(t, m))
			}
			w.srv.Advance(t, 2*time.Hour)
			w.srv.Advance(t, 0)
			if got := w.siege.Attackers(); len(got) == 0 {
				t.Fatal("no clan was chosen at the siege date")
			}
			drainFrames(t, w.c)
			for i, c := range clients {
				drainFrames(t, c)
				if tt.leads {
					w.joinParty(t, c, w.c, "Attacker")
					continue
				}
				w.joinParty(t, w.c, c, members[i].name)
			}
			if tt.foreign {
				w.joinParty(t, w.c, w.member(t, fifthLeader), fifthLeader.name)
			}
			w.talk(t, w.c, w.npcOf(t, rainbowCaretaker))
			wantRainbowPage(t, tt.name, w.link(t, w.c, "portToArena"), tt.page)
			if x, y, _ := w.srv.PlayerPosition(t, w.objID); abs(x-rainbowPost.X) > 50 || abs(y-rainbowPost.Y) > 50 {
				t.Fatalf("the Attacker was moved to %d,%d by a refused entry", x, y)
			}
		})
	}
}

// TestRainbowSpringsMessengerRefusals has the Attacker, the leader of
// Springers, ask the messenger to register it and to cancel its
// registration, and checks the refusals: a clan under five members
// (messenger_yetti011), a clan owning the hall (012), holding no war decree
// (008), registering once the registrations closed (014) and cancelling
// once they did (017). Nothing is stored or given back.
func TestRainbowSpringsMessengerRefusals(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		setup rainbowSetup
		// closed runs the registrations out before the click.
		closed bool
		event  string
		page   string
		stored map[int32]int
	}{
		{"register with a clan under five members", rainbowSetup{heldDecrees: 7}, false, "register", "messenger_yetti011.htm", nil},
		{"register while owning the hall", rainbowSetup{owner: springersClanID, heldDecrees: 7, members: springersMembers()}, false, "register", "messenger_yetti012.htm", nil},
		{"register with no decree", rainbowSetup{members: springersMembers()}, false, "register", "messenger_yetti008.htm", nil},
		{"register once the registrations closed", rainbowSetup{heldDecrees: 7, members: springersMembers()}, true, "register", "messenger_yetti014.htm", nil},
		{"cancel once the registrations closed", rainbowSetup{decrees: map[int32]int{springersClanID: 4}, members: springersMembers()}, true, "cancel", "messenger_yetti017.htm", map[int32]int{springersClanID: 4}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.setup.date = 2 * time.Hour
			w := bootRainbow(t, tt.setup)
			if tt.closed {
				w.srv.Advance(t, time.Hour)
				drainFrames(t, w.c)
			}
			w.talk(t, w.c, w.npcOf(t, rainbowMessenger))
			if tt.event == "cancel" {
				w.link(t, w.c, "messenger_yetti015.htm")
			}
			wantRainbowPage(t, tt.name, w.link(t, w.c, tt.event), tt.page)
			if got := w.storedDecrees(t); !sameDecrees(got, tt.stored) {
				t.Fatalf("stored decrees = %v, want %v", got, tt.stored)
			}
			if n := w.heldDecrees(t); n != tt.setup.heldDecrees {
				t.Fatalf("decrees held = %d, want %d", n, tt.setup.heldDecrees)
			}
		})
	}
}
