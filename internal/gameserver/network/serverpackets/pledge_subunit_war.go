package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// Clan war and sub-unit extended opcodes.
const (
	OpcodeExPledgeReceiveWarList         uint16 = 0x003e
	OpcodeExPledgeReceiveSubPledgeCreate uint16 = 0x003f
)

// warListPageSize is how many clans one page of the attacker tab lists.
const warListPageSize = 13

// FramePledgeReceiveWarList builds one tab of the clan war window: tab 0
// the clans the clan declared war on, any other tab one page of the clans
// that declared war on it. size is the stored list's length, names the
// names of its clans that still exist, in order. The count field follows
// the stored list's length, not the names written.
func FramePledgeReceiveWarList(tab, page int32, size int, names []string) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExPledgeReceiveWarList)
	w.WriteInt32(tab)
	w.WriteInt32(page)
	switch {
	case tab == 0:
		w.WriteInt32(int32(size))
	case page == 0:
		w.WriteInt32(int32(min(size, warListPageSize)))
	default:
		w.WriteInt32(int32(size % (warListPageSize * int(page))))
	}
	for i, name := range names {
		if tab != 0 {
			if i < int(page)*warListPageSize {
				continue
			}
			if i == int(page+1)*warListPageSize {
				break
			}
		}
		w.WriteString(name)
		w.WriteInt32(tab)
		w.WriteInt32(page)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FramePledgeReceiveSubPledgeCreated builds the notice of a new sub-unit:
// its pledge type, name and captain's name ("" for none).
func FramePledgeReceiveSubPledgeCreated(pledgeType int32, name, leaderName string) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExPledgeReceiveSubPledgeCreate)
	w.WriteInt32(1)
	w.WriteInt32(pledgeType)
	w.WriteString(name)
	w.WriteString(leaderName)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
