package link

// OpcodeKickPlayer is the wire opcode for KickPlayer, telling a game server
// to disconnect an account.
const OpcodeKickPlayer = 0x04

// EncodeKickPlayer builds the KickPlayer packet for account.
func EncodeKickPlayer(account string) []byte {
	w := newWriter(OpcodeKickPlayer)
	w.writeString(account)
	return w.bytes()
}
