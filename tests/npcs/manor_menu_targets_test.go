package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: RequestBypassToServer.java:126-131 hands manor_menu_select to
// onBypassFeedback of whatever NPC is targeted, at any distance and from
// any page, and sends nothing after it. The NPC types that answer any
// command before reading it answer this one too: Merchant.java:49 (the
// karma refusal page, then ActionFailed, Npc.showPkDenyChatWindow),
// DungeonGatekeeper.java:22 (ActionFailed), CastleBlacksmith.java:20-28
// (npcdefault.htm while AllowManor is off), Auctioneer.java:41-45
// (NO_CLAN_HALLS_UP_FOR_AUCTION); everything else stays silent.

const manorMenuCommand = "manor_menu_select?ask=1&state=-1&time=0"

// TestManorMenuOnOtherTargets pins the answers of targets that are no
// manor NPC.
func TestManorMenuOnOtherTargets(t *testing.T) {
	t.Parallel()
	t.Run("merchant to a talker carrying karma", func(t *testing.T) {
		t.Parallel()
		pages := dialogPages()
		pages["merchant/30001-pk.htm"] = "<html><body>No trade %objectId%</body></html>"
		w := bootFolkWorldAs(t, withKarmaCharacter(100), pages, noBypassReuse)
		f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
		w.selectFolk(t, f)
		assertAnswer(t, w.bypass(t, manorMenuCommand), []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, f,
			"<html><body>No trade %objectId%</body></html>\n")
	})
	t.Run("merchant to a talker without karma", func(t *testing.T) {
		t.Parallel()
		w := bootFolkWorld(t, dialogPages(), noBypassReuse)
		f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
		w.selectFolk(t, f)
		if got := w.bypass(t, manorMenuCommand); len(got) != 0 {
			t.Fatalf("answer = % x, want nothing", opcodes(got))
		}
	})
	t.Run("dungeon gatekeeper", func(t *testing.T) {
		t.Parallel()
		w := bootFolkWorld(t, dialogPages(), noBypassReuse)
		f := w.spawnFolk(t, folkTemplate("DungeonGatekeeper", 31095), 50)
		w.selectFolk(t, f)
		assertAnswer(t, w.bypass(t, manorMenuCommand), []byte{serverpackets.OpcodeActionFailed}, f, "")
	})
	t.Run("castle blacksmith with the manor off", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{residenceSetup: residenceSetup{manorOff: true}})
		f := w.spawnKeeper(t, "CastleBlacksmith", castleBlacksmithID, 30)
		assertAnswer(t, w.bypass(t, manorMenuCommand), []byte{serverpackets.OpcodeNpcHtmlMessage}, f, filled("npcdefault.htm", f, "%npcname%", "Keeper"))
	})
	t.Run("castle blacksmith with the manor on", func(t *testing.T) {
		t.Parallel()
		w := bootResidenceCommands(t, commandSetup{})
		w.spawnKeeper(t, "CastleBlacksmith", castleBlacksmithID, 30)
		if got := w.bypass(t, manorMenuCommand); len(got) != 0 {
			t.Fatalf("answer = % x, want nothing", opcodes(got))
		}
	})
	t.Run("auctioneer with nothing for sale", func(t *testing.T) {
		t.Parallel()
		w := bootAuction(t, auctionSetup{clanLevel: 2})
		w.open(t)
		frames := w.folkWorld.bypass(t, manorMenuCommand)
		wantMessages(t, "manor_menu_select", frames, serverpackets.SystemMessageNoClanHallsUpForAuction)
		if len(frames) != 1 {
			t.Fatalf("answer = % x, want the notice alone", opcodes(frames))
		}
	})
}
