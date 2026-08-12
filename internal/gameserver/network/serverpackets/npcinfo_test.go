package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestFrameNPCInfoWritesPvpFlagAndKarma(t *testing.T) {
	payload := framePayload(t, FrameNPCInfo(NPCInfoSnapshot{
		Name: "N", Title: "T", Summon: true, PvpFlag: 1, Karma: 500,
	}))
	fields := []byte{'N', 0, 0, 0, 'T', 0, 0, 0}
	offset := bytes.Index(payload, fields)
	if offset < 0 {
		t.Fatal("name/title fields missing")
	}
	got := payload[offset+len(fields):]
	if len(got) < 12 || binary.LittleEndian.Uint32(got[:4]) != 1 || binary.LittleEndian.Uint32(got[4:]) != 1 || binary.LittleEndian.Uint32(got[8:]) != 500 {
		t.Fatalf("summon/pvp/karma fields = %x, want 1/1/500", got)
	}
}

func TestFrameNPCInfoWritesAbnormalEffect(t *testing.T) {
	payload := framePayload(t, FrameNPCInfo(NPCInfoSnapshot{Name: "N", Title: "T", AbnormalEffect: 0x010000}))
	fields := []byte{'N', 0, 0, 0, 'T', 0, 0, 0}
	offset := bytes.Index(payload, fields)
	if offset < 0 {
		t.Fatal("name/title fields missing")
	}
	got := payload[offset+len(fields):]
	if len(got) < 16 || binary.LittleEndian.Uint32(got[12:16]) != 0x010000 {
		t.Fatalf("abnormal effect = %x, want 01000000", got)
	}
}
