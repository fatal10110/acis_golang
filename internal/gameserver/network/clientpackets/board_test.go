package clientpackets

import "testing"

// RequestBBSwrite reads six strings: the url, then arg1 to arg5.
func TestDecodeRequestBBSWrite(t *testing.T) {
	payload := []byte{
		OpcodeRequestBBSWrite,
		'M', 0, 'a', 0, 'i', 0, 'l', 0, 0, 0,
		'S', 0, 'e', 0, 'n', 0, 'd', 0, 0, 0,
		'0', 0, 0, 0,
		'B', 0, 'o', 0, 'b', 0, 0, 0,
		'H', 0, 'i', 0, 0, 0,
		0, 0,
	}
	got, err := DecodeRequestBBSWrite(payload)
	if err != nil {
		t.Fatalf("DecodeRequestBBSWrite: %v", err)
	}
	want := RequestBBSWrite{URL: "Mail", Args: [5]string{"Send", "0", "Bob", "Hi", ""}}
	if got != want {
		t.Fatalf("DecodeRequestBBSWrite = %+v, want %+v", got, want)
	}
}

func TestDecodeRequestBBSWriteShort(t *testing.T) {
	if _, err := DecodeRequestBBSWrite([]byte{OpcodeRequestBBSWrite, 'M', 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestBBSWrite: want error when arguments are missing")
	}
}

// RequestShowBoard carries one int the server ignores.
func TestDecodeRequestShowBoard(t *testing.T) {
	if _, err := DecodeRequestShowBoard([]byte{OpcodeRequestShowBoard, 1, 0, 0, 0}); err != nil {
		t.Fatalf("DecodeRequestShowBoard: %v", err)
	}
	if _, err := DecodeRequestShowBoard([]byte{OpcodeRequestShowBoard, 1}); err == nil {
		t.Fatal("DecodeRequestShowBoard: want error on a short payload")
	}
}
