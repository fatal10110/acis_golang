package character

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// createReply sends a RequestCharacterCreate and returns the CharCreateFail
// reason, or -1 for CharCreateOk (whose refreshed CharSelectInfo it reads).
func createReply(t *testing.T, c *testsupport.ScriptedClient, name string, sex int32, hairStyle byte) int32 {
	t.Helper()
	c.Send(encodeRequestCharacterCreate(name, 0, sex, 0, hairStyle, 0, 0))
	reply := c.Read()
	switch reply[0] {
	case serverpackets.OpcodeCharCreateOk:
		if next := c.Read(); next[0] != serverpackets.OpcodeCharSelectInfo {
			t.Fatalf("%s: frame after CharCreateOk = %#x, want CharSelectInfo", name, next[0])
		}
		return -1
	case serverpackets.OpcodeCharCreateFail:
		return int32(binary.LittleEndian.Uint32(reply[1:5]))
	default:
		t.Fatalf("%s: reply opcode = %#x, want CharCreateOk or CharCreateFail", name, reply[0])
		return 0
	}
}

// TestCreateSexTruncatedToByte pins the creation request's sex field
// (RequestCharacterCreate.java): only its low byte counts, so 256 and 257
// create a male and a female, and 2 creates an etc character. A sex past etc
// fails only once every other check has passed (Java throws at
// Player.create, after them), so a bad name or a taken name with a bad sex
// still answers the name's own failure, and a bad hair style the hair's.
func TestCreateSexTruncatedToByte(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Taken", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client

	failed := int32(serverpackets.CharCreateFailReasonCreationFailed)
	for _, tc := range []struct {
		label     string
		name      string
		sex       int32
		hairStyle byte
		want      int32
	}{
		{"sex 3", "Sexthree", 3, 0, failed},
		{"sex -1", "Sexminus", -1, 0, failed},
		{"sex 127", "Sexmax", 127, 0, failed},
		{"bad name, sex 3", "bad name!", 3, 0, int32(serverpackets.CharCreateFailReasonIncorrectName)},
		{"taken name, sex -1", "Taken", -1, 0, int32(serverpackets.CharCreateFailReasonNameAlreadyExists)},
		// -1 is no male, so the hair style limit is the female one (6).
		{"hair 7, sex -1", "Hairseven", -1, 7, failed},
		{"hair 5, sex 0x100", "Hairfive", 0x100, 5, failed},
	} {
		if got := createReply(t, c, tc.name, tc.sex, tc.hairStyle); got != tc.want {
			t.Fatalf("%s: reply = %d, want %d", tc.label, got, tc.want)
		}
	}

	for _, tc := range []struct {
		name    string
		sex     int32
		wantSex int
	}{
		{"Male", 0x100, 0},
		{"Female", 0x101, 1},
		{"Etc", 2, 2},
	} {
		if got := createReply(t, c, tc.name, tc.sex, 0); got != -1 {
			t.Fatalf("%s (sex %#x): CharCreateFail reason %d, want CharCreateOk", tc.name, tc.sex, got)
		}
		var sex int
		if err := srv.DB.QueryRowContext(context.Background(),
			"SELECT sex FROM characters WHERE char_name = ?", tc.name).Scan(&sex); err != nil {
			t.Fatalf("%s: read sex: %v", tc.name, err)
		}
		if sex != tc.wantSex {
			t.Fatalf("%s (sex %#x): stored sex = %d, want %d", tc.name, tc.sex, sex, tc.wantSex)
		}
	}
	var stray int
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM characters WHERE char_name IN ('Sexthree','Sexminus','Sexmax','Hairseven','Hairfive')").Scan(&stray); err != nil {
		t.Fatalf("count refused characters: %v", err)
	}
	if stray != 0 {
		t.Fatalf("%d refused characters stored, want 0", stray)
	}
}
