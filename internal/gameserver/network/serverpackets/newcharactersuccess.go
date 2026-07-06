package serverpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// OpcodeNewCharacterSuccess is the wire opcode for NewCharacterSuccess,
// listing the professions the character-creation screen offers.
const OpcodeNewCharacterSuccess = 0x17

// creationScreenClassIDs are the profession ids the creation screen expects,
// in the exact order and count it expects them: 10 entries for 9 distinct
// root professions. The Human Fighter id (0) is listed twice — that
// duplication is the client's own contract, not a mistake introduced here.
var creationScreenClassIDs = [...]int{0, 0, 10, 18, 25, 31, 38, 44, 49, 53}

// EncodeNewCharacterSuccess builds the NewCharacterSuccess packet, looking
// up each creation-screen profession in templates. It returns an error if
// templates is missing one of them or can't resolve its race.
func EncodeNewCharacterSuccess(templates *player.TemplateTable) ([]byte, error) {
	w := newWriter(OpcodeNewCharacterSuccess)
	w.WriteInt32(int32(len(creationScreenClassIDs)))

	for _, id := range creationScreenClassIDs {
		tmpl, ok := templates.Get(id)
		if !ok {
			return nil, fmt.Errorf("serverpackets: NewCharacterSuccess: profession %d not loaded", id)
		}
		race, ok := player.ClassRace(tmpl.ID)
		if !ok {
			return nil, fmt.Errorf("serverpackets: NewCharacterSuccess: profession %d has no known race", tmpl.ID)
		}

		w.WriteInt32(int32(race))
		w.WriteInt32(int32(tmpl.ID))
		writeBaseStat(w, tmpl.STR)
		writeBaseStat(w, tmpl.DEX)
		writeBaseStat(w, tmpl.CON)
		writeBaseStat(w, tmpl.INT)
		writeBaseStat(w, tmpl.WIT)
		writeBaseStat(w, tmpl.MEN)
	}
	return w.Bytes(), nil
}

// writeBaseStat writes one profession-picker stat row: the profession's
// base value bracketed by two fixed values the client expects around it.
func writeBaseStat(w *wire.Writer, value int) {
	w.WriteInt32(0x46)
	w.WriteInt32(int32(value))
	w.WriteInt32(0x0a)
}
