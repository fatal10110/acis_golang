package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
)

const (
	// OpcodeAttack is the wire opcode for a physical attack animation.
	OpcodeAttack = 0x05

	// AttackHitSoulshot marks a hit using a soulshot charge.
	AttackHitSoulshot = attack.HitSoulshot
	// AttackHitCritical marks a critical hit.
	AttackHitCritical = attack.HitCritical
	// AttackHitShield marks a shield-blocked hit.
	AttackHitShield = attack.HitShield
	// AttackHitMiss marks an evaded hit.
	AttackHitMiss = attack.HitMiss
)

// AttackHit is one target entry in an Attack packet.
type AttackHit = attack.SnapshotHit

// AttackSnapshot is the immutable data needed to broadcast one attack.
type AttackSnapshot = attack.Snapshot

// FrameAttack builds an Attack packet as an owned frame.
func FrameAttack(s AttackSnapshot) wire.Frame {
	w := newFrameWriter(OpcodeAttack)
	writeAttack(w, s)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeAttack(w *wire.Writer, s AttackSnapshot) {
	if len(s.Hits) == 0 {
		return
	}

	first := s.Hits[0]
	w.WriteInt32(s.AttackerID)
	w.WriteInt32(first.TargetID)
	w.WriteInt32(int32(first.Damage))
	w.WriteUint8(first.Flags)
	w.WriteInt32(int32(s.X))
	w.WriteInt32(int32(s.Y))
	w.WriteInt32(int32(s.Z))
	w.WriteUint16(uint16(len(s.Hits) - 1))

	for _, hit := range s.Hits[1:] {
		w.WriteInt32(hit.TargetID)
		w.WriteInt32(int32(hit.Damage))
		w.WriteUint8(hit.Flags)
	}
}
