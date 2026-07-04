package link

import (
	"encoding/binary"
	"testing"
)

func appendAttr(buf []byte, attr, value int32) []byte {
	buf = binary.LittleEndian.AppendUint32(buf, uint32(attr))
	return binary.LittleEndian.AppendUint32(buf, uint32(value))
}

func TestDecodeServerStatus(t *testing.T) {
	payload := binary.LittleEndian.AppendUint32([]byte{OpcodeServerStatus}, 4)
	payload = appendAttr(payload, 1, int32(ServerTypeNormal))
	payload = appendAttr(payload, 2, serverStatusOn)
	payload = appendAttr(payload, 4, 18)
	payload = appendAttr(payload, 7, 300)

	got, err := DecodeServerStatus(payload)
	if err != nil {
		t.Fatalf("DecodeServerStatus: %v", err)
	}
	if got.Status == nil || *got.Status != ServerTypeNormal {
		t.Errorf("Status = %v, want %v", got.Status, ServerTypeNormal)
	}
	if got.ShowClock == nil || *got.ShowClock != true {
		t.Errorf("ShowClock = %v, want true", got.ShowClock)
	}
	if got.ShowBrackets != nil {
		t.Errorf("ShowBrackets = %v, want nil (not sent)", got.ShowBrackets)
	}
	if got.AgeLimit == nil || *got.AgeLimit != 18 {
		t.Errorf("AgeLimit = %v, want 18", got.AgeLimit)
	}
	if got.MaxPlayers == nil || *got.MaxPlayers != 300 {
		t.Errorf("MaxPlayers = %v, want 300", got.MaxPlayers)
	}
}

func TestDecodeServerStatusEmpty(t *testing.T) {
	payload := binary.LittleEndian.AppendUint32([]byte{OpcodeServerStatus}, 0)
	got, err := DecodeServerStatus(payload)
	if err != nil {
		t.Fatalf("DecodeServerStatus: %v", err)
	}
	if got != (ServerStatus{}) {
		t.Fatalf("DecodeServerStatus() = %+v, want zero value", got)
	}
}

func TestDecodeServerStatusShort(t *testing.T) {
	payload := binary.LittleEndian.AppendUint32([]byte{OpcodeServerStatus}, 5)
	if _, err := DecodeServerStatus(payload); err == nil {
		t.Error("DecodeServerStatus: want error on truncated payload, got nil")
	}
}

func TestServerTypeString(t *testing.T) {
	tests := map[ServerType]string{
		ServerTypeAuto:   "Auto",
		ServerTypeGood:   "Good",
		ServerTypeNormal: "Normal",
		ServerTypeFull:   "Full",
		ServerTypeDown:   "Down",
		ServerTypeGMOnly: "Gm Only",
	}
	for st, want := range tests {
		if got := st.String(); got != want {
			t.Errorf("ServerType(%d).String() = %q, want %q", st, got, want)
		}
	}
}
