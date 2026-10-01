package clan

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Crest image sizes the client uploads, and the system messages the
// uploads answer with, as the reference numbers them.
const (
	pledgeCrestSize = 256
	largeCrestSize  = 2176

	msgNotAuthorized    = 794
	msgLevel3Needed     = 272
	msgDissolving       = 552
	msgEmblemRegistered = 1663
	msgCrestDeleted     = 1861
)

// crestWorld is a clan world whose seeded level-3 clan the founder leads
// and the recruit has joined, with crests saved to dir.
type crestWorld struct {
	*clanWorld
	dir string
}

func bootCrestWorld(t *testing.T, s clanSeed, extra ...gameservertest.Option) *crestWorld {
	t.Helper()
	dir := t.TempDir()
	opts := append([]gameservertest.Option{seedClan(t, s), gameservertest.WithCrests(datacache.NewCrestsIn(dir))}, extra...)
	w := &crestWorld{clanWorld: bootClanWorld(t, 40, 0, 0, opts...), dir: dir}
	w.recruit(t)
	drainFrames(t, w.member)
	return w
}

func encodeSetPledgeCrest(data []byte) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSetPledgeCrest)
	w.WriteInt32(int32(len(data)))
	w.WriteBytes(data)
	return w.Bytes()
}

func encodeSetLargeCrest(data []byte) []byte {
	w := encodeExtended(clientpackets.OpcodeRequestExSetPledgeCrestLarge)
	w.WriteInt32(int32(len(data)))
	w.WriteBytes(data)
	return w.Bytes()
}

func encodeAskPledgeCrest(id int64) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPledgeCrest)
	w.WriteInt32(int32(id))
	return w.Bytes()
}

func le32(v int64) []byte { return binary.LittleEndian.AppendUint32(nil, uint32(v)) }

// storedCrest reads one crest column of the seeded clan.
func storedCrest(t *testing.T, w *crestWorld, column string) int64 {
	t.Helper()
	w.srv.FlushPersistence(t)
	return queryInt(t, w.clanWorld, `SELECT `+column+` FROM clan_data WHERE clan_id = ?`, seededClanID)
}

func crestFile(w *crestWorld, prefix string, id int64) string {
	return filepath.Join(w.dir, prefix+strconv.FormatInt(id, 10)+".dds")
}

// TestSetPledgeCrest registers a pledge crest: every online member is
// refreshed (its UserInfo naming the clan and the new crest, its CharInfo
// to the players around it) before the uploader is told; the image is
// saved, its id stored, and any client asking for it gets it. A second
// upload replaces it and removes the old image; an empty upload deletes
// it, and deleting again answers nothing.
func TestSetPledgeCrest(t *testing.T) {
	w := bootCrestWorld(t, clanSeed{level: 3})
	image := bytes.Repeat([]byte{0x5a}, pledgeCrestSize)

	frames := drainSend(t, w, encodeSetPledgeCrest(image))
	if got := only(frames, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage); string(got) != string([]byte{serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("upload answer = %x, want UserInfo then SystemMessage", opcodes(frames))
	}
	if ids := messages(t, frames); !slices.Equal(ids, []int{msgEmblemRegistered}) {
		t.Fatalf("upload messages = %v", ids)
	}
	crestID := storedCrest(t, w, "crest_id")
	if crestID == 0 {
		t.Fatal("stored crest_id = 0 after the upload")
	}
	userInfo, _ := firstOpcode(frames, serverpackets.OpcodeUserInfo)
	if !bytes.Contains(userInfo, append(le32(seededClanID), le32(crestID)...)) {
		t.Fatalf("leader's UserInfo does not carry clan %d with crest %d", seededClanID, crestID)
	}
	member := drainFrames(t, w.member)
	if info, ok := firstOpcode(member, serverpackets.OpcodeUserInfo); !ok || !bytes.Contains(info, append(le32(seededClanID), le32(crestID)...)) {
		t.Fatalf("member's refresh = %x, want its UserInfo with the new crest", opcodes(member))
	}
	if info, ok := firstOpcode(member, serverpackets.OpcodeCharInfo); !ok || !bytes.Contains(info, le32(crestID)) {
		t.Fatalf("member's refresh = %x, want the leader's CharInfo with the new crest", opcodes(member))
	}
	if got, err := os.ReadFile(crestFile(w, "Crest_", crestID)); err != nil || !bytes.Equal(got, image) {
		t.Fatalf("saved crest file = %d bytes, %v", len(got), err)
	}

	w.member.Send(encodeAskPledgeCrest(crestID))
	want := append(append(append([]byte{0x6c}, le32(crestID)...), le32(pledgeCrestSize)...), image...)
	if got := w.member.Read(); !bytes.Equal(got, want) {
		t.Fatalf("PledgeCrest = %x..., want the uploaded image", got[:min(len(got), 12)])
	}

	replacement := bytes.Repeat([]byte{0x33}, pledgeCrestSize)
	if ids := messages(t, drainSend(t, w, encodeSetPledgeCrest(replacement))); !slices.Equal(ids, []int{msgEmblemRegistered}) {
		t.Fatalf("replacement messages = %v", ids)
	}
	newID := storedCrest(t, w, "crest_id")
	if newID == crestID || newID == 0 {
		t.Fatalf("stored crest_id = %d after the replacement, was %d", newID, crestID)
	}
	if _, err := os.Stat(crestFile(w, "Crest_", crestID)); !os.IsNotExist(err) {
		t.Fatalf("replaced crest file still exists: %v", err)
	}

	if ids := messages(t, drainSend(t, w, encodeSetPledgeCrest(nil))); !slices.Equal(ids, []int{msgCrestDeleted}) {
		t.Fatalf("deletion messages = %v", ids)
	}
	if got := storedCrest(t, w, "crest_id"); got != 0 {
		t.Fatalf("stored crest_id = %d after the deletion", got)
	}
	if _, err := os.Stat(crestFile(w, "Crest_", newID)); !os.IsNotExist(err) {
		t.Fatalf("deleted crest file still exists: %v", err)
	}
	drainFrames(t, w.member)
	if frames := drainSend(t, w, encodeSetPledgeCrest(nil)); len(frames) != 0 {
		t.Fatalf("deleting no crest answered %x, want nothing", opcodes(frames))
	}
}

// TestSetLargePledgeCrest registers and deletes the large crest the same
// way, in its own column and file family.
func TestSetLargePledgeCrest(t *testing.T) {
	w := bootCrestWorld(t, clanSeed{level: 3})
	image := bytes.Repeat([]byte{0x7e}, largeCrestSize)

	frames := drainSend(t, w, encodeSetLargeCrest(image))
	if ids := messages(t, frames); !slices.Equal(ids, []int{msgEmblemRegistered}) {
		t.Fatalf("upload messages = %v (%x)", ids, opcodes(frames))
	}
	crestID := storedCrest(t, w, "crest_large_id")
	if crestID == 0 || storedCrest(t, w, "crest_id") != 0 {
		t.Fatalf("stored crest_large_id = %d, crest_id = %d", crestID, storedCrest(t, w, "crest_id"))
	}
	if got, err := os.ReadFile(crestFile(w, "LargeCrest_", crestID)); err != nil || !bytes.Equal(got, image) {
		t.Fatalf("saved large crest file = %d bytes, %v", len(got), err)
	}
	if !anyContains(frames, serverpackets.OpcodeUserInfo, le32(crestID)) {
		t.Fatal("leader's UserInfo does not carry the large crest")
	}

	if ids := messages(t, drainSend(t, w, encodeSetLargeCrest(nil))); !slices.Equal(ids, []int{msgCrestDeleted}) {
		t.Fatalf("deletion messages = %v", ids)
	}
	if got := storedCrest(t, w, "crest_large_id"); got != 0 {
		t.Fatalf("stored crest_large_id = %d after the deletion", got)
	}
	if _, err := os.Stat(crestFile(w, "LargeCrest_", crestID)); !os.IsNotExist(err) {
		t.Fatalf("deleted large crest file still exists: %v", err)
	}
}

// TestSetCrestRefusals covers the refusals in the order they are checked:
// a member without the crest privilege, a clan below level 3, a clan being
// dissolved; and the uploads answered with nothing: an image of the wrong
// size and a declared length over the limit.
func TestSetCrestRefusals(t *testing.T) {
	t.Run("member without the privilege", func(t *testing.T) {
		w := bootCrestWorld(t, clanSeed{level: 3})
		w.member.Send(encodeSetPledgeCrest(bytes.Repeat([]byte{1}, pledgeCrestSize)))
		if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{msgNotAuthorized}) {
			t.Fatalf("messages = %v, want YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT", ids)
		}
		if got := storedCrest(t, w, "crest_id"); got != 0 {
			t.Fatalf("stored crest_id = %d", got)
		}
	})
	t.Run("clan below level 3", func(t *testing.T) {
		w := bootCrestWorld(t, clanSeed{level: 2})
		if ids := messages(t, drainSend(t, w, encodeSetLargeCrest(bytes.Repeat([]byte{1}, largeCrestSize)))); !slices.Equal(ids, []int{msgLevel3Needed}) {
			t.Fatalf("messages = %v, want CLAN_LVL_3_NEEDED_TO_SET_CREST", ids)
		}
		if got := storedCrest(t, w, "crest_large_id"); got != 0 {
			t.Fatalf("stored crest_large_id = %d", got)
		}
	})
	t.Run("clan being dissolved", func(t *testing.T) {
		w := bootCrestWorld(t, clanSeed{level: 3, dissolving: time.Now().Add(time.Hour).UnixMilli()})
		if ids := messages(t, drainSend(t, w, encodeSetPledgeCrest(nil))); !slices.Equal(ids, []int{msgDissolving}) {
			t.Fatalf("messages = %v, want CANNOT_SET_CREST_WHILE_DISSOLUTION_IN_PROGRESS", ids)
		}
	})
	t.Run("silent uploads", func(t *testing.T) {
		w := bootCrestWorld(t, clanSeed{level: 3})
		for name, packet := range map[string][]byte{
			"wrong size":      encodeSetPledgeCrest(bytes.Repeat([]byte{1}, pledgeCrestSize-1)),
			"over the limit":  encodeSetPledgeCrest(bytes.Repeat([]byte{1}, pledgeCrestSize+1)),
			"large too short": encodeSetLargeCrest(bytes.Repeat([]byte{1}, pledgeCrestSize)),
		} {
			if frames := drainSend(t, w, packet); len(frames) != 0 {
				t.Fatalf("%s upload answered %x, want nothing", name, opcodes(frames))
			}
		}
		if storedCrest(t, w, "crest_id") != 0 || storedCrest(t, w, "crest_large_id") != 0 {
			t.Fatal("a silent upload stored a crest")
		}
		if entries, _ := os.ReadDir(w.dir); len(entries) != 0 {
			t.Fatalf("a silent upload wrote %d crest files", len(entries))
		}
	})
}

// TestBootDropsMissingCrests clears, as the clans are restored, every crest
// id whose image the crest directory does not hold, keeping the ones it
// does.
func TestBootDropsMissingCrests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Crest_700.dds"), bytes.Repeat([]byte{9}, pledgeCrestSize), 0o644); err != nil {
		t.Fatal(err)
	}
	crests, _, err := datacache.LoadCrests(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCrests(crests),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			for _, q := range []string{
				`UPDATE characters SET clanid = 268435456, power_grade = 0 WHERE char_name = 'Founder'`,
				`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id, crest_id, crest_large_id, ally_crest_id)
					SELECT 268435456, 'Seeded', 3, obj_Id, 700, 701, 702 FROM characters WHERE char_name = 'Founder'`,
			} {
				if _, err := db.ExecContext(context.Background(), q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}
		}),
	)
	srv.FlushPersistence(t)
	var crest, large, ally int64
	if err := srv.DB.QueryRowContext(context.Background(),
		`SELECT crest_id, crest_large_id, ally_crest_id FROM clan_data WHERE clan_id = ?`, seededClanID).Scan(&crest, &large, &ally); err != nil {
		t.Fatal(err)
	}
	if crest != 700 || large != 0 || ally != 0 {
		t.Fatalf("stored crests = %d/%d/%d, want 700/0/0", crest, large, ally)
	}
}

// drainSend sends packet from the founder and returns its answer.
func drainSend(t *testing.T, w *crestWorld, packet []byte) [][]byte {
	t.Helper()
	w.leader.Send(packet)
	return drainFrames(t, w.leader)
}

func anyContains(frames [][]byte, opcode byte, sub []byte) bool {
	for _, f := range frames {
		if f[0] == opcode && bytes.Contains(f, sub) {
			return true
		}
	}
	return false
}
