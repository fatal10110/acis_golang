package network

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The command channel lines reach every member of every party in the
// channel, the speaker included, except a member who blocks the speaker.
// The commander line is the channel leader's alone and the all-channel line
// a party leader's alone; from anyone else either is dropped unheard.
func TestChatCommandChannelDelivery(t *testing.T) {
	in := sim.NewInline(time.Unix(0, 0))
	l := &GameClientLink{log: zerolog.Nop(), queues: in, parties: party.NewRegistry[*livePlayer](in.Now), relations: relation.NewManager(nil)}

	// Players 1..6 in three parties led by 1, 3 and 5; player 1's party
	// leads the channel the other two join. Player 4 blocks the channel
	// leader, player 6 blocks party leader 3.
	captures := make([]*testsupport.FrameCapture, 7)
	players := make([]*livePlayer, 7)
	for id := int32(1); id <= 6; id++ {
		captures[id] = &testsupport.FrameCapture{}
		players[id] = newTestLivePlayer(t, id, captures[id])
	}
	for _, pair := range [][2]int32{{1, 2}, {3, 4}, {5, 6}} {
		if status, _ := l.parties.BeginInvite(pair[0], 0); status != party.InviteReady {
			t.Fatalf("BeginInvite(%d) = %v", pair[0], status)
		}
		l.parties.Answer(players[pair[0]], players[pair[1]], true)
	}
	l.parties.JoinChannel(players[1], players[3])
	l.parties.JoinChannel(players[1], players[5])
	l.relations.Block(4, 1)
	l.relations.Block(6, 3)

	for _, tc := range []struct {
		name    string
		typ     chat.Type
		speaker int32
		want    []int32
	}{
		{"commander line from the channel leader", chat.PartyRoomCommander, 1, []int32{1, 2, 3, 5, 6}},
		{"all-channel line from a party leader", chat.PartyRoomAll, 3, []int32{1, 2, 3, 4, 5}},
		{"all-channel line from the channel leader", chat.PartyRoomAll, 1, []int32{1, 2, 3, 5, 6}},
		{"commander line from a party leader who does not lead the channel", chat.PartyRoomCommander, 3, nil},
		{"commander line from a party member", chat.PartyRoomCommander, 2, nil},
		{"all-channel line from a party member", chat.PartyRoomAll, 4, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testsupport.ResetCapture(captures[1:]...)
			line := chat.Line{Type: tc.typ, Text: "hold"}
			chatHandlers[tc.typ](l, nil, players[tc.speaker], line)

			frame := serverpackets.FrameCreatureSay(tc.speaker, int32(tc.typ), players[tc.speaker].Name, "hold")
			want := string(frame.Bytes()[2:]) // the capture records payloads without the length prefix
			frame.Release()
			heard := map[int32]bool{}
			for _, id := range tc.want {
				heard[id] = true
			}
			for id := int32(1); id <= 6; id++ {
				frames := captures[id].Frames()
				if !heard[id] {
					if len(frames) != 0 {
						t.Errorf("player %d got %d frames, want none", id, len(frames))
					}
					continue
				}
				if len(frames) != 1 || string(frames[0]) != want {
					t.Errorf("player %d frames = %x, want one CreatureSay %x", id, frames, want)
				}
			}
		})
	}
}
