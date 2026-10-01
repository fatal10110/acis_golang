package clan

import (
	"context"
	"database/sql"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Clan Vitality, the clan skill these tests learn: from clan level 5, 500
// reputation and one item 8166 per level, held from clan rank 2, maxHp
// x1.03 at level 1 (clanSkills.xml, skills 0300-0399.xml).
const (
	vitalityID   = 370
	vitalityItem = 8166
	vitalityCost = 500
)

const (
	extendedOpcode  = serverpackets.OpcodeExtended
	acquireSkillReq = 2 // the clan AcquireSkillType
)

var shippedSkills = struct {
	once  sync.Once
	defs  *modelskill.Table
	trees *modelskill.Trees
	err   error
}{}

// shippedSkillData loads the datapack's skill definitions and trees once.
func shippedSkillData(t *testing.T) (*modelskill.Table, *modelskill.Trees) {
	t.Helper()
	skillsDir := datapack.Path(t, "data", "xml", "skills")
	treesDir := datapack.Path(t, "data", "xml", "skillstrees")
	shippedSkills.once.Do(func() {
		shippedSkills.defs, shippedSkills.err = gamexml.LoadSkillDefinitions(skillsDir, zerolog.Nop())
		if shippedSkills.err == nil {
			shippedSkills.trees, shippedSkills.err = gamexml.LoadSkillTrees(treesDir)
		}
	})
	if shippedSkills.err != nil {
		t.Fatalf("load shipped skills: %v", shippedSkills.err)
	}
	return shippedSkills.defs, shippedSkills.trees
}

// clanSkillOptions boots the shipped skill data and the clan skill items.
func clanSkillOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	defs, trees := shippedSkillData(t)
	db := sqltest.SharedDB(t)
	templates := gameservertest.ItemTemplates().All()
	templates = append(templates, &item.Template{
		ID: vitalityItem, Name: "Clan Skill Item", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
	})
	return []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), defs, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithSkillTrees(trees),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
	}
}

// seedClanSkillRow stores s and a clan_skills row of it.
func seedClanSkillRow(t *testing.T, s clanSeed, id, level int) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		seedClanRows(t, s, db)
		if _, err := db.ExecContext(context.Background(), `INSERT INTO clan_skills (clan_id, skill_id, skill_level) VALUES (?,?,?)`, seededClanID, id, level); err != nil {
			t.Fatalf("seed clan skill: %v", err)
		}
	})
}

func encodeRequestAcquireSkillInfo(id, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkillInfo)
	w.WriteInt32(id)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

func encodeRequestAcquireSkill(id, level, skillType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAcquireSkill)
	w.WriteInt32(id)
	w.WriteInt32(level)
	w.WriteInt32(skillType)
	return w.Bytes()
}

// extendedSub is the sub-opcode of an extended frame, -1 for any other.
func extendedSub(frame []byte) int {
	if len(frame) < 3 || frame[0] != extendedOpcode {
		return -1
	}
	return int(frame[1]) | int(frame[2])<<8
}

func isPledgeSkillList(frame []byte) bool {
	return extendedSub(frame) == int(serverpackets.OpcodeExPledgeSkillList)
}

// pledgeSkills decodes a PledgeSkillList frame into id/level pairs.
func pledgeSkills(t *testing.T, frame []byte) [][2]int32 {
	t.Helper()
	if !isPledgeSkillList(frame) {
		t.Fatalf("frame %x is not PledgeSkillList", frame)
	}
	r := wire.NewReader(frame[3:])
	n := r.ReadInt32()
	out := make([][2]int32, 0, n)
	for range n {
		out = append(out, [2]int32{r.ReadInt32(), r.ReadInt32()})
	}
	return out
}

// skillListLevel returns the level skill id has in a SkillList frame, 0
// when it is not listed.
func skillListLevel(t *testing.T, frame []byte, id int32) int32 {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("opcode = %#x, want SkillList", frame[0])
	}
	r := wire.NewReader(frame[1:])
	n := r.ReadInt32()
	for range n {
		r.ReadInt32() // passive
		level, sk := r.ReadInt32(), r.ReadInt32()
		r.ReadUint8() // disabled
		if sk == id {
			return level
		}
	}
	return 0
}

// clanSkillView is the clan-skill-relevant frames, in order: system
// messages by id, other frames by name.
func clanSkillView(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodeSystemMessage:
			id, _ := sysMsg(t, f)
			out = append(out, "sm"+itoa(int32(id)))
		case f[0] == serverpackets.OpcodeSkillList:
			out = append(out, "SkillList")
		case f[0] == serverpackets.OpcodePledgeShowInfoUpdate:
			out = append(out, "PledgeShowInfoUpdate")
		case f[0] == serverpackets.OpcodeAcquireSkillList:
			out = append(out, "AcquireSkillList")
		case f[0] == serverpackets.OpcodeNpcHtmlMessage:
			out = append(out, "NpcHtmlMessage")
		case f[0] == serverpackets.OpcodeActionFailed:
			out = append(out, "ActionFailed")
		case extendedSub(f) == int(serverpackets.OpcodeExPledgeSkillListAdd):
			out = append(out, "PledgeSkillListAdd")
		case isPledgeSkillList(f):
			out = append(out, "PledgeSkillList")
		}
	}
	return out
}

// TestLearnClanSkill has the leader of a level 5 clan with 1000 reputation
// learn Clan Vitality at a village master. The skill list offers it at its
// reputation cost; its info names the cost and the item. Learning it takes
// the item, then the reputation (every member sees the clan's header), and
// gives the skill to each member whose rank reaches it, with its skill list
// and the clan skill list's addition naming it; the list then offers level
// 2. The clan_skills row and the reputation are stored, and the next login
// burst lists the skill.
func TestLearnClanSkill(t *testing.T) {
	opts := append(clanSkillOptions(t), seedClan(t, clanSeed{level: 5, reputation: 1000}))
	w := bootClanWorldCarrying(t, 40, 0, map[int32]int32{vitalityItem: 2}, opts...)
	w.recruit(t)
	w.talkToMaster(t)
	leaderHP := w.srv.PlayerMaxHP(t, w.leaderID)

	frames := w.masterCommand(t, "learn_clan_skills")
	list, ok := firstOpcode(frames, serverpackets.OpcodeAcquireSkillList)
	if !ok {
		t.Fatalf("learn_clan_skills = %v, want AcquireSkillList", clanSkillView(t, frames))
	}
	r := wire.NewReader(list[1:])
	if kind, n := r.ReadInt32(), r.ReadInt32(); kind != acquireSkillReq || n < 1 {
		t.Fatalf("AcquireSkillList type %d with %d rows, want clan rows", kind, n)
	}
	found := false
	for {
		id, level, maxLevel, cost, unk := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if r.Err() != nil {
			break
		}
		if id == vitalityID {
			found = level == 1 && maxLevel == 1 && cost == vitalityCost && unk == 0
			break
		}
	}
	if !found {
		t.Fatal("clan skill list does not offer Clan Vitality 1 at 500 reputation")
	}

	w.leader.Send(encodeRequestAcquireSkillInfo(vitalityID, 1, acquireSkillReq))
	info := drainFrames(t, w.leader)
	if len(info) != 1 || info[0][0] != serverpackets.OpcodeAcquireSkillInfo {
		t.Fatalf("skill info = %x, want one AcquireSkillInfo", opcodes(info))
	}
	r = wire.NewReader(info[0][1:])
	got := []int32{r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()}
	if want := []int32{vitalityID, 1, vitalityCost, acquireSkillReq, 1, 1, vitalityItem, 1, 0}; !slices.Equal(got, want) {
		t.Fatalf("AcquireSkillInfo = %v, want %v", got, want)
	}

	w.leader.Send(encodeRequestAcquireSkill(vitalityID, 1, acquireSkillReq))
	frames = drainFrames(t, w.leader)
	want := []string{
		"sm" + itoa(serverpackets.SystemMessageS1Disappeared),
		"PledgeShowInfoUpdate",
		"sm" + itoa(serverpackets.SystemMessageS1DeductedFromClanRep),
		"SkillList", "PledgeSkillListAdd",
		"sm" + itoa(serverpackets.SystemMessageClanSkillS1Added),
		"AcquireSkillList", "ActionFailed",
	}
	if view := clanSkillView(t, frames); !slices.Equal(view, want) {
		t.Fatalf("leader's learn = %v, want %v", view, want)
	}
	for _, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodeSkillList:
			if level := skillListLevel(t, f, vitalityID); level != 1 {
				t.Fatalf("leader's SkillList has Clan Vitality at %d, want 1", level)
			}
		case extendedSub(f) == int(serverpackets.OpcodeExPledgeSkillListAdd):
			r := wire.NewReader(f[3:])
			if id, level := r.ReadInt32(), r.ReadInt32(); id != vitalityID || level != 1 {
				t.Fatalf("PledgeSkillListAdd = %d/%d, want %d/1", id, level, vitalityID)
			}
		case f[0] == serverpackets.OpcodeSystemMessage:
			if id, params := sysMsg(t, f); id == serverpackets.SystemMessageS1DeductedFromClanRep && !slices.Equal(params, []string{"500"}) {
				t.Fatalf("deduction = %v, want 500", params)
			}
		}
	}
	if hp := w.srv.PlayerMaxHP(t, w.leaderID); hp <= leaderHP {
		t.Fatalf("leader's max HP = %d after Clan Vitality, was %d", hp, leaderHP)
	}
	// The recruit, a rank 2 member of a level 5 clan, is given it too.
	member := drainFrames(t, w.member)
	if view := clanSkillView(t, member); !slices.Equal(view, []string{
		"PledgeShowInfoUpdate", "SkillList", "PledgeSkillListAdd", "sm" + itoa(serverpackets.SystemMessageClanSkillS1Added),
	}) {
		t.Fatalf("recruit's view = %v", view)
	}

	w.srv.FlushPersistence(t)
	if level := queryInt(t, w, `SELECT skill_level FROM clan_skills WHERE clan_id = ? AND skill_id = ?`, seededClanID, vitalityID); level != 1 {
		t.Fatalf("stored clan skill level = %d, want 1", level)
	}
	if _, rep := storedClan(t, w); rep != 500 {
		t.Fatalf("stored reputation = %d, want 500", rep)
	}
	if n := queryInt(t, w, `SELECT COUNT(*) FROM character_skills WHERE skill_id = ?`, vitalityID); n != 0 {
		t.Fatalf("%d character_skills rows store the clan skill, want none", n)
	}

	w.leaveWorld(t, w.member)
	drainFrames(t, w.leader)
	burst := startInWorld(t, w.srv.DialClient(t, "player2", 1))
	for _, f := range burst {
		if isPledgeSkillList(f) {
			if got := pledgeSkills(t, f); !slices.Equal(got, [][2]int32{{vitalityID, 1}}) {
				t.Fatalf("login PledgeSkillList = %v, want Clan Vitality 1", got)
			}
		}
		if f[0] == serverpackets.OpcodeSkillList {
			if level := skillListLevel(t, f, vitalityID); level != 1 {
				t.Fatalf("login SkillList has Clan Vitality at %d, want 1", level)
			}
		}
	}
}

// TestClanSkillRefusals covers the refusals: a clan short of reputation
// is refused before any item is taken; a leader lacking the item is told
// so; both are shown the list again. A member who does not lead asks for
// the list and gets the refusal page, and its info and learn requests
// answer nothing. Nothing is learnt.
func TestClanSkillRefusals(t *testing.T) {
	t.Run("reputation short", func(t *testing.T) {
		opts := append(clanSkillOptions(t), seedClan(t, clanSeed{level: 5, reputation: vitalityCost - 1}))
		w := bootClanWorldCarrying(t, 40, 0, map[int32]int32{vitalityItem: 1}, opts...)
		w.talkToMaster(t)
		w.leader.Send(encodeRequestAcquireSkill(vitalityID, 1, acquireSkillReq))
		if view := clanSkillView(t, drainFrames(t, w.leader)); !slices.Equal(view, []string{
			"sm" + itoa(serverpackets.SystemMessageAcquireSkillFailedBadClanRepScore), "AcquireSkillList", "ActionFailed",
		}) {
			t.Fatalf("learn short of reputation = %v", view)
		}
		if n := w.srv.PlayerInventory(t, w.leaderID).ItemCount(vitalityItem, -1, false); n != 1 {
			t.Fatalf("leader carries %d of the item, want 1", n)
		}
		assertNoClanSkills(t, w)
	})

	t.Run("item missing", func(t *testing.T) {
		opts := append(clanSkillOptions(t), seedClan(t, clanSeed{level: 5, reputation: 1000}))
		w := bootClanWorldCarrying(t, 40, 0, nil, opts...)
		w.talkToMaster(t)
		w.leader.Send(encodeRequestAcquireSkill(vitalityID, 1, acquireSkillReq))
		if view := clanSkillView(t, drainFrames(t, w.leader)); !slices.Equal(view, []string{
			"sm" + itoa(serverpackets.SystemMessageNotEnoughItems),
			"sm" + itoa(serverpackets.SystemMessageItemMissingToLearnSkill), "AcquireSkillList", "ActionFailed",
		}) {
			t.Fatalf("learn without the item = %v", view)
		}
		if _, rep := storedClan(t, w); rep != 1000 {
			t.Fatalf("stored reputation = %d, want 1000", rep)
		}
		assertNoClanSkills(t, w)
	})

	t.Run("not the leader", func(t *testing.T) {
		opts := append(clanSkillOptions(t), seedClan(t, clanSeed{level: 5, reputation: 1000}))
		w := bootClanWorld(t, 40, 0, 0, opts...)
		w.recruit(t)
		w.member.Send(encodeAction(w.master.ObjectID(), w.at))
		drainFrames(t, w.member)
		w.member.Send(encodeAction(w.master.ObjectID(), w.at))
		drainFrames(t, w.member)
		w.member.Send(encodeBypass("npc_" + itoa(w.master.ObjectID()) + "_learn_clan_skills"))
		frames := drainFrames(t, w.member)
		if view := clanSkillView(t, frames); !slices.Equal(view, []string{"NpcHtmlMessage", "ActionFailed", "ActionFailed"}) {
			t.Fatalf("member's learn_clan_skills = %v, want the refusal page", view)
		}
		r := wire.NewReader(frames[0][1:])
		if id := r.ReadInt32(); id != 0 {
			t.Fatalf("refusal page from object %d, want 0", id)
		}
		w.member.Send(encodeRequestAcquireSkillInfo(vitalityID, 1, acquireSkillReq))
		w.member.Send(encodeRequestAcquireSkill(vitalityID, 1, acquireSkillReq))
		if got := drainFrames(t, w.member); len(got) != 0 {
			t.Fatalf("member's info and learn = %x, want nothing", opcodes(got))
		}
		assertNoClanSkills(t, w)
	})
}

// assertNoClanSkills checks no clan skill was stored.
func assertNoClanSkills(t *testing.T, w *clanWorld) {
	t.Helper()
	w.srv.FlushPersistence(t)
	if n := queryInt(t, w, `SELECT COUNT(*) FROM clan_skills`); n != 0 {
		t.Fatalf("%d clan skills stored, want none", n)
	}
}

// TestLearnClanSkillTakesReputationToZero learns Clan Vitality with exactly
// its price: every member first hears the clan skills are off, with its
// skill list, and the clan's header; the leader is told the deduction;
// then each sees the addition without being given the skill.
func TestLearnClanSkillTakesReputationToZero(t *testing.T) {
	opts := append(clanSkillOptions(t), seedClan(t, clanSeed{level: 5, reputation: vitalityCost}))
	w := bootClanWorldCarrying(t, 40, 0, map[int32]int32{vitalityItem: 1}, opts...)
	w.recruit(t)
	w.talkToMaster(t)

	w.leader.Send(encodeRequestAcquireSkill(vitalityID, 1, acquireSkillReq))
	frames := drainFrames(t, w.leader)
	off, added := "sm"+itoa(serverpackets.SystemMessageReputationLowClanSkillsDeactivated), "sm"+itoa(serverpackets.SystemMessageClanSkillS1Added)
	if view := clanSkillView(t, frames); !slices.Equal(view, []string{
		"sm" + itoa(serverpackets.SystemMessageS1Disappeared), off, "SkillList", "PledgeShowInfoUpdate",
		"sm" + itoa(serverpackets.SystemMessageS1DeductedFromClanRep), "PledgeSkillListAdd", added,
		// The list still offers what a level 5 clan can learn next.
		"AcquireSkillList", "ActionFailed",
	}) {
		t.Fatalf("leader's learn = %v", view)
	}
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSkillList && skillListLevel(t, f, vitalityID) != 0 {
			t.Fatal("leader was given the skill at 0 reputation")
		}
	}
	if view := clanSkillView(t, drainFrames(t, w.member)); !slices.Equal(view, []string{off, "SkillList", "PledgeShowInfoUpdate", "PledgeSkillListAdd", added}) {
		t.Fatalf("recruit's view = %v", view)
	}
}

// TestClanSkillsFollowMembership has a recruit join a clan that knows Clan
// Vitality: it is not given the skill as it joins, the reference checking
// the rank it held before joining, and no SkillList follows. Its next login
// lists the clan's skills and gives it the skill; leaving the clan takes it
// away with a new skill list.
func TestClanSkillsFollowMembership(t *testing.T) {
	opts := append(clanSkillOptions(t), seedClanSkillRow(t, clanSeed{level: 5, reputation: 1000}, vitalityID, 1))
	w := bootClanWorld(t, 40, 0, 0, opts...)
	recruitHP := w.srv.PlayerMaxHP(t, w.memberID)

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	drainFrames(t, w.member)
	w.member.Send(encodeRequestAnswerJoinPledge(1))
	joined := drainFrames(t, w.member)
	if _, ok := firstOpcode(joined, serverpackets.OpcodeJoinPledge); !ok {
		t.Fatalf("recruit did not join: %x", opcodes(joined))
	}
	if _, ok := firstOpcode(joined, serverpackets.OpcodeSkillList); ok {
		t.Fatal("joining sent a SkillList")
	}
	if hp := w.srv.PlayerMaxHP(t, w.memberID); hp != recruitHP {
		t.Fatalf("recruit's max HP on joining = %d, want %d unchanged", hp, recruitHP)
	}
	drainFrames(t, w.leader)

	w.leaveWorld(t, w.member)
	drainFrames(t, w.leader)
	c := w.srv.DialClient(t, "player2", 1)
	burst := startInWorld(t, c)
	sawList := false
	for _, f := range burst {
		if isPledgeSkillList(f) {
			sawList = true
		}
		if f[0] == serverpackets.OpcodeSkillList && skillListLevel(t, f, vitalityID) != 1 {
			t.Fatal("login SkillList misses Clan Vitality")
		}
	}
	if !sawList {
		t.Fatal("login burst has no PledgeSkillList")
	}
	loggedHP := w.srv.PlayerMaxHP(t, w.memberID)
	if loggedHP <= recruitHP {
		t.Fatalf("recruit's max HP after login = %d, want above %d", loggedHP, recruitHP)
	}

	c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	left := drainFrames(t, c)
	list, ok := firstOpcode(left, serverpackets.OpcodeSkillList)
	if !ok || skillListLevel(t, list, vitalityID) != 0 {
		t.Fatalf("withdrawal = %x, want a SkillList without Clan Vitality", opcodes(left))
	}
	if hp := w.srv.PlayerMaxHP(t, w.memberID); hp != recruitHP {
		t.Fatalf("recruit's max HP after leaving = %d, want %d", hp, recruitHP)
	}
}
