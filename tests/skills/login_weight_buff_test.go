package skills

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRestoredWeightLimitBuffSetsLoginWeightBand logs in a character carrying
// 1.5 times its base weight limit with a saved buff that doubles the limit
// (#2535). The reference restores effects while loading the character
// (Player.java:4146-4153) and decides the band only at the ItemList
// constructor's weight recompute (ItemList.java:20, PcInventory.java:112,
// Player.java:1113-1140), so the band comes from the buffed limit: a 0.75
// ratio, LEVEL_2, not the LEVEL_4 the unbuffed limit gives. Packet fields are
// filled when the selector writes them, after that recompute, so every burst
// frame that carries the band or the move speed already shows LEVEL_2: the
// first EtcStatusUpdate, the burst UserInfo, and the band refresh's UserInfo
// and EtcStatusUpdate between the load gauge and the ItemList.
func TestRestoredWeightLimitBuffSetsLoginWeightBand(t *testing.T) {
	t.Parallel()
	const (
		skillID      = 1062
		heavyIngotID = 9500 // weight 10, stackable
		con          = 43   // statClass CON
	)
	limit := int(baseWeightLimit * statbonus.CONBonus[con])
	buff := selfBuff(skillID, modelskill.FuncTemplate{Op: modelskill.FuncMul, Stat: "weightLimit", Value: 2})
	buff.Effects[0].Time = 600
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Porter", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithWeightLimitMultiplier(1),
		statClass(),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{buff})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, skillID, 1)
	load := int32(limit*15/100) * 10 // 150% of the unbuffed limit
	srv.GiveItem(t, objID, heavyIngotID, load/10)
	if _, err := srv.DB.ExecContext(context.Background(),
		`INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		objID, skillID, 1, 1, 0, 0, 0, 0, 0, 0); err != nil {
		t.Fatalf("seed saved buff: %v", err)
	}

	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	c.Send(encodeEnterWorld())
	want := []byte{
		serverpackets.OpcodeSendMacroList,
		serverpackets.OpcodeExtended, // ExStorageMaxCount
		serverpackets.OpcodeHennaInfo,
		serverpackets.OpcodeAbnormalStatusUpdate,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeExtended, // ExSetCompassZoneCode
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeQuestList,
		serverpackets.OpcodeSkillList,
		serverpackets.OpcodeFriendList,
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeStatusUpdate, // CUR_LOAD
		serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeItemList,
		serverpackets.OpcodeShortCutInit,
		serverpackets.OpcodeSkillCoolTime,
		serverpackets.OpcodeActionFailed,
	}
	frames := make([][]byte, 0, len(want))
	for i, opcode := range want {
		frame := c.Read()
		if frame == nil {
			t.Fatalf("EnterWorld frame %d (want %#x) never arrived", i, opcode)
		}
		if frame[0] != opcode {
			t.Fatalf("EnterWorld frame %d opcode = %#x, want %#x", i, frame[0], opcode)
		}
		frames = append(frames, frame)
	}
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("EnterWorld sent opcode %#x after its ActionFailed", frame[0])
	}

	for _, i := range []int{4, 14} {
		r := wire.NewReader(frames[i][1:])
		r.ReadInt32() // charges
		if band := r.ReadInt32(); band != 2 {
			t.Fatalf("EnterWorld frame %d EtcStatusUpdate weight penalty = %d, want 2", i, band)
		}
	}
	// LEVEL_2 halves the move speed (WeightPenalty.java:5-9); LEVEL_4, the
	// unbuffed band, would zero it.
	for _, i := range []int{11, 13} {
		if got := decodeUserInfoSpeeds(t, frames[i]).moveMult; got <= 0 || got >= 1 {
			t.Fatalf("EnterWorld frame %d UserInfo move multiplier = %v, want the LEVEL_2 halved speed", i, got)
		}
	}
	r := wire.NewReader(frames[12][1:])
	if id, n, typ, value := r.ReadInt32(), r.ReadInt32(), serverpackets.StatusType(r.ReadInt32()), r.ReadInt32(); id != objID || n != 1 || typ != serverpackets.StatusCurrentLoad || value != load {
		t.Fatalf("login StatusUpdate = object %d, %d attrs, type %d = %d; want %d, CUR_LOAD alone = %d", id, n, typ, value, objID, load)
	}

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world")
	}
	loaded := obj.(interface {
		WeightLimit() int
		WeightPenalty() int
	})
	if got := loaded.WeightLimit(); got != 2*limit {
		t.Fatalf("weight limit after login = %d, want the buffed %d", got, 2*limit)
	}
	if got := loaded.WeightPenalty(); got != 2 {
		t.Fatalf("weight penalty after login = %d, want 2", got)
	}
}
