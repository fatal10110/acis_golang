package npcs

import (
	"context"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSubclassReplaceRefusesASlotNotHeld pins #5063: the replace menu and
// the replace confirmation of a slot the player does not hold are refused
// with the closing ActionFailed alone. No subclass is added, the slots and
// the active class, the Spellhowler just added, stay as they were.
func TestSubclassReplaceRefusesASlotNotHeld(t *testing.T) {
	w := bootSubclassWorld(t)
	w.addSpellhowler(t)

	for _, command := range []string{
		"Subclass 6 2",
		"Subclass 7 2 " + strconv.Itoa(phantomSummoner),
		"Subclass 7 3 " + strconv.Itoa(phantomSummoner),
	} {
		w.openAnyNpcPage(t)
		if frames := w.command(t, command); len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("%s = %x, want ActionFailed alone", command, opcodes(frames))
		}
	}

	c := w.character(t)
	if c.ClassIndex() != 1 || c.ClassID() != spellhowler {
		t.Fatalf("active class = index %d class %d, want slot 1 class %d", c.ClassIndex(), c.ClassID(), spellhowler)
	}
	if subs := c.Subclasses(); len(subs) != 1 || subs[0].Index != 1 || subs[0].ClassID != spellhowler {
		t.Fatalf("subclasses = %+v, want Spellhowler in slot 1 alone", subs)
	}
	w.srv.FlushPersistence(t)
	rows, err := w.srv.DB.QueryContext(context.Background(), `SELECT class_index, class_id FROM character_subclasses WHERE char_obj_id = ? ORDER BY class_index`, w.player)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got [][2]int
	for rows.Next() {
		var index, classID int
		if err := rows.Scan(&index, &classID); err != nil {
			t.Fatal(err)
		}
		got = append(got, [2]int{index, classID})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != [2]int{1, spellhowler} {
		t.Fatalf("subclass rows = %v, want slot 1 Spellhowler alone", got)
	}
}
