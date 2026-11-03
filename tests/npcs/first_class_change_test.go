package npcs

import (
	"context"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/firstclasschange"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The first class change (FirstClassChange) paths the quest scenarios
// cannot seed or compare: a character playing a subclass and a change link
// sent by a character of another class, both in the subclass world, whose
// character is a human Gladiator; and the transfer's packets the scenarios
// do not render (the henna list and the character shown anew).

const (
	// ramosID is Grand Master Ramos, who changes human and elf fighters.
	ramosID  = 30373
	ramosDir = "script/feature/FirstClassChange/"
	// noQuestPage is the page of an NPC with nothing for the player.
	noQuestPage = "<html><body>You are either not on a quest that involves this NPC, or you don't meet this NPC's minimum quest requirements.</body></html>"
)

// firstClassChangeOptions registers the feature on Ramos with its datapack
// pages and the page that admits every command.
func firstClassChangeOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	datapack.Require(t)
	pages := map[string]string{"test/any.htm": anyNpcPage}
	entries, err := os.ReadDir(datapack.Path(t, "data", "html", "script", "feature", "FirstClassChange"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "feature", "FirstClassChange", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		pages[ramosDir+e.Name()] = string(data)
	}
	return []gameservertest.Option{
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{ramosID: script.KindFolk},
			[]script.Listing{{Path: "script.feature.FirstClassChange"}},
			script.Catalog{"script.feature.FirstClassChange": firstclasschange.New}),
	}
}

// askRamos opens Ramos's first class change dialog through a page that
// admits the command, and returns the answer.
func (w *subclassWorld) askRamos(t *testing.T, ramos *npc.Folk) [][]byte {
	t.Helper()
	w.openAnyNpcPage(t)
	return w.bypass(t, "npc_"+strconv.Itoa(int(ramos.ObjectID()))+"_Quest FirstClassChange")
}

// TestFirstClassChangeOnTheBaseClass: the Gladiator, on its second
// occupation, is told by Ramos that it changed profession long ago. The
// Warrior change link, sent anyway, answers the page for a character of
// another class, before the level or the token is looked at.
func TestFirstClassChangeOnTheBaseClass(t *testing.T) {
	t.Parallel()
	w := bootSubclassWorld(t, firstClassChangeOptions(t)...)
	ramos := w.spawnFolk(t, folkTemplate("Folk", ramosID), 60)

	wantPage(t, w.askRamos(t, ramos), ramos, scriptPage(t, ramosDir+"30373-39.htm", ramos), 2)
	w.openAnyNpcPage(t)
	wantPage(t, w.bypass(t, "Quest FirstClassChange HW"), ramos, scriptPage(t, ramosDir+"30373-40.htm", ramos), 1)
}

// TestFirstClassChangeInASubclass: a character playing a subclass gets the
// no-quest page from Ramos, whatever its classes.
func TestFirstClassChangeInASubclass(t *testing.T) {
	t.Parallel()
	w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
		exec(t, srv, `INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, 0, 0, 40, 1)`, objID, spellhowler)
		exec(t, srv, `UPDATE characters SET classid = ? WHERE obj_Id = ?`, spellhowler, objID)
	}, firstClassChangeOptions(t)...)
	ramos := w.spawnFolk(t, folkTemplate("Folk", ramosID), 60)

	wantPage(t, w.askRamos(t, ramos), ramos, noQuestPage, 2)
}

// TestFirstClassChangeTransferFrames: a level 20 human fighter holding the
// medallion of the Warrior changes to a Warrior at Ramos. The answer is, in
// order: the medallion taken and the coupons given, the fanfare, the class
// change's animation and message, the henna list and the character shown
// anew (a StatusUpdate regen tick may interleave), then the congratulation
// page and its release. The Warrior is the character's class and base
// class, in the UserInfo and in its saved row.
func TestFirstClassChangeTransferFrames(t *testing.T) {
	t.Parallel()
	const medallion, coupon = 1145, 8869
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{medallion, coupon} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	warrior := gameservertest.ClassTemplate()
	warrior.ID = 1
	srv := gameservertest.Boot(t, append(firstClassChangeOptions(t),
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithClassTemplates(warrior),
		noBypassReuse,
	)...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	srv.GiveItem(t, w.player, medallion, 1)
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	ramos := w.spawnFolk(t, folkTemplate("Folk", ramosID), 60)

	w.openAnyNpcPage(t)
	wantPage(t, w.bypass(t, npcCommand(ramos, "Quest FirstClassChange")), ramos, scriptPage(t, ramosDir+"30373-08.htm", ramos), 2)
	wantPage(t, w.bypass(t, "Quest FirstClassChange 30373-09.htm"), ramos, scriptPage(t, ramosDir+"30373-09.htm", ramos), 1)
	frames := w.bypass(t, "Quest FirstClassChange HW")
	var got []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound, serverpackets.OpcodeMagicSkillUse,
			serverpackets.OpcodeHennaInfo, serverpackets.OpcodeUserInfo,
			serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed:
			got = append(got, f[0])
		}
	}
	want := []byte{
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound,
		serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeHennaInfo, serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("transfer answer = %x, want %x", got, want)
	}
	if ids := systemMessageIDs(frames); !slices.Equal(ids, []int32{302, 53, 1308}) {
		t.Fatalf("transfer messages = %v, want the medallion taken, the coupons earned and the class transfer", ids)
	}
	html := frames[slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeNpcHtmlMessage })]
	if _, page, _ := htmlMessage(t, html); page != scriptPage(t, ramosDir+"30373-29.htm", ramos) {
		t.Fatalf("transfer page = %q, want 30373-29.htm", page)
	}
	if n, c := srv.PlayerItemCount(t, w.player, medallion), srv.PlayerItemCount(t, w.player, coupon); n != 0 || c != 15 {
		t.Fatalf("medallion %d coupons %d, want 0 and 15", n, c)
	}
	if info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)]); int(info.visibleClass) != warrior.ID {
		t.Fatalf("UserInfo class = %d, want the Warrior %d", info.visibleClass, warrior.ID)
	}
	srv.TickAutosave(t)
	srv.FlushPersistence(t)
	var classID, baseClass int
	if err := srv.DB.QueryRowContext(context.Background(), `SELECT classid, base_class FROM characters WHERE obj_Id = ?`, w.player).Scan(&classID, &baseClass); err != nil {
		t.Fatal(err)
	}
	if classID != warrior.ID || baseClass != warrior.ID {
		t.Fatalf("saved class/base class = %d/%d, want the Warrior %d for both", classID, baseClass, warrior.ID)
	}
}
