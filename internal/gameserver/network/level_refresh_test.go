package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// levelRefreshTable is a three-level table, so RealMaxLevel is 2 and a single
// level-up from 1 is legal.
func levelRefreshTable(t *testing.T) *player.LevelTable {
	t.Helper()
	table, err := player.NewLevelTable(map[int]player.Level{
		1: {RequiredExpToLevelUp: 0},
		2: {RequiredExpToLevelUp: 68},
		3: {RequiredExpToLevelUp: 363},
	})
	if err != nil {
		t.Fatalf("build level table: %v", err)
	}
	return table
}

// TestRefreshLiveLevelSkillsReconcilesAndSendsSkillList pins what the level
// refresher owes the client. The shared test profession grants skill 3 at
// level 1 only, so a character holding it at level 3 holds it above what the
// profession supports: the refresh pulls it back down and resends the list.
func TestRefreshLiveLevelSkillsReconcilesAndSendsSkillList(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetSkillLevel(3, 3)

	gcl := &GameClientLink{skills: skillstate.NewPersistence(nil, skillTable(
		modelskill.Definition{ID: 3, Level: 1},
		modelskill.Definition{ID: 3, Level: 2},
		modelskill.Definition{ID: 3, Level: 3},
	))}
	gcl.refreshLiveLevelSkills(context.Background(), live)

	if got := live.Character.SkillLevel(3); got != 1 {
		t.Errorf("SkillLevel(3) after refresh = %d, want 1 (pulled back to the granted level)", got)
	}

	sent := frames.frames
	if len(sent) != 1 {
		t.Fatalf("frames sent = %d, want 1", len(sent))
	}
	if sent[0][0] != serverpackets.OpcodeSkillList {
		t.Fatalf("frame opcode = %#x, want SkillList (%#x)", sent[0][0], serverpackets.OpcodeSkillList)
	}
	r := wire.NewReader(sent[0][1:])
	if count := r.ReadInt32(); count != 1 {
		t.Fatalf("SkillList count = %d, want 1", count)
	}
	if passive, level, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); level != 1 || id != 3 {
		t.Fatalf("SkillList entry = passive %d level %d id %d, want level 1 id 3", passive, level, id)
	}
}

func TestRefreshLiveLevelSkillsAutoLearnsBoughtSkills(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.CharLevel = 10
	live.template = &player.Template{Skills: []player.SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 249, Level: 1, MinLevel: 5, Cost: 0},
	}}
	gcl := &GameClientLink{
		skills: skillstate.NewPersistence(nil, skillTable(
			modelskill.Definition{ID: 3, Level: 1},
			modelskill.Definition{ID: 249, Level: 1},
		)),
		playerConfig: PlayerConfig{AutoLearnSkills: true},
	}

	gcl.refreshLiveLevelSkills(context.Background(), live)

	if got := live.Character.SkillLevel(3); got != 1 {
		t.Errorf("SkillLevel(3) = %d, want 1 (bought grant auto-learned)", got)
	}
	if got := live.Character.SkillLevel(249); got != 1 {
		t.Errorf("SkillLevel(249) = %d, want 1 (free grant auto-learned)", got)
	}
}

// TestRefreshLiveLevelSkillsWithoutSkillsIsSilent pins the guard: a link with
// no skill persistence attached has nothing to reconcile and sends nothing,
// rather than pushing an empty skill list that would wipe the client's copy.
func TestRefreshLiveLevelSkillsWithoutSkillsIsSilent(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SetSkillLevel(3, 3)

	(&GameClientLink{}).refreshLiveLevelSkills(context.Background(), live)

	if sent := frames.frames; len(sent) != 0 {
		t.Fatalf("frames sent = %d, want 0", len(sent))
	}
	if got := live.Character.SkillLevel(3); got != 3 {
		t.Errorf("SkillLevel(3) = %d, want 3 (untouched)", got)
	}
}

// TestSendExpSpLossFramesOrdersStatusBeforeExpMessage pins the combined
// exp+SP removal order the skill-enchant path hits: StatusUpdate(SP) goes
// out before EXP_DECREASED_BY_S1, matching PlayerStatus.setSp firing
// synchronously inside removeExpAndSp ahead of its own system messages
// (PlayerStatus.java:583-603, PlayableStatus.java:133-145).
func TestSendExpSpLossFramesOrdersStatusBeforeExpMessage(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	live.Character.SP = 100

	sendExpSpLossFrames(live, 10, 25)

	sent := frames.frames
	if len(sent) != 3 {
		t.Fatalf("frames sent = %d, want 3", len(sent))
	}
	if sent[0][0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("frame[0] opcode = %#x, want StatusUpdate (%#x)", sent[0][0], serverpackets.OpcodeStatusUpdate)
	}
	if sent[1][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("frame[1] opcode = %#x, want SystemMessage (%#x)", sent[1][0], serverpackets.OpcodeSystemMessage)
	}
	if sent[2][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("frame[2] opcode = %#x, want SystemMessage (%#x)", sent[2][0], serverpackets.OpcodeSystemMessage)
	}
}

// TestAddLevelRunsTheRegisteredRefresher pins the domain-to-network join: a
// level change is what drives the refresher, in either direction.
func TestAddLevelRunsTheRegisteredRefresher(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	tmpl, ok := testTemplates(t).Get(0)
	if !ok {
		t.Fatal("missing test class template")
	}
	gcl := &GameClientLink{skills: skillstate.NewPersistence(nil, skillTable(
		modelskill.Definition{ID: 3, Level: 1},
	))}
	live.Character.SetLevelRefresher(func() { gcl.refreshLiveLevelSkills(context.Background(), live) })

	table := levelRefreshTable(t)
	if !live.Character.AddLevel(table, tmpl, 1) {
		t.Fatal("AddLevel(1) did not report an increase")
	}
	// A drop reports false by design; it must still refresh.
	live.Character.AddLevel(table, tmpl, -1)

	// One SkillList per level change, up and down alike.
	sent := frames.frames
	skillLists := 0
	for _, frame := range sent {
		if frame[0] == serverpackets.OpcodeSkillList {
			skillLists++
		}
	}
	if skillLists != 2 {
		t.Fatalf("SkillList frames = %d, want 2 (one per level change)", skillLists)
	}
}
