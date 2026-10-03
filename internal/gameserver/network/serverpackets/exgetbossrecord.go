package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeExGetBossRecord is the extended sub-opcode of ExGetBossRecord.
const OpcodeExGetBossRecord uint16 = 0x0033

// SystemMessageRaidWasSuccessful is shown around a raid boss killed by a
// player.
const SystemMessageRaidWasSuccessful = 1209

// SoundRaidWasSuccessful is the sound played with
// SystemMessageRaidWasSuccessful.
const SoundRaidWasSuccessful = "systemmsg_e.1209"

// BossRecordEntry is one boss in a player's raid point record.
type BossRecordEntry struct {
	BossID, Points int32
}

// FrameExGetBossRecord builds a player's raid point record: its rank, its
// total points and its points per boss. hasRecord false means the player
// has no record, which is written as an empty list padded with three
// zeros.
func FrameExGetBossRecord(rank, total int32, entries []BossRecordEntry, hasRecord bool) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExGetBossRecord)
	w.WriteInt32(rank)
	w.WriteInt32(total)
	if !hasRecord {
		w.WriteInt32(0)
		w.WriteInt32(0)
		w.WriteInt32(0)
		w.WriteInt32(0)
		return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
	}
	w.WriteInt32(int32(len(entries)))
	for _, e := range entries {
		w.WriteInt32(e.BossID)
		w.WriteInt32(e.Points)
		w.WriteInt32(0)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
