package social

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference ids of the chat refusals a punishment answers.
const (
	chattingProhibited = 147
	targetIsChatBanned = 1079
)

// TestChatBanFromStoredRow pins a chat ban restored from the characters row
// (Player.restore → Punishment.load, Punishment.handle at onPlayerEnter):
// the login EtcStatusUpdate carries 1 in its third field, the reminder of
// the minutes left comes between ShortCutInit and SkillCoolTime, the
// player's chat is refused with CHATTING_PROHIBITED and whispers to it with
// TARGET_IS_CHAT_BANNED, and the ban ends on the stored timer with an
// EtcStatusUpdate clearing the flag, the notice and the sound, after which
// the row is clear and the player speaks again.
func TestChatBanFromStoredRow(t *testing.T) {
	p := bootPair(t)
	if _, err := p.srv.DB.ExecContext(context.Background(), "UPDATE characters SET punish_level = 1, punish_timer = 4000 WHERE obj_Id = ?", p.bobbyID); err != nil {
		t.Fatalf("seed chat ban: %v", err)
	}
	startInWorld(t, p.alice)

	p.bobby.Send(encodeRequestGameStart(0))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	p.bobby.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	want := []byte{
		serverpackets.OpcodeSendMacroList,
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeQuestList,
		serverpackets.OpcodeSkillList,
		serverpackets.OpcodeFriendList,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeItemList,
		serverpackets.OpcodeShortCutInit,
		serverpackets.OpcodeSystemMessage, // the reminder
		serverpackets.OpcodeSkillCoolTime,
		serverpackets.OpcodeActionFailed,
	}
	frames := make([][]byte, 0, len(want))
	for i, opcode := range want {
		frame := p.bobby.Read()
		for frame[0] == serverpackets.OpcodeCharInfo || frame[0] == serverpackets.OpcodeRelationChanged {
			frame = p.bobby.Read()
		}
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
		frames = append(frames, frame)
		if opcode == serverpackets.OpcodeEtcStatusUpdate {
			gameservertest.ReadInitialCompass(t, p.bobby, serverpackets.OpcodeCharInfo, serverpackets.OpcodeRelationChanged)
		}
	}
	assertEtcBlocked(t, frames[3], true)
	assertSystemMessageText(t, frames[12], serverpackets.SystemMessageS1, "You are still chat banned for 0 minutes.")
	drainUntilQuiet(t, p.alice)

	p.bobby.Send(encodeSay2(sayAll, "hello"))
	assertStaticSystemMessage(t, p.bobby.Read(), chattingProhibited)
	assertNoSay(t, p.alice, "chat of a chat-banned player")
	p.alice.Send(encodeTell("psst", "Bobby"))
	assertStaticSystemMessage(t, p.alice.Read(), targetIsChatBanned)
	assertNoSay(t, p.bobby, "whisper to a chat-banned player")

	var end [][]byte
	for len(end) < 3 {
		frame := p.bobby.ReadWithTimeout(15 * time.Second)
		if frame == nil {
			t.Fatal("chat ban did not end")
		}
		end = append(end, frame)
	}
	assertEtcBlocked(t, end[0], false)
	assertSystemMessageText(t, end[1], serverpackets.SystemMessageS1, "Chatting is now available.")
	assertOpcode(t, end[2], serverpackets.OpcodePlaySound, "PlaySound")
	r := wire.NewReader(end[2][1:])
	r.ReadInt32()
	if file := r.ReadString(); file != "systemmsg_e.345" {
		t.Fatalf("sound = %q, want systemmsg_e.345", file)
	}
	p.srv.AdvanceUntil(t, "chat ban row cleared", func() bool {
		var level int
		var timer int64
		if err := p.srv.DB.QueryRowContext(context.Background(), "SELECT punish_level, punish_timer FROM characters WHERE obj_Id = ?", p.bobbyID).Scan(&level, &timer); err != nil {
			t.Fatalf("read punishment: %v", err)
		}
		return level == 0 && timer == 0
	})

	p.bobby.Send(encodeSay2(sayAll, "hello"))
	assertSay(t, p.bobby, p.bobbyID, sayAll, "Bobby", "hello")
	assertSay(t, p.alice, p.bobbyID, sayAll, "Bobby", "hello")
}
