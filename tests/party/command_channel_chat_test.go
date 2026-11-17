package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Chat channels of the command channel, as the client numbers them.
const (
	sayPartyRoomCommander int32 = 15
	sayPartyRoomAll       int32 = 16
)

func encodeSay2(typ int32, text string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteString(text)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeBlockAdd(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBlock)
	w.WriteInt32(clientpackets.BlockAdd)
	w.WriteString(name)
	return w.Bytes()
}

// says returns every chat line c has queued as (speaker, channel, text).
func says(t *testing.T, c *testsupport.ScriptedClient) []say {
	t.Helper()
	var out []say
	for _, f := range drainFrames(t, c) {
		if f[0] != serverpackets.OpcodeCreatureSay {
			continue
		}
		r := wire.NewReader(f[1:])
		s := say{speaker: r.ReadInt32(), channel: r.ReadInt32()}
		s.name, s.text = r.ReadString(), r.ReadString()
		out = append(out, s)
	}
	return out
}

type say struct {
	speaker, channel int32
	name, text       string
}

// channelChat has seat from say text on channel and returns, per seat, the
// chat lines each client received for it.
func channelChat(t *testing.T, g *group, from int, channel int32, text string) [][]say {
	t.Helper()
	g.players[from].c.Send(encodeSay2(channel, text))
	out := make([][]say, len(g.players))
	for i, p := range g.players {
		out[i] = says(t, p.c)
	}
	return out
}

// wantHeard fails unless exactly the seats in heard received one line of
// the speaker on channel with text, and every other seat none.
func wantHeard(t *testing.T, g *group, got [][]say, from int, channel int32, text string, heard ...int) {
	t.Helper()
	want := make(map[int]bool, len(heard))
	for _, i := range heard {
		want[i] = true
	}
	for i, lines := range got {
		if !want[i] {
			if len(lines) != 0 {
				t.Fatalf("%s got %+v on channel %d, want nothing", g.players[i].name, lines, channel)
			}
			continue
		}
		wantLine := say{speaker: g.players[from].id, channel: channel, name: g.players[from].name, text: text}
		if len(lines) != 1 || lines[0] != wantLine {
			t.Fatalf("%s got %+v, want exactly %+v", g.players[i].name, lines, wantLine)
		}
	}
}

// TestCommandChannelChat pins the two command channel chat channels. The
// party leader of any party in the channel speaks PARTYROOM_ALL to every
// member of every party, the channel's own leader included; PARTYROOM_COMMANDER
// is the channel leader's alone. Anyone else, a party leader of the second
// party on the commander channel and a plain member on either, is dropped
// without an answer, and so is a player of no channel. A member who blocks
// the speaker is skipped; the rest still hear the line.
func TestCommandChannelChat(t *testing.T) {
	t.Parallel()
	seats := []seat{{"Leader", 40}, {"Member", 30}, {"Other", 40}, {"Fourth", 50}, {"Loner", 20}}
	g, _ := bootAuthority(t, seats, authorityClan{level: 5, leads: true}, 1)
	g.invite(t, 0, 1, 0)
	g.invite(t, 2, 3, 0)
	g.askAndAccept(t, 0, 2)
	const leader, member, other, fourth, loner = 0, 1, 2, 3, 4
	channelMembers := []int{leader, member, other, fourth}

	got := channelChat(t, g, leader, sayPartyRoomCommander, "commander line")
	wantHeard(t, g, got, leader, sayPartyRoomCommander, "commander line", channelMembers...)

	got = channelChat(t, g, leader, sayPartyRoomAll, "leader all")
	wantHeard(t, g, got, leader, sayPartyRoomAll, "leader all", channelMembers...)

	// The other party's leader speaks to the whole channel on ALL ...
	got = channelChat(t, g, other, sayPartyRoomAll, "other all")
	wantHeard(t, g, got, other, sayPartyRoomAll, "other all", channelMembers...)

	// ... but is not the channel's leader.
	got = channelChat(t, g, other, sayPartyRoomCommander, "other commander")
	wantHeard(t, g, got, other, sayPartyRoomCommander, "other commander")

	for _, who := range []int{member, fourth, loner} {
		for _, channel := range []int32{sayPartyRoomAll, sayPartyRoomCommander} {
			got = channelChat(t, g, who, channel, "not allowed")
			wantHeard(t, g, got, who, channel, "not allowed")
		}
	}

	// The Loner, in no party, never hears the channel.
	got = channelChat(t, g, leader, sayPartyRoomAll, "again")
	wantHeard(t, g, got, leader, sayPartyRoomAll, "again", channelMembers...)

	g.players[fourth].c.Send(encodeBlockAdd("Leader"))
	drainFrames(t, g.players[fourth].c)
	drainFrames(t, g.players[leader].c)
	for _, channel := range []int32{sayPartyRoomAll, sayPartyRoomCommander} {
		got = channelChat(t, g, leader, channel, "blocked")
		wantHeard(t, g, got, leader, channel, "blocked", leader, member, other)
	}
	// Blocking is per speaker: Fourth still hears Other.
	got = channelChat(t, g, other, sayPartyRoomAll, "other free")
	wantHeard(t, g, got, other, sayPartyRoomAll, "other free", channelMembers...)
}
