package skills

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// loadingPhoenixID is a saved Phoenix Blessing: a death under it offers the
// player its own resurrection.
const loadingPhoenixID = 1002

func loadingPhoenixDef() modelskill.Definition {
	return modelskill.Definition{
		ID: loadingPhoenixID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		StaticHitTime: true, SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{Name: "PhoenixBless", Time: 600, Icon: true}},
	}
}

// TestRestoredLethalPoisonDeathUnderPhoenixBlessingOffersReviveInTheBurst:
// a restored lethal poison that kills on the loading screen under a
// restored Phoenix Blessing opens the blessing's resurrection offer there,
// which stays open and refuses every other resurrection until answered.
// The death sends its client nothing, so the EnterWorld burst offers it
// right after its Die frame; accepting it revives the player.
func TestRestoredLethalPoisonDeathUnderPhoenixBlessingOffersReviveInTheBurst(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{lethalPoisonDef(), loadingPhoenixDef()})),
	)
	if !srv.DrivesClock() {
		t.Skip("holding the loading screen for a set time needs the driven clock")
	}
	objID := srv.SoleObjectID(t)
	ctx := context.Background()
	if _, err := srv.DB.ExecContext(ctx, `UPDATE characters SET curHp = ? WHERE obj_Id = ?`, lethalPoisonHP, objID); err != nil {
		t.Fatalf("seed saved HP: %v", err)
	}
	for i, saved := range []struct {
		id             modelskill.ID
		count, elapsed int32
	}{{lethalPoisonDef().ID, loadingPoisonTicks, 1}, {loadingPhoenixID, 1, 10}} {
		if _, err := srv.DB.ExecContext(ctx,
			`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			objID, saved.id, 1, saved.count, saved.elapsed, 0, 0, 0, 0, i+1); err != nil {
			t.Fatalf("seed saved effect %d: %v", saved.id, err)
		}
	}
	c := srv.Client
	selectOnly(t, c)
	srv.Advance(t, lethalPoisonKill+2*time.Second)
	if queued := c.ReadQueued(); len(queued) != 0 {
		t.Fatalf("frame %#x sent on the loading screen, want nothing before the EnterWorld burst", queued[0][0])
	}

	c.Send(encodeEnterWorld())
	frames := readFrameSequence(t, c, []byte{
		serverpackets.OpcodeSendMacroList,
		serverpackets.OpcodeExtended,
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeAbnormalStatusUpdate,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeQuestList,
		serverpackets.OpcodeSkillList,
		serverpackets.OpcodeFriendList,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeItemList,
		serverpackets.OpcodeShortCutInit,
		serverpackets.OpcodeDie,
		serverpackets.OpcodeConfirmDlg,
		serverpackets.OpcodeSkillCoolTime,
		serverpackets.OpcodeActionFailed,
	})
	// A death under a Phoenix Blessing keeps every effect, the poison too.
	entries := readAbnormalStatusUpdateEntriesFromFrame(t, frames[3])
	if len(entries) != 2 || entries[0].SkillID != loadingPhoenixID || entries[1].SkillID != int32(lethalPoisonDef().ID) {
		t.Fatalf("EnterWorld AbnormalStatusUpdate after the poison's death = %+v, want the Phoenix Blessing %d and the poison kept", entries, loadingPhoenixID)
	}
	assertOwnResurrectionOffer(t, frames[14], "Newbie")
	if !srv.PlayerDead(t, objID) {
		t.Fatal("character alive after EnterWorld, want it dead")
	}

	w := wire.NewPacketWriter(clientpackets.OpcodeDlgAnswer)
	w.WriteInt32(serverpackets.ConfirmDlgResurrectionRequest)
	w.WriteInt32(1)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	srv.Settle(t)
	if srv.PlayerDead(t, objID) {
		t.Fatal("character still dead after accepting the burst's resurrection offer")
	}
}

// assertOwnResurrectionOffer checks a ConfirmDlg frame is the resurrection
// offer naming reviver.
func assertOwnResurrectionOffer(t *testing.T, frame []byte, reviver string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeConfirmDlg, "resurrection ConfirmDlg")
	r := wireReader(frame[1:])
	if id, params, typ := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != serverpackets.ConfirmDlgResurrectionRequest || params != 1 || typ != 0 {
		t.Fatalf("ConfirmDlg = id %d params %d type %d, want %d/1/text", id, params, typ, serverpackets.ConfirmDlgResurrectionRequest)
	}
	if got := r.ReadString(); got != reviver {
		t.Fatalf("ConfirmDlg reviver = %q, want %q", got, reviver)
	}
	if r.Remaining() != 0 {
		t.Fatalf("ConfirmDlg carries %d trailing bytes, want none", r.Remaining())
	}
}
