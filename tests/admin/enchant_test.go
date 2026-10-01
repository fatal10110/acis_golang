package admin

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The fixture's D-grade sword, rigged with a passive +4 enchant skill.
const (
	swordID        int32 = 30
	enchantSkillID int32 = 3098
)

// bootEnchantRig boots the GM "Admin" and a user "Player", each wearing the
// rigged sword in the right hand, and returns the server with the clients,
// the character ids and the swords' object ids.
func bootEnchantRig(t *testing.T) (srv *gameservertest.Server, player *testsupport.ScriptedClient, gmID, playerID, gmSword, playerSword int32) {
	t.Helper()
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == swordID {
			rigged := *tmpl
			weapon := *tmpl.Weapon
			weapon.Enchant4Skill = &item.SkillRef{ID: enchantSkillID, Level: 1}
			rigged.Weapon = &weapon
			tmpl = &rigged
		}
		templates = append(templates, tmpl)
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: modelskill.ID(enchantSkillID), Level: 1, Activation: modelskill.ActivationPassive},
	}), gamesql.NewCharacterSkillStore(db))
	srv, gmID = bootAdmin(t, adminLevel,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithSkills(skills))
	gmSword = wearSword(t, srv, gmID)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	playerID = ch.ID
	playerSword = wearSword(t, srv, playerID)

	enterWorld(t, srv.Client)
	player = srv.DialClient(t, "player2", 1)
	enterWorld(t, player)
	drain(t, srv.Client)
	return srv, player, gmID, playerID, gmSword, playerSword
}

// wearSword gives ownerID the rigged sword, worn in the right hand, and
// returns its object id.
func wearSword(t *testing.T, srv *gameservertest.Server, ownerID int32) int32 {
	t.Helper()
	objectID := srv.GiveItem(t, ownerID, swordID, 1)
	row := itemRow(t, srv, ownerID, objectID)
	row.Location, row.LocationData = item.LocationPaperdoll, itemcontainer.RHand
	if err := srv.Items.Update(context.Background(), row); err != nil {
		t.Fatalf("wear sword: %v", err)
	}
	return objectID
}

func itemRow(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32) *item.Instance {
	t.Helper()
	rows, err := srv.Items.ListByOwner(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	for _, row := range rows {
		if row.ObjectID == objectID {
			return row
		}
	}
	t.Fatalf("no item row %d for owner %d", objectID, ownerID)
	return nil
}

// skillListHas reports whether a SkillList frame lists skill id.
func skillListHas(frame []byte, id int32) bool {
	r := wire.NewReader(frame[1:])
	for range r.ReadInt32() {
		r.ReadInt32() // passive
		r.ReadInt32() // level
		if r.ReadInt32() == id {
			return true
		}
		r.ReadUint8() // disabled
	}
	return false
}

// TestAdminEnchant pins //enchant (AdminEnchant.java): its argument checks
// in order, the worn item taking the level and its row persisting it, the
// +4 weapon skill granted and revoked as the level crosses +4, the refreshed
// UserInfo, and the GM's report; on a selected player the player is the one
// refreshed.
func TestAdminEnchant(t *testing.T) {
	t.Parallel()
	srv, player, gmID, playerID, gmSword, playerSword := bootEnchantRig(t)
	gm := srv.Client

	assertTexts(t, exchange(t, gm, encodeBuildCmd("enchant")),
		"Usage: //enchant slot enchant",
		"Slots: under|lear|rear|neck|lfinger|rfinger|head|rhand|lhand",
		"Slots: gloves|chest|legs|feet|cloak|face|hair|hairall")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("enchant rhand 1 2")),
		"Usage: //enchant slot enchant",
		"Slots: under|lear|rear|neck|lfinger|rfinger|head|rhand|lhand",
		"Slots: gloves|chest|legs|feet|cloak|face|hair|hairall")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("enchant boots x")), "Unknown paperdoll slot.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("enchant null 1")), "Unknown paperdoll slot.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("enchant RHAND x")), "Please specify a new enchant value.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("enchant rhand 65536")), "You must set the enchant level between 0 - 65535.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("enchant rhand -1")), "You must set the enchant level between 0 - 65535.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("enchant chest 3")), "Admin doesn't wear any item in CHEST slot.")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("enchant rhand 0")), "Admin's Sword enchant is already set to 0.")

	frames := exchange(t, gm, encodeBuildCmd("enchant rhand 4"))
	testsupport.AssertOpcodeSequence(t, frames, serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage)
	if !skillListHas(frames[0], enchantSkillID) {
		t.Fatal("SkillList after //enchant rhand 4 lacks the +4 skill")
	}
	assertTexts(t, frames[2:], "Admin's Sword enchant was modified from 0 to 4.")
	srv.InventoryUpdates.Tick()
	frames = exchange(t, gm, encodeGmList())
	if frames[0][0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("frames after the inventory tick = %x, want the sword's InventoryUpdate first", testsupport.FrameOpcodes(frames))
	}

	frames = exchange(t, gm, encodeBuildCmd("enchant rhand 65535"))
	testsupport.AssertOpcodeSequence(t, frames, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage)
	assertTexts(t, frames[1:], "Admin's Sword enchant was modified from 4 to 65535.")

	frames = exchange(t, gm, encodeBuildCmd("enchant rhand 3"))
	testsupport.AssertOpcodeSequence(t, frames, serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage)
	if skillListHas(frames[0], enchantSkillID) {
		t.Fatal("SkillList after //enchant rhand 3 still lists the +4 skill")
	}
	srv.FlushItems(t)
	if got := itemRow(t, srv, gmID, gmSword).EnchantLevel; got != 3 {
		t.Fatalf("GM sword enchant level row = %d, want 3", got)
	}

	// On a selected player: the player's sword changes and the player is
	// refreshed; the GM gets the report.
	exchange(t, gm, encodeAction(playerID))
	drain(t, player)
	gm.Send(encodeBuildCmd("enchant rhand 7"))
	if got := readText(t, gm); got != "Player's Sword enchant was modified from 0 to 7." {
		t.Fatalf("GM report = %q", got)
	}
	for range 100 {
		if player.Read()[0] == serverpackets.OpcodeUserInfo {
			break
		}
	}
	srv.FlushItems(t)
	if got := itemRow(t, srv, playerID, playerSword).EnchantLevel; got != 7 {
		t.Fatalf("player sword enchant level row = %d, want 7", got)
	}
	if got := itemRow(t, srv, gmID, gmSword).EnchantLevel; got != 3 {
		t.Fatalf("GM sword enchant level row = %d, want 3 still", got)
	}
}

// readText reads c's frames up to the next system message and returns its
// text; a GM watching the selected player also gets its CharInfo.
func readText(t *testing.T, c *testsupport.ScriptedClient) string {
	t.Helper()
	for range 100 {
		if frame := c.Read(); frame[0] == serverpackets.OpcodeSystemMessage {
			_, text := systemText(t, frame)
			return text
		}
	}
	t.Fatal("no system message within 100 frames")
	return ""
}
