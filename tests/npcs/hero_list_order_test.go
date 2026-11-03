package npcs

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// orderHero is a hero of the running era seeded for an order test.
type orderHero struct {
	id    int32
	class int
	name  string
}

// heroListNames boots the monument with heroes seeded in that order and
// returns the names "Olympiad 4" lists.
func heroListNames(t *testing.T, heroes []orderHero) []string {
	t.Helper()
	var chars, rows []string
	for _, h := range heroes {
		chars = append(chars, fmt.Sprintf("('others', %d, '%s', 0)", h.id, h.name))
		rows = append(rows, fmt.Sprintf("(%d, %d, 1, 1, 0)", h.id, h.class))
	}
	w, monument := bootMonument(t, 76,
		gameservertest.WithClanSeed(seedStatements(t,
			`INSERT INTO characters (account_name, obj_Id, char_name, clanid) VALUES `+strings.Join(chars, ", "))),
		gameservertest.WithOlympiadSeed(seedStatements(t,
			`INSERT INTO heroes (char_id, class_id, count, played, active) VALUES `+strings.Join(rows, ", "))))
	w.talkPage(t, monument, false)
	frames := w.bypass(t, npcCommand(monument, "Olympiad 4"))
	if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeExtended {
		t.Fatalf("Olympiad 4 answer = %x, want ExHeroList first", opcodes(frames))
	}
	r := wire.NewReader(frames[0][1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExHeroList {
		t.Fatalf("sub-opcode = %#x, want ExHeroList", sub)
	}
	var names []string
	for range int(r.ReadInt32()) {
		names = append(names, r.ReadString())
		r.ReadInt32()
		r.ReadString()
		r.ReadInt32()
		r.ReadString()
		r.ReadInt32()
		r.ReadInt32()
	}
	return names
}

// TestHeroListInTableOrder pins the order ExHeroList lists the heroes in:
// the order of the hash map the heroes are kept in, keyed by object id,
// not by class. A bucket keeps its heroes in the order they were loaded,
// by ascending object id; the thirteenth hero doubles the table from 16
// buckets to 32.
func TestHeroListInTableOrder(t *testing.T) {
	t.Parallel()
	t.Run("shared bucket", func(t *testing.T) {
		t.Parallel()
		// Both ids fall in bucket 1 of 16; the lower id is loaded first.
		got := heroListNames(t, []orderHero{{0x10000011, 88, "Eleven"}, {0x10000001, 90, "One"}})
		if want := []string{"One", "Eleven"}; !slices.Equal(got, want) {
			t.Fatalf("ExHeroList = %v, want %v", got, want)
		}
	})
	t.Run("grown table", func(t *testing.T) {
		t.Parallel()
		// Each id's bucket of 32 is its low five bits; ascending id and
		// class order run with k, the table order with the bucket.
		buckets := []int32{17, 3, 20, 1, 30, 5, 16, 9, 2, 25, 12, 7, 18}
		var heroes []orderHero
		for k, b := range buckets {
			heroes = append(heroes, orderHero{0x10000000 + 0x100*int32(k) + b, 88 + k, fmt.Sprintf("Hero%c", 'A'+k)})
		}
		got := heroListNames(t, heroes)
		var want []string
		for _, k := range []int{3, 8, 1, 5, 11, 7, 10, 6, 0, 12, 2, 9, 4} {
			want = append(want, heroes[k].name)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("ExHeroList = %v, want %v", got, want)
		}
	})
}
