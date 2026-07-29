package clientpackets

import "testing"

// These plaintext packets are fixed vectors from the Java readers in
// aCis_gameserver/java/net/sf/l2j/loginserver/network/clientpackets.
func TestJavaOracleClientPacketVectors(t *testing.T) {
	guard, err := DecodeAuthGameGuard([]byte{
		OpcodeAuthGameGuard, 0x44, 0x33, 0x22, 0x11, 0x88, 0x77, 0x66, 0x55,
		0xcc, 0xbb, 0xaa, 0x99, 0x00, 0xff, 0xee, 0xdd, 0x78, 0x56, 0x34, 0x12,
	})
	if err != nil || guard != (AuthGameGuard{0x11223344, 0x55667788, -0x66554434, -0x22110100, 0x12345678}) {
		t.Fatalf("DecodeAuthGameGuard() = %#v, %v", guard, err)
	}

	list, err := DecodeRequestServerList([]byte{OpcodeRequestServerList, 0x44, 0x33, 0x22, 0x11, 0xcc, 0xbb, 0xaa, 0x99})
	if err != nil || list != (RequestServerList{0x11223344, -0x66554434}) {
		t.Fatalf("DecodeRequestServerList() = %#v, %v", list, err)
	}

	login, err := DecodeRequestServerLogin([]byte{OpcodeRequestServerLogin, 0x44, 0x33, 0x22, 0x11, 0xcc, 0xbb, 0xaa, 0x99, 0x07})
	if err != nil || login != (RequestServerLogin{0x11223344, -0x66554434, 7}) {
		t.Fatalf("DecodeRequestServerLogin() = %#v, %v", login, err)
	}
}
