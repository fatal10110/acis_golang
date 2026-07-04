package serverpackets

// OpcodeServerList is the wire opcode for ServerList, listing the game
// servers a client may choose from.
const OpcodeServerList = 0x04

// ServerEntry is one game server row encoded into a ServerList packet.
// Callers assemble these from registered-gameserver and account state once
// that layer is ported (account/ban persistence and the gameserver
// registry, milestone M1) - this encoder only reproduces the wire bytes for
// already-resolved fields.
type ServerEntry struct {
	ID             byte
	IP             [4]byte
	Port           int32
	AgeLimit       byte
	PvP            bool
	CurrentPlayers uint16
	MaxPlayers     uint16
	Online         bool
	TestServer     bool
	ShowClock      bool
	ShowBrackets   bool
}

// EncodeServerList builds the ServerList packet listing servers, with
// lastServer marking the entry the client last played on.
func EncodeServerList(lastServer byte, servers []ServerEntry) []byte {
	w := newWriter(OpcodeServerList)
	w.WriteUint8(byte(len(servers)))
	w.WriteUint8(lastServer)
	for _, s := range servers {
		w.WriteUint8(s.ID)
		w.WriteBytes(s.IP[:])
		w.WriteInt32(s.Port)
		w.WriteUint8(s.AgeLimit)
		w.WriteUint8(boolByte(s.PvP))
		w.WriteInt16(s.CurrentPlayers)
		w.WriteInt16(s.MaxPlayers)
		w.WriteUint8(boolByte(s.Online))

		var bits int32
		if s.TestServer {
			bits |= 0x04
		}
		if s.ShowClock {
			bits |= 0x02
		}
		w.WriteInt32(bits)
		w.WriteUint8(boolByte(s.ShowBrackets))
	}
	return w.Bytes()
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
