package npcs

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const (
	merchantID = 30001
	// merchantPage links back to the NPC by its object id.
	merchantPage = `<html><body>Trader:<br><a action="bypass -h npc_%objectId%_Buy 1">Buy</a></body></html>`
)

func merchantPages() map[string]string {
	return map[string]string{"merchant/30001.htm": merchantPage}
}

// wantChatPage is page as the client receives it: the reference HTML cache
// ends every page with a newline, and %objectId% names f.
func wantChatPage(page string, f *npc.Folk) string {
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID()))) + "\n"
}

// TestFolkIsShownAsNotAttackableNpcInfo pins the NpcInfo a spawned
// civilian NPC is announced with: its template id, not attackable without
// forcing, running, at its spawn point, with the template speeds and body.
func TestFolkIsShownAsNotAttackableNpcInfo(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	f := w.srv.SpawnFolkNPCAt(t, folkTemplate("Merchant", merchantID), w.at)

	frame, ok := firstOpcode(drainFrames(t, w.c), serverpackets.OpcodeNPCInfo)
	if !ok {
		t.Fatal("spawned civilian NPC was not shown with NpcInfo")
	}
	r := wire.NewReader(frame[1:])
	objectID, templateID, attackable := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if objectID != f.ObjectID() || templateID != merchantID+1000000 || attackable != 0 {
		t.Fatalf("NpcInfo = object %d template %d attackable %d, want %d/%d/0", objectID, templateID, attackable, f.ObjectID(), merchantID+1000000)
	}
	if x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); int(x) != w.at.X || int(y) != w.at.Y || int(z) != w.at.Z {
		t.Fatalf("NpcInfo position = %d,%d,%d, want %+v", x, y, z, w.at)
	}
	r.ReadInt32() // heading
	r.ReadInt32()
	mAtkSpd, pAtkSpd, runSpd, walkSpd := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	// C.Spd is 333 times the WIT bonus (WIT 0 in the fixture: 0.38); P.Spd
	// is the template 253 times the DEX bonus (DEX 0: 0.84).
	if mAtkSpd != 126 || pAtkSpd != 212 || runSpd != 120 || walkSpd != 50 {
		t.Fatalf("NpcInfo speeds = matk %d patk %d run %d walk %d, want 126/212/120/50", mAtkSpd, pAtkSpd, runSpd, walkSpd)
	}
	for range 6 { // the other three run/walk pairs
		r.ReadInt32()
	}
	// The movement multiplier is the running speed 120 * 0.84 over the base
	// 120; the attack speed multiplier is (float) (1.1 * 212 / 253).
	run, dexBonus, patk, base := 120.0, 0.84, 212.0, 253.0
	wantMove := float64(float32(run*dexBonus) / float32(run))
	wantAttack := float64(float32(1.1 * patk / base))
	if move, attack := r.ReadFloat64(), r.ReadFloat64(); move != wantMove || attack != wantAttack {
		t.Fatalf("NpcInfo multipliers = move %v attack %v, want %v/%v", move, attack, wantMove, wantAttack)
	}
}

// TestFolkClickSelectsThenTalks pins the reference click flow on a civilian
// NPC in reach: the first click selects it (level-difference color, since
// it is attackable when forced, and its full HP); the second releases the
// client, shows the player facing it from the interaction distance, plays
// its talk animation for the watchers, and opens its chat page with its
// object id filled in, then ActionFailed.
func TestFolkClickSelectsThenTalks(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)

	sel := w.selectFolk(t, f)
	if got := string(opcodes(sel)); !strings.HasPrefix(got, string([]byte{serverpackets.OpcodeValidateLocation, serverpackets.OpcodeMyTargetSelected, serverpackets.OpcodeStatusUpdate})) {
		t.Fatalf("first click opcodes = %x, want ValidateLocation, MyTargetSelected, StatusUpdate first", got)
	}
	frame, _ := firstOpcode(sel, serverpackets.OpcodeMyTargetSelected)
	r := wire.NewReader(frame[1:])
	if id, color := r.ReadInt32(), int16(r.ReadUint16()); id != f.ObjectID() || color != playerLevel-70 {
		t.Fatalf("MyTargetSelected = %d color %d, want %d color %d", id, color, f.ObjectID(), playerLevel-70)
	}
	frame, _ = firstOpcode(sel, serverpackets.OpcodeStatusUpdate)
	r = wire.NewReader(frame[1:])
	r.ReadInt32()
	if n := r.ReadInt32(); n != 2 {
		t.Fatalf("StatusUpdate attribute count = %d, want 2", n)
	}
	// 2444 base HP times the CON 43 bonus 1.58.
	for _, want := range []int32{10, 3861, 9, 3861} {
		if got := r.ReadInt32(); got != want {
			t.Fatalf("StatusUpdate field = %d, want %d (max/current HP 3861)", got, want)
		}
	}
	if hasHTML := func() bool { _, ok := firstOpcode(sel, serverpackets.OpcodeNpcHtmlMessage); return ok }(); hasHTML {
		t.Fatal("first click opened a chat window")
	}

	frames := w.talk(t, f, false)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	if got := interactOrder(frames); string(got) != string(want) {
		t.Fatalf("talk order = %x, want %x (all %x)", got, want, opcodes(frames))
	}
	frame, _ = firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
	if mover, target, distance := moveToPawn(t, frame); mover != w.player || target != f.ObjectID() || distance != 150 {
		t.Fatalf("MoveToPawn = %d->%d at %d, want %d->%d at 150", mover, target, distance, w.player, f.ObjectID())
	}
	frame, _ = firstOpcode(frames, serverpackets.OpcodeSocialAction)
	r = wire.NewReader(frame[1:])
	if id, action := r.ReadInt32(), r.ReadInt32(); id != f.ObjectID() || action < 0 || action > 7 {
		t.Fatalf("SocialAction = %d action %d, want %d action in [0,7]", id, action, f.ObjectID())
	}
	frame, _ = firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	objectID, html, itemID := htmlMessage(t, frame)
	if objectID != f.ObjectID() || itemID != 0 || html != wantChatPage(merchantPage, f) {
		t.Fatalf("NpcHtmlMessage = object %d item %d html %q, want %d/0 %q", objectID, itemID, html, f.ObjectID(), wantChatPage(merchantPage, f))
	}

	// A second talk within the animation interval opens the page again
	// without another animation.
	frames = w.talk(t, f, false)
	want = []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	if got := interactOrder(frames); string(got) != string(want) {
		t.Fatalf("second talk order = %x, want %x", got, want)
	}
}

// TestFolkOutOfReachWalksThenTalks pins the approach: a talk from afar
// walks toward the NPC stopping 100 short, and the arrival talks.
func TestFolkOutOfReachWalksThenTalks(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 400)
	w.selectFolk(t, f)

	frames := w.talk(t, f, false)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatalf("talk from 400 opened the page at once: %x", opcodes(frames))
	}
	frame, ok := firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
	if !ok {
		t.Fatalf("talk from 400 did not approach: %x", opcodes(frames))
	}
	if mover, target, distance := moveToPawn(t, frame); mover != w.player || target != f.ObjectID() || distance != 100 {
		t.Fatalf("approach = %d->%d at %d, want %d->%d at 100", mover, target, distance, w.player, f.ObjectID())
	}

	mover := w.srv.PlayerMove(t, w.player)
	w.srv.AdvanceUntil(t, "approach arrival", func() bool { return !mover.Moving() })
	frames = drainFrames(t, w.c)
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	if got := interactOrder(frames); string(got) != string(want) {
		t.Fatalf("arrival order = %x, want %x (all %x)", got, want, opcodes(frames))
	}
}

// TestFolkShiftTalkFromAfarDoesNotWalk pins shift: out of reach, the
// interact ends idle after releasing the client.
func TestFolkShiftTalkFromAfarDoesNotWalk(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 400)
	w.selectFolk(t, f)

	frames := w.talk(t, f, true)
	if got := interactOrder(frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("shift talk from afar = %x, want only ActionFailed", opcodes(frames))
	}
}

// TestFolkChatPagePaths pins where each civilian type reads its first
// chat page, and the missing-page notice.
func TestFolkChatPagePaths(t *testing.T) {
	t.Parallel()
	pages := map[string]string{
		"default/30100.htm":              "<html><body>default %objectId%</body></html>",
		"npcdefault.htm":                 "<html><body>I have nothing to say to you.</body></html>",
		"gatekeeper/30006.htm":           "<html><body>gatekeeper</body></html>",
		"warehouse/30005.htm":            "<html><body>warehouse</body></html>",
		"trainer/30008.htm":              "<html><body>trainer</body></html>",
		"villagemaster/30026.htm":        "<html><body>master</body></html>",
		"symbolmaker/SymbolMaker.htm":    "<html><body>symbols</body></html>",
		"adventurer_guildsman/31729.htm": "<html><body>adventurer</body></html>",
		"fisherman/31562.htm":            "<html><body>fisherman</body></html>",
	}
	tests := []struct {
		kind string
		id   int
		want string
	}{
		{"Folk", 30100, pages["default/30100.htm"]},
		{"Folk", 30101, pages["npcdefault.htm"]},
		{"Gatekeeper", 30006, pages["gatekeeper/30006.htm"]},
		{"DungeonGatekeeper", 30006, pages["gatekeeper/30006.htm"]},
		{"WarehouseKeeper", 30005, pages["warehouse/30005.htm"]},
		{"Trainer", 30008, pages["trainer/30008.htm"]},
		{"VillageMasterFighter", 30026, pages["villagemaster/30026.htm"]},
		{"SymbolMaker", 30613, pages["symbolmaker/SymbolMaker.htm"]},
		{"Adventurer", 31729, pages["adventurer_guildsman/31729.htm"]},
		{"Fisherman", 31562, pages["fisherman/31562.htm"]},
		{"Merchant", 30002, "<html><body>My html is missing:<br>data/html/merchant/30002.htm</body></html>"},
	}
	w := bootFolkWorld(t, pages)
	for _, tt := range tests {
		f := w.spawnFolk(t, folkTemplate(tt.kind, tt.id), 30)
		w.selectFolk(t, f)
		frame, ok := firstOpcode(w.talk(t, f, false), serverpackets.OpcodeNpcHtmlMessage)
		if !ok {
			t.Fatalf("%s %d opened no chat window", tt.kind, tt.id)
		}
		want := wantChatPage(tt.want, f)
		if strings.HasPrefix(tt.want, "<html><body>My html is missing") {
			want = tt.want
		}
		if _, html, _ := htmlMessage(t, frame); html != want {
			t.Fatalf("%s %d page = %q, want %q", tt.kind, tt.id, html, want)
		}
	}
}

// TestFolkKarmaRefusalPage pins the karma gate on shop dialogs: with
// KarmaPlayerCanShop off, a player carrying karma gets the merchant's
// refusal page as is; with it on, the ordinary page.
func TestFolkKarmaRefusalPage(t *testing.T) {
	t.Parallel()
	const pkPage = "<html><body>No trade with killers %objectId%</body></html>"
	pages := map[string]string{"merchant/30001.htm": merchantPage, "merchant/30001-pk.htm": pkPage}
	for _, allowed := range []bool{false, true} {
		t.Run(fmt.Sprintf("allowed=%v", allowed), func(t *testing.T) {
			w := bootFolkWorldAs(t, withKarmaCharacter(100), pages, withShop(allowed))
			f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
			w.selectFolk(t, f)
			frames := w.talk(t, f, false)
			frame, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
			if !ok {
				t.Fatalf("no chat window: %x", opcodes(frames))
			}
			want := wantChatPage(merchantPage, f)
			if !allowed {
				want = pkPage + "\n"
			}
			if _, html, _ := htmlMessage(t, frame); html != want {
				t.Fatalf("page = %q, want %q", html, want)
			}
		})
	}
}

// TestMutedAndUnportedFolkOpenNothing pins the NPCs whose interact opens
// no page: a muted NPC, and a type whose dialog reads state not in place.
// Both still release the client and face the player toward the NPC.
func TestMutedAndUnportedFolkOpenNothing(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, map[string]string{"default/31000.htm": "<html><body>x</body></html>", "npcdefault.htm": "<html><body>y</body></html>"})
	for _, kind := range []string{"MutedFolk", "Doorman", "SignsPriest"} {
		f := w.spawnFolk(t, folkTemplate(kind, 31000), 40)
		w.selectFolk(t, f)
		frames := w.talk(t, f, false)
		want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn}
		if kind != "MutedFolk" {
			want = append(want, serverpackets.OpcodeSocialAction)
		}
		if got := interactOrder(frames); string(got) != string(want) {
			t.Fatalf("%s talk = %x, want %x", kind, got, want)
		}
	}
}
