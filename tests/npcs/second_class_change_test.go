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
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/secondclasschange"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The second class change (SecondClassChange) paths the quest scenarios
// cannot seed or compare: a character playing a subclass, and a change link
// sent by a character of another occupation, both in the subclass world,
// whose character is a human Gladiator; and the transfer's packets the
// scenarios do not render (the henna list and the character shown anew)
// with the saved class.

const (
	// hannavaltID is Grand Master Hannavalt, who changes human and elf
	// fighters.
	hannavaltID = 30109
	secondDir   = "script/feature/SecondClassChange/"
)

// secondClassChangeOptions registers the feature on Hannavalt with its
// datapack pages and the page that admits every command.
func secondClassChangeOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	datapack.Require(t)
	pages := map[string]string{"test/any.htm": anyNpcPage}
	entries, err := os.ReadDir(datapack.Path(t, "data", "html", "script", "feature", "SecondClassChange"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "feature", "SecondClassChange", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		pages[secondDir+e.Name()] = string(data)
	}
	return []gameservertest.Option{
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{hannavaltID: script.KindFolk},
			[]script.Listing{{Path: "script.feature.SecondClassChange"}},
			script.Catalog{"script.feature.SecondClassChange": secondclasschange.New}),
	}
}

// askHannavalt opens Hannavalt's second class change dialog through a page
// that admits the command, and returns the answer.
func (w *folkWorld) askHannavalt(t *testing.T, hannavalt *npc.Folk) [][]byte {
	t.Helper()
	w.openAnyNpcPage(t)
	return w.bypass(t, npcCommand(hannavalt, "Quest SecondClassChange"))
}

// TestSecondClassChangeOnTheBaseClass: the Gladiator, on its second
// occupation, is told by Hannavalt that it has made its second change. The
// Gladiator change link, sent anyway, answers the fighters' page for a
// character of another occupation, before the level or the marks are
// looked at.
func TestSecondClassChangeOnTheBaseClass(t *testing.T) {
	t.Parallel()
	w := bootSubclassWorld(t, secondClassChangeOptions(t)...)
	hannavalt := w.spawnFolk(t, folkTemplate("Folk", hannavaltID), 60)

	wantPage(t, w.askHannavalt(t, hannavalt), hannavalt, scriptPage(t, secondDir+"master_human_elf_fighter-77.htm", hannavalt), 2)
	w.openAnyNpcPage(t)
	wantPage(t, w.bypass(t, "Quest SecondClassChange GL"), hannavalt, scriptPage(t, secondDir+"master_human_elf_fighter-78.htm", hannavalt), 1)
}

// TestSecondClassChangeInASubclass: a character playing a subclass gets the
// no-quest page from Hannavalt, whatever its classes.
func TestSecondClassChangeInASubclass(t *testing.T) {
	t.Parallel()
	w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
		exec(t, srv, `INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, 0, 0, 40, 1)`, objID, spellhowler)
		exec(t, srv, `UPDATE characters SET classid = ? WHERE obj_Id = ?`, spellhowler, objID)
	}, secondClassChangeOptions(t)...)
	hannavalt := w.spawnFolk(t, folkTemplate("Folk", hannavaltID), 60)

	wantPage(t, w.askHannavalt(t, hannavalt), hannavalt, noQuestPage, 2)
}

// TestSecondClassChangeTransferFrames: a level 40 human Warrior holding the
// marks of Challenger, Trust and Duelist changes to a Gladiator at
// Hannavalt. The answer is, in order: the three marks taken and the coupons
// given, the fanfare, the class change's animation and message, the henna
// list and the character shown anew (a StatusUpdate regen tick may
// interleave), then the congratulation page and its release. The Gladiator
// is the character's class and base class, in the UserInfo and in its
// saved row.
func TestSecondClassChangeTransferFrames(t *testing.T) {
	t.Parallel()
	const challenger, trust, duelist, coupon = 2627, 2734, 2762, 8870
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{challenger, trust, duelist, coupon} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	warrior, gladiator := gameservertest.ClassTemplate(), gameservertest.ClassTemplate()
	warrior.ID, gladiator.ID = 1, 2
	srv := gameservertest.Boot(t, append(secondClassChangeOptions(t),
		gameservertest.WithCharacter("Talker", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithClassTemplates(warrior, gladiator),
		noBypassReuse,
	)...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	exec(t, srv, `UPDATE characters SET classid = ?, base_class = ? WHERE obj_Id = ?`, warrior.ID, warrior.ID, w.player)
	for _, id := range []int32{challenger, trust, duelist} {
		srv.GiveItem(t, w.player, id, 1)
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	hannavalt := w.spawnFolk(t, folkTemplate("Folk", hannavaltID), 60)

	wantPage(t, w.askHannavalt(t, hannavalt), hannavalt, scriptPage(t, secondDir+"master_human_elf_fighter-29.htm", hannavalt), 2)
	wantPage(t, w.bypass(t, "Quest SecondClassChange master_human_elf_fighter-30.htm"), hannavalt,
		scriptPage(t, secondDir+"master_human_elf_fighter-30.htm", hannavalt), 1)
	frames := w.bypass(t, "Quest SecondClassChange GL")
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
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound,
		serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeHennaInfo, serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("transfer answer = %x, want %x", got, want)
	}
	if ids := systemMessageIDs(frames); !slices.Equal(ids, []int32{302, 302, 302, 53, 1308}) {
		t.Fatalf("transfer messages = %v, want the three marks taken, the coupons earned and the class transfer", ids)
	}
	html := frames[slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeNpcHtmlMessage })]
	if _, page, _ := htmlMessage(t, html); page != scriptPage(t, secondDir+"master_human_elf_fighter-71.htm", hannavalt) {
		t.Fatalf("transfer page = %q, want master_human_elf_fighter-71.htm", page)
	}
	for _, id := range []int32{challenger, trust, duelist} {
		if n := srv.PlayerItemCount(t, w.player, id); n != 0 {
			t.Fatalf("mark %d held = %d, want 0", id, n)
		}
	}
	if c := srv.PlayerItemCount(t, w.player, coupon); c != 15 {
		t.Fatalf("coupons = %d, want 15", c)
	}
	if info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)]); int(info.visibleClass) != gladiator.ID {
		t.Fatalf("UserInfo class = %d, want the Gladiator %d", info.visibleClass, gladiator.ID)
	}
	srv.TickAutosave(t)
	srv.FlushPersistence(t)
	var classID, baseClass int
	if err := srv.DB.QueryRowContext(context.Background(), `SELECT classid, base_class FROM characters WHERE obj_Id = ?`, w.player).Scan(&classID, &baseClass); err != nil {
		t.Fatal(err)
	}
	if classID != gladiator.ID || baseClass != gladiator.ID {
		t.Fatalf("saved class/base class = %d/%d, want the Gladiator %d for both", classID, baseClass, gladiator.ID)
	}
}
