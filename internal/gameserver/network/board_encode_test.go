package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

func encodeRequestShowBoard() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestShowBoard)
	w.WriteInt32(0)
	return w.Bytes()
}

// encodeRequestBBSWrite writes a board form: url, then the five arguments,
// args first and the rest empty.
func encodeRequestBBSWrite(url string, args ...string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBBSWrite)
	w.WriteString(url)
	for i := range 5 {
		arg := ""
		if i < len(args) {
			arg = args[i]
		}
		w.WriteString(arg)
	}
	return w.Bytes()
}
