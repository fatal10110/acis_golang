package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestBypassToServer.java:126-131 hands manor_menu_select to
// onBypassFeedback of whatever NPC is targeted, at any distance and from
// any page, and sends nothing after it. The NPC types that answer any
// command before reading it answer this one too: Merchant.java:49 (the
// karma refusal page, then ActionFailed, Npc.showPkDenyChatWindow),
// DungeonGatekeeper.java:22 (ActionFailed), CastleBlacksmith.java:20-28
// (npcdefault.htm while AllowManor is off), Auctioneer.java:41-45
// (NO_CLAN_HALLS_UP_FOR_AUCTION), WarehouseKeeper.java:54-60
// (ALREADY_TRADING to a talker tied up in a trade, else the enchant
// selection cancelled); a wyvern manager (WyvernManagerNpc.java:34-35, 95)
// heeds only its current talker, then runs the chamberlain's manor
// windows; everything else stays silent.

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
	t.Run("warehouse keeper to a talker with a trade request pending", func(t *testing.T) {
		t.Parallel()
		w := bootFolkWorld(t, dialogPages(), noBypassReuse)
		otherID := w.srv.SeedCharacterFor(t, "player2", "Other", playerLevel, 0).ID
		other := w.srv.DialClient(t, "player2", 1)
		startInWorld(t, w.srv, other)
		drainUntilQuiet(t, w.c)
		f := w.spawnFolk(t, folkTemplate("WarehouseKeeper", 30005), 50)
		drainUntilQuiet(t, other)
		w.selectFolk(t, f)
		w.c.Send(encodeTradeRequest(otherID))
		drainUntilQuiet(t, w.c)
		drainUntilQuiet(t, other)
		frames := w.bypass(t, manorMenuCommand)
		assertAnswer(t, frames, []byte{serverpackets.OpcodeSystemMessage}, f, "")
		wantMessages(t, "manor_menu_select", frames, serverpackets.SystemMessageAlreadyTrading)
	})
	t.Run("warehouse keeper to a talker choosing an item to enchant", func(t *testing.T) {
		t.Parallel()
		srv := gameservertest.Boot(t,
			gameservertest.WithCharacter("Talker", playerLevel, 0), gameservertest.WithWantChars(1),
			gameservertest.WithHTMLPages(dialogPages()), noBypassReuse)
		w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
		scroll := srv.GiveItem(t, w.player, scrollID, 1)
		startInWorld(t, srv, w.c)
		x, y, z := srv.PlayerPosition(t, w.player)
		w.at.X, w.at.Y, w.at.Z = x, y, z
		f := w.spawnFolk(t, folkTemplate("WarehouseKeeper", 30005), 50)
		w.selectFolk(t, f)
		w.c.Send(encodeUseItem(scroll))
		drainUntilQuiet(t, w.c)
		frames := w.bypass(t, manorMenuCommand)
		assertAnswer(t, frames, []byte{serverpackets.OpcodeEnchantResult, serverpackets.OpcodeSystemMessage}, f, "")
		if got := wire.NewReader(frames[0][1:]).ReadInt32(); got != int32(serverpackets.EnchantResultCancelled) {
			t.Fatalf("EnchantResult = %d, want CANCELLED", got)
		}
		// The selection is gone: a second command cancels nothing.
		if got := w.bypass(t, manorMenuCommand); len(got) != 0 {
			t.Fatalf("second answer = % x, want nothing", opcodes(got))
		}
	})
	t.Run("wyvern manager", func(t *testing.T) {
		t.Parallel()
		cfg := castlemanor.DefaultConfig()
		cfg.SavePeriod = 0
		w := bootResidenceCommands(t, commandSetup{
			residenceSetup: residenceSetup{rank: 5, privs: privCastleManor},
			extra: []gameservertest.Option{
				gameservertest.WithManor(network.ManorConfig{Allowed: true, CropRate: 1, Seeds: manorSetupSeeds()}),
				gameservertest.WithCastleManor(cfg, manorSetupStart),
			},
		})
		w.spawnKeeper(t, "WyvernManagerNpc", wyvernManagerID, 30)
		frames := w.bypass(t, "manor_menu_select?ask=5&state=-1&time=0")
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeExtended {
			t.Fatalf("manor window = % x, want ExShowManorDefaultInfo alone", opcodes(frames))
		}
		if sub := wire.NewReader(frames[0][1:]).ReadUint16(); sub != 0x1e {
			t.Fatalf("manor window = %#x, want ExShowManorDefaultInfo", sub)
		}
		// The current-talker check cannot turn this route away: selecting
		// a civilian NPC makes it the talker's current one, so the wyvern
		// manager targeted is always its current NPC.
		if got := w.bypass(t, "manor_menu_select?ask=7&state=-1&time=0"); len(got) != 1 || got[0][0] != serverpackets.OpcodeExtended {
			t.Fatalf("seed setting window = % x, want it alone", opcodes(got))
		}
	})
}
