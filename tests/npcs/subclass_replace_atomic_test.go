package npcs

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// slotTables are the tables that keep rows per class index, the subclass
// row last.
var slotTables = []string{"character_hennas", "character_shortcuts", "character_skills_save", "character_skills", "character_subclasses"}

// storedSubclassLevel is the level slot 1's stored Spellhowler reached, past
// the level a fresh subclass starts at.
const storedSubclassLevel = 77

// TestSubclassReplaceFailureKeepsTheOldSubclassWhole fails a replace at a
// table past the first ones it clears, and at the new row's write: every
// row of the replaced subclass survives, the live character keeps the
// subclass at its progression, and the relogged character plays it as
// stored (#5074).
func TestSubclassReplaceFailureKeepsTheOldSubclassWhole(t *testing.T) {
	for _, tc := range []struct {
		name, event, table string
		// active replaces the slot played. Its saved effects are then the
		// live ones, which the save of the class left rewrites.
		active bool
	}{
		{"skills delete fails", "DELETE", "character_skills", false},
		{"subclass row delete fails", "DELETE", "character_subclasses", false},
		{"new row insert fails", "INSERT", "character_subclasses", false},
		{"active slot, skills delete fails", "DELETE", "character_skills", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
				exec(t, srv, `INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, 0, 0, ?, 1)`, objID, spellhowler, storedSubclassLevel)
				exec(t, srv, `INSERT INTO character_hennas (char_obj_id, symbol_id, slot, class_index) VALUES (?, 1, 1, 1)`, objID)
				exec(t, srv, `INSERT INTO character_shortcuts (char_obj_id, slot, page, type, id, level, class_index) VALUES (?, 0, 0, 'SKILL', ?, 1, 1)`, objID, learnedSkill)
				exec(t, srv, `INSERT INTO character_skills_save (char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index)
					VALUES (?, ?, 1, 1, 10, 0, 0, 0, 1, 1)`, objID, learnedSkill)
				exec(t, srv, `INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) VALUES (?, ?, 1, 1)`, objID, learnedSkill)
				if tc.active {
					exec(t, srv, `UPDATE characters SET classid = ? WHERE obj_Id = ?`, spellhowler, objID)
				}
			}, gameservertest.WithReuseDelays(0, 0)) // the relog selects the character again
			stored := slotRows(t, w)
			if tc.active {
				delete(stored, "character_skills_save")
			}
			for table, n := range stored {
				if n == 0 {
					t.Fatalf("test setup: no %s row for slot 1", table)
				}
			}

			allow := refuseSlotWrite(t, w, tc.event, tc.table)
			w.command(t, "Subclass 3")
			w.command(t, "Subclass 6 1")
			frames := w.command(t, "Subclass 7 1 "+strconv.Itoa(phantomSummoner))
			if n := len(frames); n < 2 || frames[n-2][0] != serverpackets.OpcodeSystemMessage {
				t.Fatalf("failed replace answer = %x, want the reverted text then ActionFailed", opcodes(frames))
			}
			if text := systemMessageText(t, frames[len(frames)-2]); text != "The sub class could not be added, you have been reverted to your base class." {
				t.Fatalf("failed replace message = %q, want the reverted text", text)
			}
			if info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)]); info.visibleClass != gladiatorClass {
				t.Fatalf("class after the failed replace = %d, want the base class %d", info.visibleClass, gladiatorClass)
			}
			w.srv.FlushPersistence(t)
			assertSlotOneKept(t, w, stored, "the failed replace")
			// The live character still holds the subclass the database kept.
			assertChangeMenuOffersSlotOne(t, w)

			// After a relog the character holds the same subclass, whole.
			w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
			if reply := w.c.Read(); reply[0] != serverpackets.OpcodeRestartResponse {
				t.Fatalf("restart opcode = %#x, want RestartResponse", reply[0])
			}
			if reply := w.c.Read(); reply[0] != serverpackets.OpcodeCharSelectInfo {
				t.Fatalf("post-restart opcode = %#x, want CharSelectInfo", reply[0])
			}
			w.srv.FlushPersistence(t)
			assertSlotOneKept(t, w, stored, "the logout save")
			allow()
			startInWorld(t, w.srv, w.c)
			w.selectFolk(t, w.folk)
			assertChangeMenuOffersSlotOne(t, w)
			frames = w.changeTo(t, 1)
			// UserInfo shows the base class; the level is the subclass's.
			if info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)]); info.level != storedSubclassLevel {
				t.Fatalf("level after the relog's change to slot 1 = %d, want %d", info.level, storedSubclassLevel)
			}
			if skills := skillListIDs(t, frames[lastIndex(frames, serverpackets.OpcodeSkillList)]); !slices.Contains(skills, learnedSkill) {
				t.Fatalf("slot 1 skills after the relog = %v, want %d kept", skills, learnedSkill)
			}
		})
	}
}

// assertSlotOneKept checks that w's slot 1 keeps the stored rows counted
// (a table missing from stored is not checked) and its Spellhowler row at
// the level it reached.
func assertSlotOneKept(t *testing.T, w *subclassWorld, stored map[string]int, after string) {
	t.Helper()
	got := slotRows(t, w)
	maps.DeleteFunc(got, func(table string, _ int) bool { _, ok := stored[table]; return !ok })
	if !maps.Equal(got, stored) {
		t.Fatalf("slot 1 rows after %s = %v, want %v kept", after, got, stored)
	}
	var classID, level int
	if err := w.srv.DB.QueryRowContext(context.Background(),
		`SELECT class_id, level FROM character_subclasses WHERE char_obj_id = ? AND class_index = 1`, w.player).Scan(&classID, &level); err != nil ||
		classID != spellhowler || level != storedSubclassLevel {
		t.Fatalf("slot 1 row after %s = class %d level %d, %v; want %d at %d", after, classID, level, err, spellhowler, storedSubclassLevel)
	}
}

// slotRows counts the rows each per-class table keeps for w's slot 1.
func slotRows(t *testing.T, w *subclassWorld) map[string]int {
	t.Helper()
	out := make(map[string]int, len(slotTables))
	for _, table := range slotTables {
		var n int
		if err := w.srv.DB.QueryRowContext(context.Background(),
			"SELECT COUNT(*) FROM "+table+" WHERE char_obj_id = ? AND class_index = 1", w.player).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		out[table] = n
	}
	return out
}

// assertChangeMenuOffersSlotOne opens the change menu, which links each
// subclass held.
func assertChangeMenuOffersSlotOne(t *testing.T, w *subclassWorld) {
	t.Helper()
	w.openMenu(t)
	if _, html := pageOf(t, w.command(t, "Subclass 2")); !strings.Contains(html, "Subclass 5 1") {
		t.Fatalf("change menu = %q, want slot 1 offered", html)
	}
}

// refuseSlotWrite makes the database refuse event on table for w's slot 1
// rows until the returned func drops the trigger. The trigger is dropped at
// cleanup too, so a pooled database never carries it into another test.
func refuseSlotWrite(t *testing.T, w *subclassWorld, event, table string) func() {
	t.Helper()
	ctx := context.Background()
	drop := func() {
		if _, err := w.srv.DB.ExecContext(ctx, "DROP TRIGGER IF EXISTS subclass_replace_fault"); err != nil {
			t.Errorf("drop fault trigger: %v", err)
		}
	}
	t.Cleanup(drop)
	row := "OLD"
	if event == "INSERT" {
		row = "NEW"
	}
	if _, err := w.srv.DB.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER subclass_replace_fault BEFORE %s ON %s FOR EACH ROW
		IF %s.char_obj_id = %d AND %s.class_index = 1 THEN
			SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected subclass replace fault';
		END IF`, event, table, row, w.player, row)); err != nil {
		t.Fatalf("create fault trigger: %v", err)
	}
	return drop
}
