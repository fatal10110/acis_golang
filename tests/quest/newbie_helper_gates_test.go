package quest

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The class ids the gates below switch the player to.
const (
	humanFighterClass = 0
	gladiatorClass    = 2
)

// setClass makes the player play classID.
func (w *newbieWorld) setClass(t *testing.T, classID int) {
	t.Helper()
	onCharacter(t, w.srv, w.player, func(c *player.Character) { c.SetClassID(classID) })
}

// TestNewbieTokenTeleportList: a newbie asking a guide for the travel
// token opens the guide's NEWBIE_TOKEN rows of teleports.xml, each priced
// one travel token; its first row takes the player to the Dark Elf Village
// for a token.
func TestNewbieTokenTeleportList(t *testing.T) {
	t.Parallel()
	teleports, err := gamexml.LoadTeleports(datapack.Path(t, "data", "xml", "teleports.xml"))
	if err != nil {
		t.Fatalf("load teleports.xml: %v", err)
	}
	w := bootNewbieHelper(t, 1, 0, nil, gameservertest.WithTeleports(teleports, nil, false, nil))
	guide := w.spawnGuide(t, cnacelotID, 20, 0)
	w.talk(t, guide.ObjectID())

	tokenList := w.event(t, "NewbieToken")
	first := fmt.Sprintf(`bypass -h npc_%d_teleport 0`, guide.ObjectID())
	if len(tokenList) != 1 || !strings.Contains(tokenList[0], first) || !strings.Contains(tokenList[0], "Dark Elf Village") ||
		strings.Contains(tokenList[0], "Gludin") {
		t.Fatalf("newbie token list = %q, want the guide's newbie token teleports", tokenList)
	}
	w.srv.Client.Send(encodeBypass(fmt.Sprintf("npc_%d_teleport 0", guide.ObjectID())))
	w.srv.AdvanceUntil(t, "token spent", func() bool { return w.srv.PlayerItemCount(t, w.player, travelToken) == 11 })
	if x, y, _ := w.srv.PlayerPosition(t, w.player); abs(x-9716) > 100 || abs(y-15502) > 100 {
		t.Fatalf("player at %d,%d after the token teleport, want near 9716,15502", x, y)
	}
}

// TestNewbieGuideGatesBeyondNewbie: past level 25 a guide answers the
// travel token with its invalid page, and its support magic says the
// player is too high for it. The blessing is refused to a second class and
// to a player above level 39.
func TestNewbieGuideGatesBeyondNewbie(t *testing.T) {
	t.Parallel()
	buffs := []modelskill.NewbieBuff{modelskill.NewNewbieBuff(4322, 1, 8, 24, false)}
	w := bootNewbieHelper(t, 1, 0, buffs)
	guide := w.spawnGuide(t, cnacelotID, 20, 0)
	w.talk(t, guide.ObjectID())

	w.srv.AddPlayerLevel(t, w.player, 25)
	w.srv.ReadQueued(t, w.srv.Client)
	expectLines(t, "token past level 25", w.event(t, "NewbieToken"), "page human/guide_human_cnacelot002.htm", "ActionFailed")
	expectLines(t, "support past level 25", w.event(t, "SupportMagic"), "page guide_for_newbie003.htm", "ActionFailed")

	w.setClass(t, gladiatorClass)
	expectLines(t, "blessing to a second class", w.event(t, "GiveBlessing"), "page human/guide_human_cnacelot002.htm", "ActionFailed")
	expectLines(t, "no blessing cast", w.after(t, 2*time.Second))

	w.setClass(t, humanFighterClass)
	w.srv.AddPlayerLevel(t, w.player, 14)
	w.srv.ReadQueued(t, w.srv.Client)
	expectLines(t, "blessing above level 39", w.event(t, "GiveBlessing"), "page human/guide_human_cnacelot002.htm", "ActionFailed")
	expectLines(t, "no blessing cast", w.after(t, 2*time.Second))
}

// TestNewbieSupportMagicDroppedAtLogout: the support magic casts its
// first buff at once and the rest a second apart on the player's queue; a
// player who logs out before the second cast gets none of the rest.
func TestNewbieSupportMagicDroppedAtLogout(t *testing.T) {
	t.Parallel()
	buffs := []modelskill.NewbieBuff{
		modelskill.NewNewbieBuff(4322, 1, 8, 24, false),
		modelskill.NewNewbieBuff(4323, 1, 8, 24, false),
		modelskill.NewNewbieBuff(4324, 1, 8, 24, false),
	}
	w := bootNewbieHelper(t, 8, 0, buffs)
	guide := w.spawnGuide(t, cnacelotID, 20, 0)
	w.talk(t, guide.ObjectID())

	var c *player.Character
	onCharacter(t, w.srv, w.player, func(ch *player.Character) { c = ch })
	expectLines(t, "first buff", w.event(t, "SupportMagic"), "cast 4322-1 player->player")

	w.srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	readUntil(t, w.srv.Client, serverpackets.OpcodeLeaveWorld)
	w.srv.Client.ExpectClosed()
	w.srv.Advance(t, 3*time.Second)

	held := func(id modelskill.ID) bool {
		return slices.ContainsFunc(c.EffectList().All(), func(e *effect.Effect) bool { return e.Skill.ID == id })
	}
	// The first buff is still held: logging out cleared nothing, so the
	// missing ones were never cast.
	if !held(4322) {
		t.Fatal("first buff not held after logout")
	}
	for _, id := range []modelskill.ID{4323, 4324} {
		if held(id) {
			t.Fatalf("buff %d landed after logout", id)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
