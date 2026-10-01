package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// FrameExShowQuestInfo opens the client's own quest information window.
func FrameExShowQuestInfo() wire.Frame {
	return frameExtendedOnly(OpcodeExShowQuestInfo)
}
