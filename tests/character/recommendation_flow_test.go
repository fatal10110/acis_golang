package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeRequestEvaluate(targetID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestEvaluate)
	w.WriteInt32(targetID)
	return w.Bytes()
}

// quietFrames collects every frame c receives until the stream stays quiet.
func quietFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var out [][]byte
	for range 100 {
		frame := c.ReadWithTimeout(rejectSilenceWindow)
		if frame == nil {
			return out
		}
		out = append(out, frame)
	}
	t.Fatal("client kept receiving frames after 100 drains")
	return nil
}

func enterWorldQuiet(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	drainQuiet(t, c)
}

type systemMessage struct {
	id     int32
	texts  []string
	number []int32
}

func parseSystemMessage(t *testing.T, frame []byte) systemMessage {
	t.Helper()
	r := wire.NewReader(frame[1:])
	m := systemMessage{id: r.ReadInt32()}
	for range r.ReadInt32() {
		switch r.ReadInt32() {
		case serverpackets.SystemMessageParamText:
			m.texts = append(m.texts, r.ReadString())
		default:
			m.number = append(m.number, r.ReadInt32())
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("parse SystemMessage: %v", err)
	}
	return m
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}

func onlineChar(t *testing.T, srv *gameservertest.Server, id int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d not online", id)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d is not a live character", id)
	}
	return c
}

func recommendationColumns(t *testing.T, srv *gameservertest.Server, id int32) (have, left int) {
	t.Helper()
	if err := srv.DB.QueryRowContext(context.Background(), `SELECT rec_have, rec_left FROM characters WHERE obj_Id = ?`, id).Scan(&have, &left); err != nil {
		t.Fatalf("read recommendation columns of %d: %v", id, err)
	}
	return have, left
}

// TestRecommendationFlow drives RequestEvaluate between two online players:
// the stored counters reach UserInfo, a recommendation answers both sides
// and persists, a repeat, an unknown target and a self-recommendation are
// refused with their messages, a target other than the selection is
// ignored, the record of who was recommended survives a relog, and the
// daily refresh resets online and stored characters alike.
func TestRecommendationFlow(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Giver", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
	)
	giver := srv.Client
	giverID := srv.SoleObjectID(t)
	takerID := srv.SeedCharacterFor(t, "player2", "Taker", 1, 0).ID
	taker := srv.DialClient(t, "player2", 1)
	if _, err := srv.DB.ExecContext(context.Background(), `UPDATE characters SET rec_left = 2, rec_have = 7 WHERE obj_Id = ?`, giverID); err != nil {
		t.Fatalf("seed counters: %v", err)
	}

	enterWorldQuiet(t, giver)
	enterWorldQuiet(t, taker)
	drainQuiet(t, giver)
	if g := onlineChar(t, srv, giverID); g.RecommendationsLeft() != 2 || g.RecommendationsHave() != 7 {
		t.Fatalf("restored giver counters left %d have %d, want 2 and 7", g.RecommendationsLeft(), g.RecommendationsHave())
	}

	// Nothing selected yet: the request is ignored.
	giver.Send(encodeRequestEvaluate(takerID))
	if frame := giver.ReadWithTimeout(rejectSilenceWindow); frame != nil {
		t.Fatalf("evaluate of an unselected player answered %#x", frame[0])
	}
	// An id no online player has: Select target.
	giver.Send(encodeRequestEvaluate(999999))
	if got := parseSystemMessage(t, giver.Read()); got.id != 242 {
		t.Fatalf("unknown target message %d, want 242", got.id)
	}

	giver.Send(encodeAction(takerID, 0, 0, 0, false))
	drainQuiet(t, giver)
	drainQuiet(t, taker)

	giver.Send(encodeRequestEvaluate(takerID))
	frames := quietFrames(t, giver)
	if len(frames) < 3 || frames[0][0] != serverpackets.OpcodeSystemMessage || frames[1][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("giver frames %x, want SystemMessage, UserInfo, then the taker's CharInfo", opcodes(frames))
	}
	if got := parseSystemMessage(t, frames[0]); got.id != 830 || len(got.texts) != 1 || got.texts[0] != "Taker" || len(got.number) != 1 || got.number[0] != 1 {
		t.Fatalf("giver message %+v, want 830 naming Taker with 1 left", got)
	}
	if !containsOpcode(frames[2:], serverpackets.OpcodeCharInfo) {
		t.Fatalf("giver frames %x lack the taker's refreshed CharInfo", opcodes(frames))
	}
	takerFrames := quietFrames(t, taker)
	if len(takerFrames) < 2 || takerFrames[0][0] != serverpackets.OpcodeSystemMessage || takerFrames[1][0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("taker frames %x, want SystemMessage then UserInfo", opcodes(takerFrames))
	}
	if got := parseSystemMessage(t, takerFrames[0]); got.id != 831 || len(got.texts) != 1 || got.texts[0] != "Giver" {
		t.Fatalf("taker message %+v, want 831 naming Giver", got)
	}
	if g, tk := onlineChar(t, srv, giverID), onlineChar(t, srv, takerID); g.RecommendationsLeft() != 1 || tk.RecommendationsHave() != 1 {
		t.Fatalf("after recommending: giver left %d, taker have %d; want 1 and 1", g.RecommendationsLeft(), tk.RecommendationsHave())
	}

	// The same target again: already recommended.
	giver.Send(encodeRequestEvaluate(takerID))
	if got := parseSystemMessage(t, giver.Read()); got.id != 832 {
		t.Fatalf("repeat message %d, want 832", got.id)
	}
	// The taker selects itself and is refused before its level is checked.
	taker.Send(encodeAction(takerID, 0, 0, 0, false))
	drainQuiet(t, taker)
	taker.Send(encodeRequestEvaluate(takerID))
	if got := parseSystemMessage(t, taker.Read()); got.id != 829 {
		t.Fatalf("self message %d, want 829", got.id)
	}
	// The level-1 taker cannot recommend the giver.
	taker.Send(encodeAction(giverID, 0, 0, 0, false))
	drainQuiet(t, taker)
	drainQuiet(t, giver)
	taker.Send(encodeRequestEvaluate(giverID))
	if got := parseSystemMessage(t, taker.Read()); got.id != 898 {
		t.Fatalf("low-level message %d, want 898", got.id)
	}

	srv.FlushPersistence(t)
	var recorded int
	if err := srv.DB.QueryRow(`SELECT COUNT(*) FROM character_recommends WHERE char_id = ? AND target_id = ?`, giverID, takerID).Scan(&recorded); err != nil || recorded != 1 {
		t.Fatalf("character_recommends rows = %d, %v; want 1", recorded, err)
	}
	if have, _ := recommendationColumns(t, srv, takerID); have != 1 {
		t.Fatalf("stored taker rec_have = %d, want 1", have)
	}
	if _, left := recommendationColumns(t, srv, giverID); left != 1 {
		t.Fatalf("stored giver rec_left = %d, want 1", left)
	}

	// After a relog the giver still may not recommend the taker again.
	giver.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	drainQuiet(t, giver)
	drainQuiet(t, taker)
	enterWorldQuiet(t, giver)
	drainQuiet(t, taker)
	giver.Send(encodeAction(takerID, 0, 0, 0, false))
	drainQuiet(t, giver)
	giver.Send(encodeRequestEvaluate(takerID))
	if got := parseSystemMessage(t, giver.Read()); got.id != 832 {
		t.Fatalf("repeat after relog message %d, want 832", got.id)
	}

	// The daily refresh: each online player gets its UserInfo with the
	// counters reset by level, and the stored rows follow.
	drainQuiet(t, giver)
	drainQuiet(t, taker)
	srv.RefreshDailyRecommendations(t)
	for _, c := range []*testsupport.ScriptedClient{giver, taker} {
		if frames := quietFrames(t, c); len(frames) != 1 || frames[0][0] != serverpackets.OpcodeUserInfo {
			t.Fatalf("refresh frames %x, want one UserInfo", opcodes(frames))
		}
	}
	if g, tk := onlineChar(t, srv, giverID), onlineChar(t, srv, takerID); g.RecommendationsLeft() != 6 || g.RecommendationsHave() != 5 ||
		tk.RecommendationsLeft() != 3 || tk.RecommendationsHave() != 0 {
		t.Fatalf("after refresh: giver left %d have %d, taker left %d have %d; want 6/5 and 3/0",
			g.RecommendationsLeft(), g.RecommendationsHave(), tk.RecommendationsLeft(), tk.RecommendationsHave())
	}
	if have, left := recommendationColumns(t, srv, giverID); have != 5 || left != 6 {
		t.Fatalf("stored giver after refresh have %d left %d, want 5 and 6", have, left)
	}
	if have, left := recommendationColumns(t, srv, takerID); have != 0 || left != 3 {
		t.Fatalf("stored taker after refresh have %d left %d, want 0 and 3", have, left)
	}
	// The record is gone, so the giver may recommend the taker again.
	giver.Send(encodeRequestEvaluate(takerID))
	if got := parseSystemMessage(t, giver.Read()); got.id != 830 || got.number[0] != 5 {
		t.Fatalf("recommend after refresh %+v, want 830 with 5 left", got)
	}
}

func containsOpcode(frames [][]byte, opcode byte) bool {
	for _, f := range frames {
		if f[0] == opcode {
			return true
		}
	}
	return false
}
