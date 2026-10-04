package npcs

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/classmaster"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The subclass scenario plays a Gladiator (class 2) whose active subclass,
// slot 1, is a level 76 Spellhowler taking its third occupation, Storm
// Screamer (class 110).
const (
	stormScreamer      = 110
	subclassThirdLevel = 76
	smThirdClass       = 1606
)

// TestClassManagerChangeOnASubclassKeepsTheBaseClass pins an occupation
// change made while a subclass is active: the subclass slot takes the new
// class, while the base class, its body and the class the client shows
// stay the Gladiator, and the next save writes them so.
func TestClassManagerChangeOnASubclassKeepsTheBaseClass(t *testing.T) {
	jobs, err := classmaster.ParseJobs("3;[57(" + strconv.Itoa(classPrice) + ")];[]")
	if err != nil {
		t.Fatal(err)
	}
	screamer := gameservertest.ClassTemplate()
	screamer.ID = stormScreamer
	screamer.Skills = nil
	screamer.SafeFallHeightMale, screamer.SafeFallHeightFemale = 400, 420
	templates := append(darkMysticLine(), screamer)

	w := bootClassManagerWorldWith(t, classPrice, func(srv *gameservertest.Server, objID int32) {
		exec(t, srv, `UPDATE characters SET classid = ?, base_class = ? WHERE obj_Id = ?`, spellhowler, gladiatorClass, objID)
		exec(t, srv, `INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, 0, 0, ?, 1)`, objID, spellhowler, subclassThirdLevel)
	}, gameservertest.WithClassMaster(classmaster.NewConfig(false, jobs)), gameservertest.WithClassTemplates(templates...))

	w.command(t, "3rdClass")
	frames := w.command(t, "change_class "+strconv.Itoa(stormScreamer))
	if _, messages := changeOrder(frames); !slices.Contains(messages, smThirdClass) {
		t.Fatalf("change answer = %x messages %v, want THIRD_CLASS_TRANSFER", opcodes(frames), messages)
	}
	cast := frames[firstIndex(frames, serverpackets.OpcodeMagicSkillUse)]
	if skill := wire.NewReader(cast[9:]).ReadInt32(); skill != occupationChangeSkill {
		t.Fatalf("MagicSkillUse skill = %d, want %d", skill, occupationChangeSkill)
	}
	// The client keeps showing the base class.
	if info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)]); info.visibleClass != gladiatorClass || info.level != subclassThirdLevel {
		t.Fatalf("UserInfo class/level = %d/%d, want base class %d at %d", info.visibleClass, info.level, gladiatorClass, subclassThirdLevel)
	}

	// The body stays the Gladiator's: a 300-unit drop hurts, where the
	// Storm Screamer template would fall it safely.
	w.c.Send(encodeValidatePosition(location.Location{X: w.at.X, Y: w.at.Y, Z: w.at.Z - 300}))
	fall := drainFrames(t, w.c)
	if i := firstIndex(fall, serverpackets.OpcodeSystemMessage); i < 0 || systemMessageID(fall[i]) != 296 {
		t.Fatalf("300-unit drop answer = %x, want FALL_DAMAGE_S1", opcodes(fall))
	}

	w.srv.TickAutosave(t)
	w.srv.FlushPersistence(t)
	var classID, baseClass, slotClass int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT classid, base_class FROM characters WHERE obj_Id = ?`, w.player).Scan(&classID, &baseClass); err != nil {
		t.Fatal(err)
	}
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT class_id FROM character_subclasses WHERE char_obj_id = ? AND class_index = 1`, w.player).Scan(&slotClass); err != nil {
		t.Fatal(err)
	}
	if classID != stormScreamer || baseClass != gladiatorClass || slotClass != stormScreamer {
		t.Fatalf("saved class/base class/slot 1 = %d/%d/%d, want %d/%d/%d", classID, baseClass, slotClass, stormScreamer, gladiatorClass, stormScreamer)
	}
}
