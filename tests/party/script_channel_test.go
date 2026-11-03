package party

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The command channel reads and group screen messages of the script-facing
// API freeze (#3808), as the Frintezza teleporter, Freya's deacon and Ice
// Fairy Sirra call them: the channel of the player's party, its parties and
// members and leader, and ExShowScreenMessage(text, time) to every member
// of a party or of a channel.

// Seats of the suite: Leader with Member and Other with Fourth form two
// parties joined in one channel Leader leads; Loner is in no party.
const (
	scLeader = iota
	scMember
	scOther
	scFourth
	scLoner
)

// bootScriptChannel boots the five seats with a Director script registered
// and Leader the leader of a level 5 clan holding a Strategy Guide, forms
// the two parties, and, when channel is set, joins them in one channel.
func bootScriptChannel(t *testing.T, channel bool) *group {
	t.Helper()
	seats := []seat{{"Leader", 40}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}, {"Loner", 20}}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter(seats[0].name, seats[0].level, 0),
		gameservertest.WithWantChars(1),
		strategyGuideTemplates(),
		authorityClan{level: 5, leads: true}.option(t),
		gameservertest.WithScripts([]script.Listing{{Path: "quest.Director"}}, script.Catalog{
			"quest.Director": func() script.Script { return script.Script{} },
		}),
	)
	leaderID := srv.SoleObjectID(t)
	srv.GiveItem(t, leaderID, strategyGuideID, 1)
	g := &group{srv: srv, players: []player{{c: srv.Client, id: leaderID, name: seats[0].name}}}
	for i, s := range seats[1:] {
		account := "member" + string(rune('a'+i))
		id := srv.SeedCharacterFor(t, account, s.name, s.level, 0).ID
		g.players = append(g.players, player{c: srv.DialClient(t, account, 1), id: id, name: s.name})
	}
	for _, p := range g.players {
		startInWorld(t, p.c)
	}
	g.quiet(t)
	g.invite(t, scLeader, scMember, 0)
	g.invite(t, scOther, scFourth, 0)
	if channel {
		g.askAndAccept(t, scLeader, scOther)
	}
	return g
}

// runDirector runs fn as the Director script for seat i's player.
func (g *group) runDirector(t *testing.T, i int, fn func(p *script.Player)) {
	t.Helper()
	if !g.srv.RunScript(t, g.players[i].id, "Director", func(_ *script.Script, p *script.Player) { fn(p) }) {
		t.Fatal("the script panicked")
	}
}

func playerNames(players []*script.Player) []string {
	if players == nil {
		return nil
	}
	out := make([]string, len(players))
	for i, p := range players {
		out[i] = p.Name()
	}
	return out
}

// TestScriptReadsCommandChannel: every member of either party reads the
// same channel, its parties in the order they joined, each leader first,
// its members party by party, and its leader. A party in no channel and a
// player in no party read none.
func TestScriptReadsCommandChannel(t *testing.T) {
	t.Parallel()
	g := bootScriptChannel(t, true)

	wantParties := [][]string{{"Leader", "Member"}, {"Other", "Fourth"}}
	for _, i := range []int{scLeader, scMember, scOther, scFourth} {
		g.runDirector(t, i, func(p *script.Player) {
			parties := p.ChannelParties()
			got := make([][]string, len(parties))
			for j, party := range parties {
				got[j] = playerNames(party)
			}
			if !slices.EqualFunc(got, wantParties, slices.Equal) {
				t.Errorf("%s: ChannelParties() = %v, want %v", g.players[i].name, got, wantParties)
			}
			if got, want := playerNames(p.ChannelMembers()), []string{"Leader", "Member", "Other", "Fourth"}; !slices.Equal(got, want) {
				t.Errorf("%s: ChannelMembers() = %v, want %v", g.players[i].name, got, want)
			}
			if leader := p.ChannelLeader(); leader == nil || leader.ObjectID() != g.players[scLeader].id {
				t.Errorf("%s: ChannelLeader() = %v, want Leader", g.players[i].name, leader)
			}
		})
	}

	g.runDirector(t, scLoner, func(p *script.Player) {
		if p.ChannelParties() != nil || p.ChannelMembers() != nil || p.ChannelLeader() != nil {
			t.Errorf("Loner: channel %v %v %v, want none", p.ChannelParties(), p.ChannelMembers(), p.ChannelLeader())
		}
	})
	g.quiet(t)
	for _, p := range g.players {
		assertSilent(t, p.c, p.name+" after the reads")
	}
}

// TestScriptPartyInNoChannel: a party outside any channel reads none, and
// its member's channel message reaches no one.
func TestScriptPartyInNoChannel(t *testing.T) {
	t.Parallel()
	g := bootScriptChannel(t, false)
	g.runDirector(t, scMember, func(p *script.Player) {
		if p.ChannelParties() != nil || p.ChannelMembers() != nil || p.ChannelLeader() != nil {
			t.Errorf("Member: channel %v %v %v, want none", p.ChannelParties(), p.ChannelMembers(), p.ChannelLeader())
		}
		p.ShowChannelScreenMessage(10000, "nobody reads this")
	})
	for _, p := range g.players {
		assertSilent(t, p.c, p.name+" after a channel message of no channel")
	}
}

// screenFrames returns the frames among frames that are not the party's
// periodic position reports.
func screenFrames(frames [][]byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] != serverpackets.OpcodePartyMemberPosition {
			out = append(out, f)
		}
	}
	return out
}

// requireScreenMessage asserts frames hold exactly one frame, an
// ExShowScreenMessage of text for durationMs at the top center: type 1, no
// system message, position 2, no hide, normal size, no effect, no fade.
func requireScreenMessage(t *testing.T, who string, frames [][]byte, text string, durationMs int32) []byte {
	t.Helper()
	frames = screenFrames(frames)
	if len(frames) != 1 {
		t.Fatalf("%s: %d frames %x, want one ExShowScreenMessage", who, len(frames), opcodes(frames))
	}
	f := frames[0]
	if f[0] != serverpackets.OpcodeExtended || binary.LittleEndian.Uint16(f[1:3]) != 0x38 {
		t.Fatalf("%s: frame % x, want ExShowScreenMessage", who, f)
	}
	r := wire.NewReader(f[3:])
	var fields [10]int32
	for i := range fields {
		fields[i] = r.ReadInt32()
	}
	if want := [10]int32{1, -1, 2, 0, 0, 0, 0, 0, durationMs, 0}; fields != want {
		t.Fatalf("%s: screen message fields %v, want %v", who, fields, want)
	}
	if got := r.ReadString(); got != text {
		t.Fatalf("%s: screen message text %q, want %q", who, got, text)
	}
	return f
}

// TestScriptGroupScreenMessages: a party message reaches the speaker's
// party alone, the speaker included; a channel message reaches every
// member of both parties with identical bytes, each its own copy; a player
// in no party shows either message to no one.
func TestScriptGroupScreenMessages(t *testing.T) {
	t.Parallel()
	g := bootScriptChannel(t, true)
	const partyText, channelText = "The tomb seals in 20 minutes.", "The tomb seals in 35 minutes."

	g.runDirector(t, scFourth, func(p *script.Player) { p.ShowPartyScreenMessage(10000, partyText) })
	for _, i := range []int{scOther, scFourth} {
		requireScreenMessage(t, g.players[i].name, drainFrames(t, g.players[i].c), partyText, 10000)
	}
	for _, i := range []int{scLeader, scMember, scLoner} {
		assertSilent(t, g.players[i].c, g.players[i].name+" outside the party")
	}

	g.runDirector(t, scMember, func(p *script.Player) { p.ShowChannelScreenMessage(100000, channelText) })
	var got [][]byte
	for _, i := range []int{scLeader, scMember, scOther, scFourth} {
		got = append(got, requireScreenMessage(t, g.players[i].name, drainFrames(t, g.players[i].c), channelText, 100000))
	}
	assertSilent(t, g.players[scLoner].c, "Loner outside the channel")
	want := slices.Clone(got[0])
	got[0][len(got[0])-3] ^= 0xff
	for i, f := range got[1:] {
		if !slices.Equal(f, want) {
			t.Fatalf("%s's channel message % x, want % x", g.players[i+1].name, f, want)
		}
	}

	g.runDirector(t, scLoner, func(p *script.Player) {
		p.ShowPartyScreenMessage(10000, partyText)
		p.ShowChannelScreenMessage(10000, channelText)
	})
	for _, p := range g.players {
		assertSilent(t, p.c, p.name+" after a groupless message")
	}
}
