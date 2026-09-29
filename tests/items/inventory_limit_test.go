package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPickupRefusedAtConfiguredInventoryLimit pins the player slot limit to
// the configured MaximumSlotsForNoDwarf: a character holding that many
// stacks is told its limit on entry, a new stack on the ground is refused
// with ActionFailed then SlotsFull and stays there, and a pickup that only
// merges into a held stack still goes through.
func TestPickupRefusedAtConfiguredInventoryLimit(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithInventorySlots(2, 3),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	srv.GiveItem(t, objID, 30, 1)
	srv.GiveItem(t, objID, item.AdenaID, 10)
	burst := startInWorld(t, c)

	storage := burst[1]
	r := wire.NewReader(storage[1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExStorageMaxCount {
		t.Fatalf("enter-world extended frame = %#x, want ExStorageMaxCount", sub)
	}
	if got := r.ReadInt32(); got != 2 {
		t.Fatalf("ExStorageMaxCount inventory limit = %d, want the configured 2", got)
	}

	srv.SeedGroundItem(t, 0, item.AdenaID, 5, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	adenaID := soleGroundObjectID(t, srv)
	c.Send(encodeAction(adenaID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "pickup release")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeGetItem, "GetItem")
	drainUntilQuiet(t, c)
	if _, ok := srv.State.Object(adenaID); ok {
		t.Fatal("stackable adena stayed on the ground although it needs no new slot")
	}

	srv.SeedGroundItem(t, 0, 30, 1, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	swordID := soleGroundObjectID(t, srv)
	c.Send(encodeAction(swordID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "slots-full lead")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSlotsFull)
	barrier(t, c)
	if _, ok := srv.State.Object(swordID); !ok {
		t.Fatal("ground weapon left the ground for a full inventory")
	}
	if n := carriedCount(t, srv, objID, 30); n != 1 {
		t.Fatalf("carried weapons after the refused pickup = %d, want 1", n)
	}
}

// inventoryLimitArmorID is the catalog's chest armor, cloned with an
// inventoryLimit bonus for the equip-toggle scenario.
const inventoryLimitArmorID int32 = 40

// TestEquipToggleResendsStorageLimitWhenInventoryLimitMoves pins the
// storage-limit resend at the end of an equip toggle: putting on armor whose
// inventoryLimit bonus raises the limit sends ExStorageMaxCount with the new
// limit after the UserInfo refresh, taking it off sends the lowered limit
// the same way, and toggling an item that leaves the limit alone sends none.
func TestEquipToggleResendsStorageLimitWhenInventoryLimitMoves(t *testing.T) {
	t.Parallel()
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == inventoryLimitArmorID {
			clone := *tmpl
			clone.Modifiers = append(append([]item.StatModifier(nil), tmpl.Modifiers...),
				item.StatModifier{Op: item.FuncAdd, Stat: "inventoryLimit", Value: 5})
			tmpl = &clone
		}
		templates = append(templates, tmpl)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	armor := srv.GiveItem(t, objID, inventoryLimitArmorID, 1)
	weapon := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)

	c.Send(encodeUseItem(armor, false))
	assertStorageLimitAfterUserInfo(t, collectUntilQuiet(t, c), 85)

	c.Send(encodeUseItem(armor, false))
	assertStorageLimitAfterUserInfo(t, collectUntilQuiet(t, c), 80)

	for range 2 {
		c.Send(encodeUseItem(weapon, false))
		frames := collectUntilQuiet(t, c)
		if storageLimitFrame(frames) >= 0 {
			t.Fatal("toggling a weapon without an inventoryLimit bonus sent ExStorageMaxCount")
		}
		if userInfoFrame(frames) < 0 {
			t.Fatal("weapon toggle sent no UserInfo refresh")
		}
	}
}

// assertStorageLimitAfterUserInfo requires exactly one ExStorageMaxCount in
// frames, after the first UserInfo, reporting the wanted inventory limit.
func assertStorageLimitAfterUserInfo(t *testing.T, frames [][]byte, want int32) {
	t.Helper()
	storage, userInfo := storageLimitFrame(frames), userInfoFrame(frames)
	if storage < 0 || userInfo < 0 || storage < userInfo {
		t.Fatalf("ExStorageMaxCount at frame %d, UserInfo at %d: want ExStorageMaxCount after UserInfo", storage, userInfo)
	}
	for _, f := range frames[storage+1:] {
		if isStorageLimitFrame(f) {
			t.Fatal("equip toggle sent a second ExStorageMaxCount")
		}
	}
	r := wire.NewReader(frames[storage][3:])
	if got := r.ReadInt32(); got != want {
		t.Fatalf("ExStorageMaxCount inventory limit = %d, want %d", got, want)
	}
}

func isStorageLimitFrame(f []byte) bool {
	return len(f) >= 3 && f[0] == serverpackets.OpcodeExtended &&
		wire.NewReader(f[1:]).ReadUint16() == serverpackets.OpcodeExStorageMaxCount
}

func storageLimitFrame(frames [][]byte) int {
	for i, f := range frames {
		if isStorageLimitFrame(f) {
			return i
		}
	}
	return -1
}

func userInfoFrame(frames [][]byte) int {
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeUserInfo {
			return i
		}
	}
	return -1
}
