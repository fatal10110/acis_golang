package admin

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: AdminCursedWeapon.java (//cw), CursedWeapon.java
// (teleportTo, endOfLife, reActivate) and CursedWeaponManager.reload.

const (
	zaricheID  int32 = 8190
	akamanahID int32 = 8689
)

// cursedWeaponTable is cursedWeapons.xml as the datapack ships it.
func cursedWeaponTable(t *testing.T, weapons ...int32) *entity.CursedWeaponTable {
	t.Helper()
	zariche := entity.CursedWeapon{
		ItemID: zaricheID, Skill: modelskill.Ref{ID: 3603, Level: 3}, Name: "Demonic Sword Zariche",
		DropRate: 1, Duration: 72, DurationLost: 24, DisappearChance: 50, StageKills: 10,
	}
	akamanah := zariche
	akamanah.ItemID, akamanah.Name, akamanah.Skill = akamanahID, "Blood Sword Akamanah", modelskill.Ref{ID: 3629, Level: 3}
	var defs []entity.CursedWeapon
	for _, def := range []entity.CursedWeapon{zariche, akamanah} {
		for _, id := range weapons {
			if def.ItemID == id {
				defs = append(defs, def)
			}
		}
	}
	table, err := entity.NewCursedWeaponTable(defs)
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	return table
}

// cursedItemTemplates is the shared catalog with Zariche worn in both
// hands, as the datapack declares it, and Akamanah as a copy of it.
func cursedItemTemplates() *item.Table {
	templates := gameservertest.ItemTemplates().All()
	for i, tmpl := range templates {
		if tmpl.ID == zaricheID {
			lr := *tmpl
			lr.Slot = item.SlotLRHand
			lr.Dropable, lr.Destroyable, lr.Tradable = false, false, false
			templates[i] = &lr
			akamanah := lr
			akamanah.ID, akamanah.Name = akamanahID, "Blood Sword Akamanah"
			templates = append(templates, &akamanah)
			break
		}
	}
	return item.NewTable(templates)
}

// cursedBase is the clock the cursed weapons read in these tests.
var cursedBase = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// bootCursedAdmin boots the game master with both cursed weapons running
// on a clock stopped at cursedBase plus at, and the shipped cwinfo.htm.
func bootCursedAdmin(t *testing.T, at time.Duration, opts ...gameservertest.Option) (*gameservertest.Server, int32) {
	t.Helper()
	return bootAdmin(t, adminLevel, append([]gameservertest.Option{
		gameservertest.WithHTMLPages(shippedAdminPages(t, "main_menu.htm", "game_menu.htm", "teleports.htm", "cwinfo.htm")),
		gameservertest.WithItemTemplates(cursedItemTemplates()),
		gameservertest.WithCursedWeapons(cursedWeaponTable(t, zaricheID, akamanahID)),
		gameservertest.WithCursedWeaponClock(func() time.Time { return cursedBase.Add(at) }),
	}, opts...)...)
}

// cwPage is cwinfo.htm, read from the datapack, with %cwinfo% filled.
func cwPage(t *testing.T, info string) string {
	t.Helper()
	raw, err := os.ReadFile(datapack.Path(t, "data", "html", "admin", "cwinfo.htm"))
	if err != nil {
		t.Fatalf("read cwinfo.htm: %v", err)
	}
	return strings.ReplaceAll(string(raw), "%cwinfo%", info)
}

// The panel rows of AdminCursedWeapon.showCursedWeaponSelectPage, written
// out from the reference's literals.
func cwNotOut(name string, id string) string {
	return `<table width=280><tr><td>Name:</td><td>` + name + `</td></tr>` +
		`<tr><td>Position:</td><td>Doesn't exist.</td></tr><tr><td><button value="Set CW" action="bypass -h admin_cw set ` + id + `" width=75 height=21 back="L2UI_ch3.Btn1_normalOn" fore="L2UI_ch3.Btn1_normal"></td><td></td></tr>` +
		`</table>`
}

func cwHeld(name, id, owner, karma, pks, stage, overall, hungry, kills string) string {
	return `<table width=280><tr><td>Name:</td><td>` + name + `</td></tr>` +
		`<tr><td>Owner:</td><td>` + owner + `</td></tr><tr><td>Stored values:</td><td>Karma=` + karma + ` PKs=` + pks + `</td></tr>` +
		`<tr><td>Current stage:</td><td>` + stage + `</td></tr><tr><td>Overall time:</td><td>` + overall + `</td></tr>` +
		`<tr><td>Hungry time:</td><td>` + hungry + `m.</td></tr><tr><td>Current kills:</td><td>` + kills + `</td></tr>` +
		`<tr><td><button value="Remove CW" action="bypass -h admin_cw remove ` + id + `" width=75 height=21 back="L2UI_ch3.Btn1_normalOn" fore="L2UI_ch3.Btn1_normal"></td>` +
		`<td><button value="Teleport To" action="bypass -h admin_cw teleportto ` + id + `" width=75 height=21 back="L2UI_ch3.Btn1_normalOn" fore="L2UI_ch3.Btn1_normal"></td></tr>` +
		`</table>`
}

func cwOnGround(name, id, overall string) string {
	return `<table width=280><tr><td>Name:</td><td>` + name + `</td></tr>` +
		`<tr><td>Position:</td><td>Lying on the ground</td></tr><tr><td>Overall time:</td><td>` + overall + `</td></tr>` +
		`<tr><td><button value="Remove" action="bypass -h admin_cw remove ` + id + `" width=75 height=21 back="L2UI_ch3.Btn1_normalOn" fore="L2UI_ch3.Btn1_normal"></td>` +
		`<td><button value="Go" action="bypass -h admin_cw teleportto ` + id + `" width=75 height=21 back="L2UI_ch3.Btn1_normalOn" fore="L2UI_ch3.Btn1_normal"></td></tr>` +
		`</table>`
}

// Both weapons not out, in the reference's hash map order: Akamanah first.
var cwNoneOut = cwNotOut("Blood Sword Akamanah", "8689") + cwNotOut("Demonic Sword Zariche", "8190")

// lastPage requires the last of frames to be the panel and returns it,
// without the line break the page cache ends a page with.
func lastPage(t *testing.T, frames [][]byte) string {
	t.Helper()
	if len(frames) == 0 {
		t.Fatal("no frames, want the panel last")
	}
	return strings.TrimSuffix(htmlBody(t, frames[len(frames)-1]), "\n")
}

// readUntilPage reads c until an NpcHtmlMessage, returning every frame up
// to it: the panel a command sends from another player's queue.
func readUntilPage(t *testing.T, c *testsupport.ScriptedClient, frames [][]byte) [][]byte {
	t.Helper()
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeNpcHtmlMessage {
			return frames
		}
	}
	for range 200 {
		f := c.ReadWithTimeout(5 * time.Second)
		if f == nil {
			t.Fatalf("no panel after %x", testsupport.FrameOpcodes(frames))
		}
		frames = append(frames, f)
		if f[0] == serverpackets.OpcodeNpcHtmlMessage {
			return frames
		}
	}
	t.Fatal("no panel")
	return nil
}

// teleportFrame requires frames to hold a TeleportToLocation, then end on
// the panel, and returns the teleport.
func teleportFrame(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	lastPage(t, frames)
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeTeleportToLocation {
			return f
		}
	}
	t.Fatalf("frames = %x, want a teleport, then the panel", testsupport.FrameOpcodes(frames))
	return nil
}

// itemMessage returns a SystemMessage frame's id and item-name parameter,
// false when it is no message naming exactly one item.
func itemMessage(frame []byte) (int32, int32, bool) {
	if frame[0] != serverpackets.OpcodeSystemMessage {
		return 0, 0, false
	}
	r := wire.NewReader(frame[1:])
	id, n := r.ReadInt32(), r.ReadInt32()
	if n != 1 || r.ReadInt32() != serverpackets.SystemMessageParamItemName {
		return 0, 0, false
	}
	return id, r.ReadInt32(), true
}

// indexOfItemMessage finds the first message id naming itemID, or -1.
func indexOfItemMessage(frames [][]byte, id, itemID int32) int {
	for i, f := range frames {
		if got, gotItem, ok := itemMessage(f); ok && got == id && gotItem == itemID {
			return i
		}
	}
	return -1
}

func nextStageKills(t *testing.T, srv *gameservertest.Server, itemID int32) string {
	t.Helper()
	for _, w := range srv.CursedWeapons.Weapons() {
		if w.ItemID == itemID {
			return strconv.Itoa(int(w.Kills)) + " / " + strconv.Itoa(int(w.NextStageKills))
		}
	}
	t.Fatalf("no weapon %d", itemID)
	return ""
}

// TestAdminCursedWeaponsOnGM pins //cw on the game master itself: the panel
// lists both weapons not out; //cw set gives the weapon to gm, which gets it
// as it would picking it up and becomes its holder, the weapon starting its
// full 72 hours and 24 hours of hunger; the panel then shows gm holding it.
// A second set is refused. teleportto moves gm to the holder; remove gives
// gm its karma back, takes the weapon and tells everyone it disappeared.
func TestAdminCursedWeaponsOnGM(t *testing.T) {
	t.Parallel()
	srv, gmID := bootCursedAdmin(t, 0)
	gm := srv.Client
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET karma = 100, pkkills = 3 WHERE obj_Id = ?", gmID); err != nil {
		t.Fatalf("seed karma: %v", err)
	}
	enterWorld(t, gm)

	if got, want := lastPage(t, exchange(t, gm, encodeBuildCmd("cw"))), cwPage(t, cwNoneOut); got != want {
		t.Fatalf("//cw page = %q\nwant %q", got, want)
	}

	frames := exchange(t, gm, encodeBuildCmd("cw set 8190"))
	picked := indexOfItemMessage(frames, serverpackets.SystemMessageYouPickedUpS1, zaricheID)
	if picked < 0 || picked == len(frames)-1 {
		t.Fatalf("//cw set frames = %x, want the pickup message ahead of the panel", testsupport.FrameOpcodes(frames))
	}
	holder := onlineCharacter(t, srv, gmID)
	if holder.CursedWeaponID() != zaricheID || holder.Karma() != 9999999 {
		t.Fatalf("gm holds %d with karma %d, want Zariche and 9999999", holder.CursedWeaponID(), holder.Karma())
	}
	held := cwHeld("Demonic Sword Zariche", "8190", "Admin", "100", "3", "1", "3d. 0h. 0m.", "1440", nextStageKills(t, srv, zaricheID))
	if got, want := lastPage(t, frames), cwPage(t, cwNotOut("Blood Sword Akamanah", "8689")+held); got != want {
		t.Fatalf("//cw set page = %q\nwant %q", got, want)
	}
	if left := srv.CursedWeapons.TimeLeft(zaricheID); left != 72*time.Hour {
		t.Fatalf("time left = %v, want 72h", left)
	}

	frames = exchange(t, gm, encodeBuildCmd("cw set zariche"))
	if len(frames) != 2 {
		t.Fatalf("second //cw set frames = %x, want the refusal and the panel", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "This cursed weapon is already active.")

	frames = exchange(t, gm, encodeBuildCmd("cw teleportto 8190"))
	if id, _ := teleportTo(t, teleportFrame(t, frames)); id != gmID {
		t.Fatalf("teleported %d, want gm", id)
	}

	frames = exchange(t, gm, encodeBuildCmd("cw remove demonic_sword"))
	if gone := indexOfItemMessage(frames, serverpackets.SystemMessageS1HasDisappeared, zaricheID); gone < 0 || gone == len(frames)-1 {
		t.Fatalf("//cw remove frames = %x, want S1_HAS_DISAPPEARED ahead of the panel", testsupport.FrameOpcodes(frames))
	}
	if got, want := lastPage(t, frames), cwPage(t, cwNoneOut); got != want {
		t.Fatalf("//cw remove page = %q\nwant %q", got, want)
	}
	if holder.CursedWeaponEquipped() || holder.Karma() != 100 || holder.ProgressionValues().PKKills != 3 || holder.Inventory().ItemByTemplateID(zaricheID) != nil {
		t.Fatalf("after remove: weapon %d karma %d pk %d, want none, 100, 3 and no Zariche", holder.CursedWeaponID(), holder.Karma(), holder.ProgressionValues().PKKills)
	}
}

// TestAdminCursedWeaponsParameters pins how //cw reads its parameters: no
// weapon, or an id past an int, answers the usage; a name or id no weapon
// has is unknown; another command word only reopens the panel; teleportto
// a weapon not out says it is not in the world, then reopens the panel;
// remove ends even a weapon not out, telling everyone it disappeared.
func TestAdminCursedWeaponsParameters(t *testing.T) {
	t.Parallel()
	srv, _ := bootCursedAdmin(t, 0)
	gm := srv.Client
	enterWorld(t, gm)

	usage := "Usage: //cw [set|remove|teleportto itemid|name]"
	for _, tc := range []struct{ cmd, want string }{
		{"cw set", usage},
		{"cw remove 99999999999", usage},
		{"cw set 1234", "Unknown cursed weapon ID."},
		{"cw set excalibur", "Unknown cursed weapon ID."},
	} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd(tc.cmd)), tc.want)
	}
	if got, want := lastPage(t, exchange(t, gm, encodeBuildCmd("cw look 8190"))), cwPage(t, cwNoneOut); got != want {
		t.Fatalf("//cw look page = %q\nwant %q", got, want)
	}

	frames := exchange(t, gm, encodeBuildCmd("cw teleportto BLOOD_sword"))
	if len(frames) != 2 {
		t.Fatalf("//cw teleportto frames = %x, want the notice and the panel", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "Blood Sword Akamanah isn't in the world.")

	frames = exchange(t, gm, encodeBuildCmd("cw remove 8689"))
	if len(frames) != 2 || indexOfItemMessage(frames, serverpackets.SystemMessageS1HasDisappeared, akamanahID) != 0 {
		t.Fatalf("//cw remove of a weapon not out frames = %x, want S1_HAS_DISAPPEARED and the panel", testsupport.FrameOpcodes(frames))
	}
}

// TestAdminCursedWeaponsOnTarget pins //cw on a selected player: set gives
// it the weapon on its own queue, and the panel names it; teleportto takes
// gm to it; remove gives it its karma and PK kills back and takes the
// weapon, everyone hearing it disappeared before the panel reopens.
func TestAdminCursedWeaponsOnTarget(t *testing.T) {
	t.Parallel()
	srv, _ := bootCursedAdmin(t, 90*time.Minute)
	gm := srv.Client
	enterWorld(t, gm)
	holderClient, holderID := addPlayer(t, srv, "player2", "Holder", userLevel)
	drain(t, gm)
	exchange(t, gm, encodeAction(holderID))

	frames := readUntilPage(t, gm, exchange(t, gm, encodeBuildCmd("cw set 8689")))
	if indexOfItemMessage(settle(t, holderClient), serverpackets.SystemMessageYouPickedUpS1, akamanahID) < 0 {
		t.Fatal("the holder was not told it picked Akamanah up")
	}
	holder := onlineCharacter(t, srv, holderID)
	if holder.CursedWeaponID() != akamanahID {
		t.Fatalf("holder holds %d, want Akamanah", holder.CursedWeaponID())
	}
	// The life started at the set: 72h left, read at the same clock.
	held := cwHeld("Blood Sword Akamanah", "8689", "Holder", "0", "0", "1", "3d. 0h. 0m.", "1440", nextStageKills(t, srv, akamanahID))
	if got, want := lastPage(t, frames), cwPage(t, held+cwNotOut("Demonic Sword Zariche", "8190")); got != want {
		t.Fatalf("//cw set page = %q\nwant %q", got, want)
	}

	teleportFrame(t, exchange(t, gm, encodeBuildCmd("cw teleportto 8689")))

	frames = readUntilPage(t, gm, exchange(t, gm, encodeBuildCmd("cw remove 8689")))
	if gone := indexOfItemMessage(frames, serverpackets.SystemMessageS1HasDisappeared, akamanahID); gone < 0 || gone == len(frames)-1 {
		t.Fatalf("//cw remove frames = %x, want S1_HAS_DISAPPEARED ahead of the panel", testsupport.FrameOpcodes(frames))
	}
	if got, want := lastPage(t, frames), cwPage(t, cwNoneOut); got != want {
		t.Fatalf("//cw remove page = %q\nwant %q", got, want)
	}
	if holder.CursedWeaponEquipped() || holder.Karma() != 0 || holder.Inventory().ItemByTemplateID(akamanahID) != nil {
		t.Fatalf("after remove: weapon %d karma %d, want none, 0 and no Akamanah", holder.CursedWeaponID(), holder.Karma())
	}
}

// seedOfflineHolder makes a character out of the world Zariche's holder:
// karma 100 and 3 PK kills before it, stage 2, 2 of 5 kills, 600 minutes
// of hunger, its life ending at end.
func seedOfflineHolder(t *testing.T, srv *gameservertest.Server, end time.Time) int32 {
	t.Helper()
	ctx := context.Background()
	ch := srv.SeedCharacterFor(t, "player2", "Gone", 1, 0)
	srv.GiveItem(t, ch.ID, zaricheID, 1)
	if _, err := srv.DB.ExecContext(ctx, "UPDATE characters SET karma = 9999999, pkkills = 0 WHERE obj_Id = ?", ch.ID); err != nil {
		t.Fatalf("seed holder karma: %v", err)
	}
	if _, err := srv.DB.ExecContext(ctx,
		"INSERT INTO cursed_weapons (itemId, playerId, playerKarma, playerPkKills, nbKills, currentStage, numberBeforeNextStage, hungryTime, endTime) VALUES (?, ?, 100, 3, 2, 2, 5, 600, ?)",
		zaricheID, ch.ID, end.UnixMilli()); err != nil {
		t.Fatalf("seed cursed_weapons: %v", err)
	}
	if err := srv.CursedWeapons.Restore(ctx); err != nil {
		t.Fatalf("restore cursed weapons: %v", err)
	}
	return ch.ID
}

// TestAdminCursedWeaponsOfflineHolder pins //cw with the holder out of the
// world: the panel names no owner ("null") and shows the stored values and
// the time left; teleportto cannot reach the holder and answers the usage;
// remove gives the stored character its karma and PK kills back and takes
// the weapon from its stored items.
func TestAdminCursedWeaponsOfflineHolder(t *testing.T) {
	t.Parallel()
	srv, _ := bootCursedAdmin(t, 0)
	gm := srv.Client
	enterWorld(t, gm)
	// 70h29m15s left.
	holderID := seedOfflineHolder(t, srv, cursedBase.Add(70*time.Hour+29*time.Minute+15*time.Second))

	held := cwHeld("Demonic Sword Zariche", "8190", "null", "100", "3", "2", "2d. 22h. 29m.", "600", "2 / 5")
	if got, want := lastPage(t, exchange(t, gm, encodeBuildCmd("cw"))), cwPage(t, cwNotOut("Blood Sword Akamanah", "8689")+held); got != want {
		t.Fatalf("//cw page = %q\nwant %q", got, want)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("cw teleportto 8190")), "Usage: //cw [set|remove|teleportto itemid|name]")

	frames := exchange(t, gm, encodeBuildCmd("cw remove 8190"))
	if gone := indexOfItemMessage(frames, serverpackets.SystemMessageS1HasDisappeared, zaricheID); gone != 0 || len(frames) != 2 {
		t.Fatalf("//cw remove frames = %x, want S1_HAS_DISAPPEARED and the panel", testsupport.FrameOpcodes(frames))
	}
	if got, want := lastPage(t, frames), cwPage(t, cwNoneOut); got != want {
		t.Fatalf("//cw remove page = %q\nwant %q", got, want)
	}
	srv.FlushPersistence(t)
	ctx := context.Background()
	var karma, pk, weapons, rows int32
	if err := srv.DB.QueryRowContext(ctx, "SELECT karma, pkkills FROM characters WHERE obj_Id = ?", holderID).Scan(&karma, &pk); err != nil {
		t.Fatalf("read character: %v", err)
	}
	if err := srv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM items WHERE owner_id = ? AND item_id = ?", holderID, zaricheID).Scan(&weapons); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if err := srv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM cursed_weapons").Scan(&rows); err != nil {
		t.Fatalf("count cursed_weapons: %v", err)
	}
	if karma != 100 || pk != 3 || weapons != 0 || rows != 0 {
		t.Fatalf("stored karma %d pk %d weapons %d rows %d, want 100, 3, 0, 0", karma, pk, weapons, rows)
	}
}

// TestAdminCursedWeaponsOnGround pins the panel and teleportto for a
// weapon lying on the ground.
func TestAdminCursedWeaponsOnGround(t *testing.T) {
	t.Parallel()
	srv, gmID := bootCursedAdmin(t, 0)
	gm := srv.Client
	enterWorld(t, gm)
	if itemID, ok := srv.CursedWeapons.RollDrop(func(int) int { return 0 }); !ok || itemID != akamanahID {
		t.Fatalf("drop = %d %v, want Akamanah", itemID, ok)
	}
	srv.CursedWeapons.PlaceOnGround(akamanahID, 1, location.Location{X: 1000, Y: 2000, Z: -300})

	want := cwPage(t, cwOnGround("Blood Sword Akamanah", "8689", "3d. 0h. 0m.")+cwNotOut("Demonic Sword Zariche", "8190"))
	if got := lastPage(t, exchange(t, gm, encodeBuildCmd("cw"))); got != want {
		t.Fatalf("//cw page = %q\nwant %q", got, want)
	}
	frames := exchange(t, gm, encodeBuildCmd("cw teleportto 8689"))
	if id, at := teleportTo(t, teleportFrame(t, frames)); id != gmID || at != [3]int32{1000, 2000, -300} {
		t.Fatalf("teleported %d to %v, want gm to the weapon", id, at)
	}
}

// TestAdminReloadCursedWeapons pins //reload cw: every weapon ends, a held
// one giving its holder its karma back and each heard to disappear, then
// the weapons are read anew; the panel lists the new ones, none out.
func TestAdminReloadCursedWeapons(t *testing.T) {
	t.Parallel()
	fresh := cursedWeaponTable(t, zaricheID)
	srv, gmID := bootCursedAdmin(t, 0, gameservertest.WithDataReloads(network.DataReloads{
		CursedWeapons: func() (*entity.CursedWeaponTable, error) { return fresh, nil },
	}))
	gm := srv.Client
	enterWorld(t, gm)
	exchange(t, gm, encodeBuildCmd("cw set 8190"))

	frames := exchange(t, gm, encodeBuildCmd("reload cw"))
	frames = append(frames, settle(t, gm)...)
	aka := indexOfItemMessage(frames, serverpackets.SystemMessageS1HasDisappeared, akamanahID)
	zar := indexOfItemMessage(frames, serverpackets.SystemMessageS1HasDisappeared, zaricheID)
	if aka < 0 || zar < 0 {
		t.Fatalf("//reload cw frames = %x, want both weapons to disappear", testsupport.FrameOpcodes(frames))
	}
	var reloaded bool
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			if id, text := systemTextOrEmpty(f); id == serverpackets.SystemMessageS1 && text == "Cursed weapons have been reloaded." {
				reloaded = true
			}
		}
	}
	if !reloaded {
		t.Fatal("//reload cw did not answer that the cursed weapons were reloaded")
	}
	holder := onlineCharacter(t, srv, gmID)
	if holder.CursedWeaponEquipped() || holder.Karma() != 0 || holder.Inventory().ItemByTemplateID(zaricheID) != nil {
		t.Fatalf("after the reload: weapon %d karma %d, want none, 0 and no Zariche", holder.CursedWeaponID(), holder.Karma())
	}
	if got, want := lastPage(t, exchange(t, gm, encodeBuildCmd("cw"))), cwPage(t, cwNotOut("Demonic Sword Zariche", "8190")); got != want {
		t.Fatalf("//cw page after the reload = %q\nwant %q", got, want)
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("cw set 8689")), "Unknown cursed weapon ID.")
}

// systemTextOrEmpty returns a SystemMessage frame's id and its one text
// parameter, "" when it has none.
func systemTextOrEmpty(frame []byte) (int, string) {
	r := wire.NewReader(frame[1:])
	id := int(r.ReadInt32())
	if n := r.ReadInt32(); n != 1 {
		return id, ""
	}
	if r.ReadInt32() != serverpackets.SystemMessageParamText {
		return id, ""
	}
	return id, r.ReadString()
}
