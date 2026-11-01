package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// charInfoTitleEffect returns a CharInfo frame's object id, title and abnormal
// effect mask, read field by field.
func charInfoTitleEffect(t *testing.T, frame []byte) (int32, string, int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("opcode = %#x, want CharInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	r.ReadBytes(4 * 4) // x, y, z, boat
	id := r.ReadInt32()
	r.ReadString()        // name
	r.ReadBytes(3 * 4)    // race, sex, class
	r.ReadBytes(12 * 4)   // paperdoll
	r.ReadBytes(4*2 + 4)  // padding, right-hand augmentation
	r.ReadBytes(12*2 + 4) // padding, left-hand augmentation
	r.ReadBytes(4 * 2)    // padding
	r.ReadBytes(6 * 4)    // pvp flag, karma, casting/attack speed, pvp flag, karma
	r.ReadBytes(8 * 4)    // speeds
	r.ReadBytes(4 * 8)    // speed multipliers, collision
	r.ReadBytes(3 * 4)    // hair style, hair color, face
	title := r.ReadString()
	r.ReadBytes(5 * 4) // clan, crest, ally, ally crest, relation
	r.ReadBytes(5)     // standing, running, in combat, alike dead, invisible
	r.ReadBytes(2)     // mount type, operate type
	cubics := r.ReadUint16()
	r.ReadBytes(int(cubics) * 2)
	r.ReadBytes(1) // party room
	effect := r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("decode CharInfo: %v", err)
	}
	return id, title, effect
}

// stealthEffect is the abnormal effect bit that draws a player in stealth.
const stealthEffect = 0x100000

// TestHiddenGMShowsInvisibleTitleToGMViewers pins what a game master sees of
// an invisible player: it is drawn visible, in stealth, under the title
// "Invisible", while a plain player gets it hidden with its own title and
// effects.
func TestHiddenGMShowsInvisibleTitleToGMViewers(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	watcher, _ := addPlayer(t, srv, "player2", "Watcher", userLevel)
	otherGM, _ := addPlayer(t, srv, "player3", "OtherGM", adminLevel)
	drain(t, gm)
	drain(t, watcher)
	drain(t, otherGM)

	exchange(t, gm, encodeBuildCmd("hide"))
	settle(t, watcher)
	settle(t, otherGM)

	// A walk/run toggle rebroadcasts the hidden GM's CharInfo to both.
	exchange(t, gm, encodeActionUse(actionWalkRun))
	for who, want := range map[string]struct {
		frames  [][]byte
		title   string
		stealth bool
	}{
		"OtherGM": {settle(t, otherGM), "Invisible", true},
		"Watcher": {settle(t, watcher), "Admin", false}, // the title bootAdmin gives its game master
	} {
		var infos [][]byte
		for _, f := range want.frames {
			if f[0] != serverpackets.OpcodeCharInfo {
				continue
			}
			if id, _, _ := charInfoTitleEffect(t, f); id == gmID {
				infos = append(infos, f)
			}
		}
		if len(infos) != 1 {
			t.Fatalf("%s: got %d CharInfo of the GM, want 1", who, len(infos))
		}
		_, title, effect := charInfoTitleEffect(t, infos[0])
		if title != want.title {
			t.Errorf("%s: CharInfo title = %q, want %q", who, title, want.title)
		}
		if got := effect&stealthEffect != 0; got != want.stealth {
			t.Errorf("%s: CharInfo stealth effect = %v, want %v (mask %#x)", who, got, want.stealth, effect)
		}
	}
}

// TestHiddenGMIsShownOnEnterRangeByViewerKind pins the first-sight CharInfo
// (a viewer entering the world after the //hide): a game master viewer gets
// the hidden GM under the title "Invisible" in stealth, a plain viewer the
// GM's own title with no stealth.
func TestHiddenGMIsShownOnEnterRangeByViewerKind(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	exchange(t, gm, encodeBuildCmd("hide"))
	settle(t, gm)

	for _, tc := range []struct {
		account, name string
		level         int
		title         string
		stealth       bool
	}{
		{"player2", "LateGM", adminLevel, "Invisible", true},
		{"player3", "LatePlayer", userLevel, "Admin", false},
	} {
		ch := srv.SeedCharacterFor(t, tc.account, tc.name, 1, 0)
		setAccessLevel(t, srv, ch.ID, tc.level)
		viewer := srv.DialClient(t, tc.account, 1)
		frames := enterWorldFrames(t, viewer)

		var infos [][]byte
		for _, f := range frames {
			if f[0] != serverpackets.OpcodeCharInfo {
				continue
			}
			if id, _, _ := charInfoTitleEffect(t, f); id == gmID {
				infos = append(infos, f)
			}
		}
		if len(infos) != 1 {
			t.Fatalf("%s: got %d CharInfo of the hidden GM, want 1", tc.name, len(infos))
		}
		_, title, effect := charInfoTitleEffect(t, infos[0])
		if title != tc.title {
			t.Errorf("%s: CharInfo title = %q, want %q", tc.name, title, tc.title)
		}
		if got := effect&stealthEffect != 0; got != tc.stealth {
			t.Errorf("%s: CharInfo stealth effect = %v, want %v (mask %#x)", tc.name, got, tc.stealth, effect)
		}
	}
}
