package skills

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestEnterWorldAnnouncesShadowSenseState seeds a dark elf holding Shadow
// Sense (skill 294) through the real stores and verifies EnterWorld announces
// the night/day state change with the skill-name parameter tuple.
func TestEnterWorldAnnouncesShadowSenseState(t *testing.T) {
	var objID int32
	srv := gameservertest.Boot(t,
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
			tmpl, ok := gameservertest.Templates(t).Get(0)
			if !ok {
				t.Fatal("missing test class template")
			}
			ch, err := player.NewCharacter(500, tmpl, "player1", "Shadow", 1, 0, 0, player.SexMale)
			if err != nil {
				t.Fatalf("new shadow: %v", err)
			}
			ch.Race = player.RaceDarkElf
			if err := chars.Create(context.Background(), ch); err != nil {
				t.Fatalf("seed shadow: %v", err)
			}
			objID = ch.ID
		}),
	)
	c := srv.DialClient(t, "player1", 1)
	// Persist Shadow Sense through the real known-skill store exactly like
	// a learned passive; EnterWorld's restore reads it back.
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, 294, 1); err != nil {
		t.Fatalf("seed Shadow Sense: %v", err)
	}

	c.Send(encodeRequestGameStart(0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	c.Send(encodeEnterWorld())

	found := readShadowSenseAnnouncement(t, c)
	drainUntilQuiet(t, c)
	if !found {
		t.Fatal("Shadow Sense state message missing after EnterWorld")
	}
}

// readShadowSenseAnnouncement scans the EnterWorld frames for the night/day
// skill announcement and validates its parameter tuple.
func readShadowSenseAnnouncement(t *testing.T, c *testsupport.ScriptedClient) bool {
	t.Helper()
	for range 20 {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			return false
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(frame[1:])
		messageID := r.ReadInt32()
		if messageID != serverpackets.SystemMessageNightSkillEffectApplies && messageID != serverpackets.SystemMessageDaySkillEffectDisappears {
			continue
		}
		if params := r.ReadInt32(); params != 1 {
			t.Fatalf("Shadow Sense message params = %d, want 1", params)
		}
		if kind := r.ReadInt32(); kind != serverpackets.SystemMessageParamSkillName {
			t.Fatalf("Shadow Sense message parameter type = %d, want skill name", kind)
		}
		if skillID, level := r.ReadInt32(), r.ReadInt32(); skillID != 294 || level != 1 {
			t.Fatalf("Shadow Sense message skill = %d level %d, want 294 level 1", skillID, level)
		}
		return true
	}
	return false
}

// seedEnchanter boots a linked server whose client plays an enchant-eligible
// character: third profession (class 88), level 76, deep SP/exp pockets, and
// skill 1 known at currentLevel.
func seedEnchanter(t *testing.T, currentLevel int, opts ...gameservertest.Option) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	var objID int32
	opts = append([]gameservertest.Option{
		gameservertest.WithWantChars(1),
		gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
			tmpl, ok := gameservertest.Templates(t).Get(88)
			if !ok {
				t.Fatal("missing duelist test template")
			}
			ch, err := player.NewCharacter(500, tmpl, "player1", "Enchanter", 1, 0, 0, player.SexMale)
			if err != nil {
				t.Fatalf("new enchanter: %v", err)
			}
			ch.CharLevel = 76
			ch.SP = 1_000_000
			ch.Exp = 100_000_000
			if err := chars.Create(context.Background(), ch); err != nil {
				t.Fatalf("seed enchanter: %v", err)
			}
			objID = ch.ID
		}),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{ID: 1, Level: 1},
			{ID: 1, Level: 101},
			{ID: 1, Level: 102},
		})),
	}, opts...)
	srv := gameservertest.Boot(t, opts...)
	seedKnownSkill(t, srv, objID, 1, currentLevel)
	return srv, srv.Client, objID
}

// TestEnchantSkillSuccessRefreshesShortcut walks a deterministic successful
// enchant: the trainer info quotes the offer's rate, the apply re-points the
// bound shortcut before the success message lands, refreshes the skill list
// and user info, and persists the enchanted level.
func TestEnchantSkillSuccessRefreshesShortcut(t *testing.T) {
	srv, c, objID := seedEnchanter(t, 1, gameservertest.WithSkillTrees(enchantTree(101, 100)))
	bindSkillShortcut(t, srv, objID, 0, 1, 1)
	startInWorld(t, c)

	c.Send(encodeRequestExEnchantSkillInfo(1, 101))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeExtended, "ExEnchantSkillInfo")
	r := wireReader(frame[1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExEnchantSkillInfo {
		t.Fatalf("extended opcode = %#x, want ExEnchantSkillInfo (%#x)", sub, serverpackets.OpcodeExEnchantSkillInfo)
	}
	id, level, spCost, xpCost, rate, reqs := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt64(), r.ReadInt32(), r.ReadInt32()
	if id != 1 || level != 101 || spCost != 0 || xpCost != 0 || rate != 100 || reqs != 0 {
		t.Fatalf("ExEnchantSkillInfo = %d/%d/%d/%d/%d/%d, want 1/101/0/0/100/0", id, level, spCost, xpCost, rate, reqs)
	}

	c.Send(encodeRequestExEnchantSkill(1, 101))
	assertShortCutRegister(t, c, 0, 1, 101)
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageSucceededEnchantingSkillS1, 1, 101)
	assertSkillListContains(t, c.Read(), skillListEntry{passive: 1, level: 101, id: 1})
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "post-enchant UserInfo")
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{1: 101})
}

// TestEnchantSkillFailureResetsShortcutAndLevel verifies a failed roll (rate
// 0, deterministic dice above the rate) resets the skill to its top
// non-enchanted level, re-points the shortcut first, and reports the failure.
func TestEnchantSkillFailureResetsShortcutAndLevel(t *testing.T) {
	srv, c, objID := seedEnchanter(t, 101,
		gameservertest.WithSkillTrees(enchantTree(102, 0)),
		gameservertest.WithSkillEnchantRoll(func() int { return 50 }),
	)
	bindSkillShortcut(t, srv, objID, 0, 1, 101)
	startInWorld(t, c)

	c.Send(encodeRequestExEnchantSkill(1, 102))
	assertShortCutRegister(t, c, 0, 1, 1)
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageFailedEnchantingSkillS1, 1, 102)
	assertSkillListContains(t, c.Read(), skillListEntry{passive: 1, level: 1, id: 1})
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "post-enchant UserInfo")
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{1: 1})
}
