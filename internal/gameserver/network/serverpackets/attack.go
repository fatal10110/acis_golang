package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const (
	// OpcodeAttack is the wire opcode for a physical attack animation.
	OpcodeAttack = 0x05

	// AttackHitSoulshot marks a hit using a soulshot charge.
	AttackHitSoulshot = 0x10
	// AttackHitCritical marks a critical hit.
	AttackHitCritical = 0x20
	// AttackHitShield marks a shield-blocked hit.
	AttackHitShield = 0x40
	// AttackHitMiss marks an evaded hit.
	AttackHitMiss = 0x80
)

// AttackHit is one target entry in an Attack packet.
type AttackHit struct {
	TargetID int32
	Damage   int
	Flags    uint8
}

// AttackSnapshot is the immutable data needed to broadcast one attack.
type AttackSnapshot struct {
	AttackerID int32
	X, Y, Z    int
	Hits       []AttackHit
}

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
