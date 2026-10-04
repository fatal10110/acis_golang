package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// OpcodeExHeroList is the extended sub-opcode of ExHeroList.
const OpcodeExHeroList uint16 = 0x0023

// SystemMessageClanMemberS1BecameHeroAndGainedS2ReputationPoints tells a
// clan its member became a hero: string parameter the hero's name, number
// parameter the reputation the clan gained.
const SystemMessageClanMemberS1BecameHeroAndGainedS2ReputationPoints = 1776

// HeroListEntry is one hero in ExHeroList.
type HeroListEntry struct {
	Name      string
	ClassID   int32
	ClanName  string
	ClanCrest int32
	AllyName  string
	AllyCrest int32
	Count     int32
}

// FrameExHeroList lists the heroes of the running era.
func FrameExHeroList(heroes []HeroListEntry) wire.Frame {
	w := newFrameWriter(OpcodeExtended)
	w.WriteUint16(OpcodeExHeroList)
	w.WriteInt32(int32(len(heroes)))
	for _, h := range heroes {
		w.WriteString(h.Name)
		w.WriteInt32(h.ClassID)
		w.WriteString(h.ClanName)
		w.WriteInt32(h.ClanCrest)
		w.WriteString(h.AllyName)
		w.WriteInt32(h.AllyCrest)
		w.WriteInt32(h.Count)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
