package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// loginWeightBurst is the EnterWorld reply a solo login with weighted items
// receives, compass included. Restore leaves the carried weight at 0, and the
// first recompute is the one the ItemList send makes, so the load gauge lands
// between UserInfo and ItemList rather than ahead of the burst
// (EnterWorld.java:221-223, ItemList.java:14-24, PcInventory.java:101-113).
var loginWeightBurst = []byte{
	serverpackets.OpcodeSendMacroList,
	serverpackets.OpcodeExtended, // ExStorageMaxCount
	serverpackets.OpcodeHennaInfo,
	serverpackets.OpcodeEtcStatusUpdate,
	serverpackets.OpcodeExtended, // ExSetCompassZoneCode
	serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodeQuestList,
	serverpackets.OpcodeSkillList,
	serverpackets.OpcodeFriendList,
	serverpackets.OpcodeUserInfo,
	serverpackets.OpcodeStatusUpdate, // CUR_LOAD
	serverpackets.OpcodeItemList,
	serverpackets.OpcodeShortCutInit,
	serverpackets.OpcodeSkillCoolTime,
	serverpackets.OpcodeActionFailed,
}

// overloadedLoginBurst is loginWeightBurst for a load past the weight limit:
// the recompute also moves the penalty band from NONE to LEVEL_4, whose
// refresh sends UserInfo and EtcStatusUpdate right after the load gauge
// (Player.java:1113-1140), still ahead of the ItemList.
var overloadedLoginBurst = []byte{
	serverpackets.OpcodeSendMacroList,
	serverpackets.OpcodeExtended,
	serverpackets.OpcodeHennaInfo,
	serverpackets.OpcodeEtcStatusUpdate,
	serverpackets.OpcodeExtended,
	serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodeSystemMessage,
	serverpackets.OpcodeQuestList,
	serverpackets.OpcodeSkillList,
	serverpackets.OpcodeFriendList,
	serverpackets.OpcodeUserInfo,
	serverpackets.OpcodeStatusUpdate,
	serverpackets.OpcodeUserInfo,
	serverpackets.OpcodeEtcStatusUpdate,
	serverpackets.OpcodeItemList,
	serverpackets.OpcodeShortCutInit,
	serverpackets.OpcodeSkillCoolTime,
	serverpackets.OpcodeActionFailed,
}

// readLoginBurst selects slot 0, enters the world and reads exactly
// len(want) frames, failing on the first opcode out of place.
func readLoginBurst(t *testing.T, c *testsupport.ScriptedClient, want []byte) [][]byte {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
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
	if frame := c.ReadWithTimeout(rejectSilenceWindow); frame != nil {
		t.Fatalf("EnterWorld sent opcode %#x after its ActionFailed", frame[0])
	}
	return frames
}

// userInfoCurrentLoad decodes the carried-weight field of a UserInfo frame
// (UserInfo.java:32-53: location, heading, object id, name, race, sex,
// class, level, exp, six base stats, HP/MP pairs, SP, then current load).
func userInfoCurrentLoad(t *testing.T, frame []byte) int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("opcode = %#x, want UserInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	for range 4 { // race, sex, class, level
		r.ReadInt32()
	}
	r.ReadInt64()
	for range 6 + 5 { // STR..MEN, max HP, HP, max MP, MP, SP
		r.ReadInt32()
	}
	return r.ReadInt32()
}

// etcStatusWeightPenalty decodes the weight penalty band of an
// EtcStatusUpdate frame (EtcStatusUpdate.java:19-21: charges, then band).
func etcStatusWeightPenalty(t *testing.T, frame []byte) int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodeEtcStatusUpdate {
		t.Fatalf("opcode = %#x, want EtcStatusUpdate", frame[0])
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // charges
	return r.ReadInt32()
}

// TestEnterWorldSendsRestoredWeightBetweenUserInfoAndItemList covers issues
// #1144 and #2535. Restore rebuilds the inventory without computing its
// weight, so the burst's UserInfo still reports a load of 0; the recompute
// the ItemList send makes then reports the real load in a StatusUpdate that
// sits between UserInfo and ItemList (EnterWorld.java:221-223,
// ItemList.java:14-24, PcInventory.java:101-113). Nothing about the weight
// reaches the client ahead of SendMacroList.
func TestEnterWorldSendsRestoredWeightBetweenUserInfoAndItemList(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, items *gamesql.ItemStore) {
			objID := seedCharacter(t, chars, "Newbie", 1, 0)
			if err := items.Create(context.Background(), objID, item.Instance{
				ObjectID: 500, TemplateID: 9500, OwnerID: objID, Count: 5, Location: item.LocationInventory,
			}); err != nil {
				t.Fatalf("seed item: %v", err)
			}
		}),
	)
	objID := srv.SoleObjectID(t)

	frames := readLoginBurst(t, srv.Client, loginWeightBurst)
	if got := userInfoCurrentLoad(t, frames[10]); got != 0 {
		t.Fatalf("burst UserInfo current load = %d, want 0 (weight not yet recomputed)", got)
	}
	assertStatusAttrs(t, frames[11], objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentLoad, Value: 50},
	})
	if got := etcStatusWeightPenalty(t, frames[3]); got != 0 {
		t.Fatalf("burst EtcStatusUpdate weight penalty = %d, want 0", got)
	}

	player, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing after EnterWorld", objID)
	}
	if got := player.(interface{ CurrentWeight() int }).CurrentWeight(); got != 50 {
		t.Fatalf("current weight after login = %d, want 50", got)
	}
}

// TestEnterWorldOverloadedRestoreRefreshesPenaltyBeforeItemList covers the
// band half of issue #2535. A restored load past the weight limit leaves the
// penalty band at NONE until the ItemList send's weight recompute, so the
// burst's first EtcStatusUpdate still carries band 0 and the LEVEL_4 refresh
// (UserInfo, EtcStatusUpdate carrying band 4) follows the load gauge between
// UserInfo and ItemList (Player.java:4153 refreshes at restore with no
// client attached and a still-zero load; Player.java:1113-1140). The player
// is left in band 4 with zero movement speed once the burst is done.
func TestEnterWorldOverloadedRestoreRefreshesPenaltyBeforeItemList(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithWeightLimitMultiplier(1),
	)
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 9500, 100_000)

	frames := readLoginBurst(t, srv.Client, overloadedLoginBurst)
	if got := etcStatusWeightPenalty(t, frames[3]); got != 0 {
		t.Fatalf("burst EtcStatusUpdate weight penalty = %d, want 0 (band not yet refreshed)", got)
	}
	if got := userInfoCurrentLoad(t, frames[10]); got != 0 {
		t.Fatalf("burst UserInfo current load = %d, want 0", got)
	}
	assertStatusAttrs(t, frames[11], objID, []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentLoad, Value: 1_000_000},
	})
	if got := userInfoCurrentLoad(t, frames[12]); got != 1_000_000 {
		t.Fatalf("band refresh UserInfo current load = %d, want 1000000", got)
	}
	if got := decodeUserInfoSpeeds(t, frames[12]).moveMult; got != 0 {
		t.Fatalf("band refresh UserInfo move multiplier = %v, want 0", got)
	}
	if got := etcStatusWeightPenalty(t, frames[13]); got != 4 {
		t.Fatalf("band refresh EtcStatusUpdate weight penalty = %d, want 4", got)
	}

	player, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing after EnterWorld", objID)
	}
	loaded := player.(interface {
		WeightPenalty() int
		MoveSpeed() float64
	})
	if got := loaded.WeightPenalty(); got != 4 {
		t.Fatalf("weight penalty after login = %d, want 4", got)
	}
	if got := loaded.MoveSpeed(); got != 0 {
		t.Fatalf("move speed after login = %v, want 0", got)
	}
}

// TestEnterWorldReGrantsFreeSkills is the behavior-suite regression test
// for issue #1149: Player.giveSkills() runs again on every login, right
// after restoreCharData() (Player.java:4139), so a free level-unlocked
// grant — which a prior in-session level-up handed out in memory only, per
// GiveSkills's own doc comment — comes back on relog instead of staying
// dropped. The shared test template grants skill 900001 for free from
// level 50; no other flow's character reaches that level, so this is the
// only enter-world path affected by the added grant.
func TestEnterWorldReGrantsFreeSkills(t *testing.T) {
	skills := skillstate.NewPersistence(nil, modelskill.NewTable([]modelskill.Definition{{ID: 900001, Level: 1}}))
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skills),
		gameservertest.WithCharacter("Newbie", 50, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)

	skillList := frames[7]
	if skillList[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("frame[6] opcode = %#x, want SkillList (%#x)", skillList[0], serverpackets.OpcodeSkillList)
	}
	r := wire.NewReader(skillList[1:])
	count := r.ReadInt32()
	for range count {
		if _, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id == 900001 {
			if level != 1 {
				t.Fatalf("skill 900001 level = %d, want 1", level)
			}
			return
		}
	}
	t.Fatalf("SkillList (%d entries) missing free grant skill 900001 re-derived on login", count)
}

func seedCharacter(t *testing.T, chars *gamesql.CharacterStore, name string, level, sp int) int32 {
	t.Helper()
	tmpl, ok := gameservertest.Templates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	ch, err := player.NewCharacter(100, tmpl, "player1", name, 1, 0, 0, player.SexMale)
	if err != nil {
		t.Fatalf("seed character: %v", err)
	}
	ch.CharLevel = level
	ch.SP = sp
	if err := chars.Create(context.Background(), ch); err != nil {
		t.Fatalf("seed character store: %v", err)
	}
	return ch.ID
}
