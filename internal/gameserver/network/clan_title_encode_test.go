package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

func encodeRequestGiveNickName(name, title string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGiveNickName)
	w.WriteString(name)
	w.WriteString(title)
	return w.Bytes()
}
