package quest

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeRequestCharacterCreate(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	for range 9 {
		w.WriteInt32(0)
	}
	w.WriteInt32(1)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeRequestCharacterDelete(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterDelete)
	w.WriteInt32(slot)
	return w.Bytes()
}

func encodeSingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}

// readUntil reads frames until one with opcode arrives and returns it.
func readUntil(t *testing.T, c *testsupport.ScriptedClient, opcode byte) []byte {
	t.Helper()
	for range 100 {
		if frame := c.Read(); frame[0] == opcode {
			return frame
		}
	}
	t.Fatalf("no frame with opcode %#x within 100 frames", opcode)
	return nil
}

// questLine is one parsed QuestList entry.
type questLine struct{ id, flags int32 }

func parseQuestList(t *testing.T, frame []byte) []questLine {
	t.Helper()
	if frame[0] != serverpackets.OpcodeQuestList {
		t.Fatalf("opcode = %#x, want QuestList (%#x)", frame[0], serverpackets.OpcodeQuestList)
	}
	r := wire.NewReader(frame[1:])
	n := int(r.ReadUint16())
	var out []questLine
	for range n {
		out = append(out, questLine{r.ReadInt32(), r.ReadInt32()})
	}
	if err := r.Err(); err != nil || r.Remaining() != 0 {
		t.Fatalf("parse QuestList %x: err %v, %d bytes left", frame, err, r.Remaining())
	}
	return out
}

func assertQuestList(t *testing.T, what string, frame []byte, want []questLine) {
	t.Helper()
	if got := parseQuestList(t, frame); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s QuestList = %v, want %v", what, got, want)
	}
}

// journalRow is one character_quests row.
type journalRow struct {
	charID         int32
	name, variable string
	value          sql.NullString
}

func insertJournal(t *testing.T, db *sql.DB, rows ...journalRow) {
	t.Helper()
	for _, r := range rows {
		if _, err := db.ExecContext(context.Background(),
			"INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?)", r.charID, r.name, r.variable, r.value); err != nil {
			t.Fatalf("insert %+v: %v", r, err)
		}
	}
}

func readJournal(t *testing.T, db *sql.DB) []journalRow {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "SELECT charId,name,var,value FROM character_quests ORDER BY charId,name,var")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []journalRow
	for rows.Next() {
		var r journalRow
		if err := rows.Scan(&r.charID, &r.name, &r.variable, &r.value); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func val(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
