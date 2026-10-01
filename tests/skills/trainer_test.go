package skills

import (
	"strconv"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The trainer scenarios below follow the reference trainer flow: every
// learn and enchant request is made at the civilian NPC the player last
// selected and can still interact with (RequestAcquireSkillInfo,
// RequestAcquireSkill, RequestExEnchantSkillInfo, RequestExEnchantSkill:
// getCurrentFolk + canDoInteract early returns); only the two info
// requests also ask whether that NPC trains the player's profession
// (NpcTemplate.canTeach, a third profession checked through its parent).
// The list reopened after a learn or an enchant roll is Folk.showSkillList
// / Folk.showEnchantSkillList / Fisherman.showFishSkillList, and the
// npc_<id>_SkillList / EnchantSkillList / FishSkillList dialog commands
// open the same lists before the dispatcher's own ActionFailed.

// trainerPages are the dialog pages the bypass scenarios need.
func trainerPages() map[string]string {
	return map[string]string{
		"test/any.htm": anyNpcLinkPage,
		"trainer/" + strconv.Itoa(trainerNpcID) + "-noskills.htm": noSkillsPage,
	}
}

const noSkillsPage = "<html><body>I cannot teach you.</body></html>"

// noBypassReuse lifts the bypass reuse delay, so a scenario can send its
// commands back to back.
var noBypassReuse = gameservertest.WithReuseDelays(3*time.Second, 0)

// assertSilent asserts the server sends nothing in answer to the last
// request.
func assertSilent(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	if frames := readUntilQuiet(t, c); len(frames) != 0 {
		t.Fatalf("%s answered %x, want nothing", what, frameOpcodes(frames))
	}
}

// TestTrainerLearnNeedsCurrentFolkInReach pins the learn gate: with no NPC
// selected, or with the selected one out of interaction distance, the info
// and learn requests answer nothing and teach nothing; once a trainer in
// reach is selected the same requests are answered.
func TestTrainerLearnNeedsCurrentFolkInReach(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootLearner(t, generalLearnOpts(t, 50)...)
	startInWorld(t, c)

	for _, step := range []func(){
		func() {},
		func() {
			far := spawnTrainer(t, srv, c, objID, "Trainer", 1000, 0)
			selectNpc(t, srv, c, objID, far)
		},
	} {
		step()
		c.Send(encodeRequestAcquireSkillInfo(3, 1, 0))
		assertSilent(t, c, "AcquireSkillInfo without a trainer in reach")
		c.Send(encodeRequestAcquireSkill(3, 1, 0))
		assertSilent(t, c, "AcquireSkill without a trainer in reach")
		c.Send(encodeRequestAcquireSkillInfo(1368, 1, 1))
		assertSilent(t, c, "fishing AcquireSkillInfo without a trainer in reach")
	}
	assertKnownSkills(t, srv, objID, map[int]int{})

	selectTrainer(t, srv, c, objID, 0)
	c.Send(encodeRequestAcquireSkillInfo(3, 1, 0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAcquireSkillInfo, "AcquireSkillInfo at a trainer")
	drainUntilQuiet(t, c)
}

// TestTrainerLearnOfNotNextLevelAnswersNothing pins the silent learn
// refusal (RequestAcquireSkill case 0 returns with no packet when the
// requested level is not one above the known one): at a trainer in reach a
// level that skips the next one, and a level already known, answer nothing,
// charge no SP and teach nothing.
func TestTrainerLearnOfNotNextLevelAnswersNothing(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootLearner(t, generalLearnOpts(t, 100)...)
	startInWorld(t, c)
	selectTrainer(t, srv, c, objID, 0)

	c.Send(encodeRequestAcquireSkill(3, 2, 0))
	assertSilent(t, c, "AcquireSkill of a level past the next one")
	assertLiveSP(t, srv, objID, 100)
	assertKnownSkills(t, srv, objID, map[int]int{})

	c.Send(encodeRequestAcquireSkill(3, 1, 0))
	assertSPStatus(t, c.Read(), objID, 50)
	drainUntilQuiet(t, c)
	assertKnownSkills(t, srv, objID, map[int]int{3: 1})

	c.Send(encodeRequestAcquireSkill(3, 1, 0))
	assertSilent(t, c, "AcquireSkill of a level already known")
	assertLiveSP(t, srv, objID, 50)
	assertKnownSkills(t, srv, objID, map[int]int{3: 1})
}

// TestTrainerEnchantNeedsCurrentFolkInReach pins the enchant gate: with no
// NPC selected, or with the selected one out of interaction distance, the
// info and enchant requests answer nothing and change nothing.
func TestTrainerEnchantNeedsCurrentFolkInReach(t *testing.T) {
	t.Parallel()
	srv, c, objID := seedEnchanter(t, 1, gameservertest.WithSkillTrees(enchantTree(101, 100)))
	startInWorld(t, c)

	for _, step := range []func(){
		func() {},
		func() {
			far := spawnTrainer(t, srv, c, objID, "Trainer", 1000, duelistParentClass)
			selectNpc(t, srv, c, objID, far)
		},
	} {
		step()
		c.Send(encodeRequestExEnchantSkillInfo(1, 101))
		assertSilent(t, c, "ExEnchantSkillInfo without a trainer in reach")
		c.Send(encodeRequestExEnchantSkill(1, 101))
		assertSilent(t, c, "ExEnchantSkill without a trainer in reach")
	}
	assertKnownSkills(t, srv, objID, map[int]int{1: 1})
}

// TestTrainerInfoNeedsProfessionItTeaches pins canTeach on the info
// requests: a trainer of another profession answers neither the learn nor
// the enchant info, and a third profession is taught through the second
// one it upgrades from, not by its own id.
func TestTrainerInfoNeedsProfessionItTeaches(t *testing.T) {
	t.Parallel()
	t.Run("learn", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t, generalLearnOpts(t, 50)...)
		startInWorld(t, c)
		selectTrainer(t, srv, c, objID, 10)
		c.Send(encodeRequestAcquireSkillInfo(3, 1, 0))
		assertSilent(t, c, "AcquireSkillInfo at a trainer of another profession")
	})
	t.Run("enchant", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := seedEnchanter(t, 1, gameservertest.WithSkillTrees(enchantTree(101, 100)))
		startInWorld(t, c)
		selectTrainer(t, srv, c, objID, 88)
		c.Send(encodeRequestExEnchantSkillInfo(1, 101))
		assertSilent(t, c, "ExEnchantSkillInfo at a trainer listing the third profession itself")
	})
}

// TestTrainerLearnAtOtherProfessionReopensNoSkillsPage pins that the learn
// and the enchant themselves do not ask canTeach: at a trainer of another
// profession the skill is still learned or enchanted, and the list they
// reopen is that trainer's no-skills page, with no ActionFailed after it.
func TestTrainerLearnAtOtherProfessionReopensNoSkillsPage(t *testing.T) {
	t.Parallel()
	t.Run("learn", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t, append(generalLearnOpts(t, 50), gameservertest.WithHTMLPages(trainerPages()))...)
		startInWorld(t, c)
		f := selectTrainer(t, srv, c, objID, 10)

		c.Send(encodeRequestAcquireSkill(3, 1, 0))
		assertSPStatus(t, c.Read(), objID, 0)
		assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSystemMessage, "SP-decreased SystemMessage")
		assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageLearnedSkill, 3, 1)
		assertSkillList(t, c.Read(), skillListEntry{passive: 0, level: 1, id: 3})
		assertNpcHTML(t, c.Read(), f.ObjectID(), noSkillsPage+"\n")
		assertSilent(t, c, "after the no-skills page")
		assertKnownSkills(t, srv, objID, map[int]int{3: 1})
	})
	t.Run("enchant", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := seedEnchanter(t, 1, gameservertest.WithSkillTrees(enchantTree(101, 100)))
		startInWorld(t, c)
		f := selectTrainer(t, srv, c, objID, 10)

		c.Send(encodeRequestExEnchantSkill(1, 101))
		frames := readUntilQuiet(t, c)
		last := frames[len(frames)-1]
		assertNpcHTML(t, last, f.ObjectID(), "<html><body>My html is missing:<br>data/html/trainer/30010-noskills.htm</body></html>")
		if prev := frames[len(frames)-2]; prev[0] != serverpackets.OpcodeUserInfo {
			t.Fatalf("frame before the no-skills page = %#x, want UserInfo", prev[0])
		}
		assertKnownSkills(t, srv, objID, map[int]int{1: 101})
	})
}

// TestTrainerSkillListBypass pins npc_<id>_SkillList: the learnable list
// then two ActionFailed (the list's, then the dispatcher's); with nothing
// to learn yet, the level of the next skill and AcquireSkillDone instead;
// at a trainer of another profession its no-skills page and the
// dispatcher's ActionFailed alone.
func TestTrainerSkillListBypass(t *testing.T) {
	t.Parallel()
	t.Run("list", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t, append(generalLearnOpts(t, 50), gameservertest.WithHTMLPages(trainerPages()), noBypassReuse)...)
		startInWorld(t, c)
		f := selectTrainer(t, srv, c, objID, 0)

		frames := trainerBypass(t, c, f, "SkillList")
		if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeAcquireSkillList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}; string(got) != string(want) {
			t.Fatalf("SkillList = %x, want %x", got, want)
		}
		r := wireReader(frames[0][1:])
		if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeUsual) || count != 1 {
			t.Fatalf("AcquireSkillList = type %d count %d, want usual with 1 entry", skillType, count)
		}
		if id, level, shownLevel, cost, unknown := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != 3 || level != 1 || shownLevel != 1 || cost != 50 || unknown != 0 {
			t.Fatalf("AcquireSkillList entry = %d/%d/%d/%d/%d, want 3/1/1/50/0", id, level, shownLevel, cost, unknown)
		}
	})
	t.Run("nothing yet", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t,
			gameservertest.WithCharacter("Newbie", 1, 0),
			gameservertest.WithWantChars(1),
			gameservertest.WithSkills(learnerTable(t, modelskill.Definition{ID: 3, Level: 1, Activation: modelskill.ActivationActive})),
			gameservertest.WithHTMLPages(trainerPages()), noBypassReuse)
		startInWorld(t, c)
		f := selectTrainer(t, srv, c, objID, 0)

		frames := trainerBypass(t, c, f, "SkillList")
		if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeAcquireSkillDone, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}; string(got) != string(want) {
			t.Fatalf("SkillList = %x, want %x", got, want)
		}
		assertNumberSystemMessage(t, frames[0], serverpackets.SystemMessageDoNotHaveFurtherSkillsToLearnS1, 5)
	})
	t.Run("other profession", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t, append(generalLearnOpts(t, 50), gameservertest.WithHTMLPages(trainerPages()), noBypassReuse)...)
		startInWorld(t, c)
		f := selectTrainer(t, srv, c, objID, 10)

		frames := trainerBypass(t, c, f, "SkillList")
		if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}; string(got) != string(want) {
			t.Fatalf("SkillList = %x, want %x", got, want)
		}
		assertNpcHTML(t, frames[0], f.ObjectID(), noSkillsPage+"\n")
	})
}

// TestTrainerEnchantSkillListBypass pins npc_<id>_EnchantSkillList and the
// list resent after an enchant roll: the enchantable list (id, level, SP,
// exp per row) then two ActionFailed; a talker short of a third
// profession gets a page saying so; with nothing to enchant below level
// 74, the level-74 notice and AcquireSkillDone.
func TestTrainerEnchantSkillListBypass(t *testing.T) {
	t.Parallel()
	t.Run("list and resend after a roll", func(t *testing.T) {
		t.Parallel()
		tree := &modelskill.Trees{Enchant: []modelskill.EnchantSkill{
			{ID: 1, Level: 101, SP: 1000, Exp: 2000, Rate76: 100},
			{ID: 1, Level: 102, SP: 3000, Exp: 4000, Rate76: 100},
		}}
		srv, c, objID := seedEnchanter(t, 1, gameservertest.WithSkillTrees(tree), gameservertest.WithHTMLPages(trainerPages()), noBypassReuse)
		startInWorld(t, c)
		f := selectTrainer(t, srv, c, objID, duelistParentClass)

		frames := trainerBypass(t, c, f, "EnchantSkillList")
		if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeExtended, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}; string(got) != string(want) {
			t.Fatalf("EnchantSkillList = %x, want %x", got, want)
		}
		assertEnchantSkillList(t, frames[0], enchantRow{1, 101, 1000, 2000})

		c.Send(encodeRequestExEnchantSkill(1, 101))
		frames = readUntilQuiet(t, c)
		n := len(frames)
		if n < 3 || frames[n-3][0] != serverpackets.OpcodeUserInfo || frames[n-1][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("enchant = %x, want ... UserInfo, ExEnchantSkillList, ActionFailed", frameOpcodes(frames))
		}
		assertEnchantSkillList(t, frames[n-2], enchantRow{1, 102, 3000, 4000})
		assertKnownSkills(t, srv, objID, map[int]int{1: 101})
	})
	t.Run("short of a third profession", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := bootLearner(t, append(generalLearnOpts(t, 50), gameservertest.WithHTMLPages(trainerPages()), noBypassReuse)...)
		startInWorld(t, c)
		f := selectTrainer(t, srv, c, objID, 0)

		frames := trainerBypass(t, c, f, "EnchantSkillList")
		if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}; string(got) != string(want) {
			t.Fatalf("EnchantSkillList = %x, want %x", got, want)
		}
		assertNpcHTML(t, frames[0], f.ObjectID(), "<html><body> You must have 3rd class change quest completed.</body></html>")
	})
	t.Run("nothing to enchant below 74", func(t *testing.T) {
		t.Parallel()
		srv, c, objID := seedEnchanterAt(t, 70, 1, gameservertest.WithHTMLPages(trainerPages()), noBypassReuse)
		startInWorld(t, c)
		f := selectTrainer(t, srv, c, objID, duelistParentClass)

		frames := trainerBypass(t, c, f, "EnchantSkillList")
		want := []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeAcquireSkillDone, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
		if got := frameOpcodes(frames); string(got) != string(want) {
			t.Fatalf("EnchantSkillList = %x, want %x", got, want)
		}
		assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageThereIsNoSkillThatEnablesEnchant)
		assertNumberSystemMessage(t, frames[1], serverpackets.SystemMessageDoNotHaveFurtherSkillsToLearnS1, 74)
	})
}

// TestTrainerFishSkillListBypass pins a fisherman's npc_<id>_FishSkillList:
// the learnable fishing list then two ActionFailed; with nothing to learn
// yet, the level of the next fishing skill and AcquireSkillDone.
func TestTrainerFishSkillListBypass(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		level int
		want  []byte
	}{
		{"list", 5, []byte{serverpackets.OpcodeAcquireSkillList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}},
		{"nothing yet", 1, []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeAcquireSkillDone, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID := bootLearner(t,
				gameservertest.WithCharacter("Newbie", tc.level, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(learnerTable(t, modelskill.Definition{ID: 1368, Level: 1, Activation: modelskill.ActivationActive})),
				gameservertest.WithSkillTrees(fishingTrees()),
				gameservertest.WithHTMLPages(trainerPages()), noBypassReuse)
			startInWorld(t, c)
			f := spawnTrainer(t, srv, c, objID, "Fisherman", 50)
			selectNpc(t, srv, c, objID, f)

			frames := trainerBypass(t, c, f, "FishSkillList")
			if got := frameOpcodes(frames); string(got) != string(tc.want) {
				t.Fatalf("FishSkillList = %x, want %x", got, tc.want)
			}
			if tc.want[0] == serverpackets.OpcodeSystemMessage {
				assertNumberSystemMessage(t, frames[0], serverpackets.SystemMessageDoNotHaveFurtherSkillsToLearnS1, 5)
				return
			}
			r := wireReader(frames[0][1:])
			if skillType, count := r.ReadInt32(), r.ReadInt32(); skillType != int32(serverpackets.AcquireSkillTypeFishing) || count != 1 {
				t.Fatalf("AcquireSkillList = type %d count %d, want fishing with 1 entry", skillType, count)
			}
		})
	}
}

// enchantRow is one decoded ExEnchantSkillList row.
type enchantRow struct {
	id, level, sp int32
	exp           int64
}

// assertEnchantSkillList asserts frame is an ExEnchantSkillList carrying
// exactly rows, in order.
func assertEnchantSkillList(t *testing.T, frame []byte, rows ...enchantRow) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeExtended, "ExEnchantSkillList")
	r := wireReader(frame[1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExEnchantSkillList {
		t.Fatalf("extended opcode = %#x, want ExEnchantSkillList (%#x)", sub, serverpackets.OpcodeExEnchantSkillList)
	}
	if count := r.ReadInt32(); count != int32(len(rows)) {
		t.Fatalf("ExEnchantSkillList count = %d, want %d", count, len(rows))
	}
	for _, want := range rows {
		got := enchantRow{r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt64()}
		if got != want {
			t.Fatalf("ExEnchantSkillList row = %+v, want %+v", got, want)
		}
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read ExEnchantSkillList: %v", err)
	}
}
