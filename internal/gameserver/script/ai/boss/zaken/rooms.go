package zaken

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// roomCorner returns the corner of room i, 1 to 15, of Zaken's ship: five
// rooms on each of its three decks. Any other i gives the zero point.
func roomCorner(i int) script.Loc {
	switch i {
	case 1:
		return script.Loc{X: 53950, Y: 219860, Z: -3488}
	case 2:
		return script.Loc{X: 55980, Y: 219820, Z: -3488}
	case 3:
		return script.Loc{X: 54950, Y: 218790, Z: -3488}
	case 4:
		return script.Loc{X: 55970, Y: 217770, Z: -3488}
	case 5:
		return script.Loc{X: 53930, Y: 217760, Z: -3488}
	case 6:
		return script.Loc{X: 55970, Y: 217770, Z: -3216}
	case 7:
		return script.Loc{X: 55980, Y: 219920, Z: -3216}
	case 8:
		return script.Loc{X: 54960, Y: 218790, Z: -3216}
	case 9:
		return script.Loc{X: 53950, Y: 219860, Z: -3216}
	case 10:
		return script.Loc{X: 53930, Y: 217760, Z: -3216}
	case 11:
		return script.Loc{X: 55970, Y: 217770, Z: -2944}
	case 12:
		return script.Loc{X: 55980, Y: 219920, Z: -2944}
	case 13:
		return script.Loc{X: 54960, Y: 218790, Z: -2944}
	case 14:
		return script.Loc{X: 53950, Y: 219860, Z: -2944}
	case 15:
		return script.Loc{X: 53930, Y: 217760, Z: -2944}
	}
	return script.Loc{}
}

// roomPoint draws a point of room i: its corner moved by 0 to 649 on X,
// drawn first, then on Y.
func roomPoint(s *script.Script, i int) script.Loc {
	at := roomCorner(i)
	at.X += s.Rnd(650)
	at.Y += s.Rnd(650)
	return at
}

// randomRoomPoint draws a room, one of the first 14, then a point of it.
func randomRoomPoint(s *script.Script) script.Loc {
	return roomPoint(s, s.Rnd(14)+1)
}

// private is one private of a wave placed at a fixed point.
type private struct {
	id int32
	at script.Loc
}

// fixedWave returns the privates wave 5, 6 or 7 places at fixed points, in
// the order they appear; nil for any other wave.
func fixedWave(wave int32) []private {
	switch wave {
	case 5:
		return []private{
			{29023, script.Loc{X: 52675, Y: 219371, Z: -3290}},
			{29023, script.Loc{X: 52687, Y: 219596, Z: -3368}},
			{29023, script.Loc{X: 52672, Y: 219740, Z: -3418}},
			{29027, script.Loc{X: 52857, Y: 219992, Z: -3488}},
			{29026, script.Loc{X: 52959, Y: 219997, Z: -3488}},
			{29024, script.Loc{X: 53381, Y: 220151, Z: -3488}},
			{29026, script.Loc{X: 54236, Y: 220948, Z: -3488}},
			{29027, script.Loc{X: 54885, Y: 220144, Z: -3488}},
			{29027, script.Loc{X: 55264, Y: 219860, Z: -3488}},
			{29026, script.Loc{X: 55399, Y: 220263, Z: -3488}},
			{29027, script.Loc{X: 55679, Y: 220129, Z: -3488}},
			{29024, script.Loc{X: 56276, Y: 220783, Z: -3488}},
			{29024, script.Loc{X: 57173, Y: 220234, Z: -3488}},
			{29027, script.Loc{X: 56267, Y: 218826, Z: -3488}},
			{29023, script.Loc{X: 56294, Y: 219482, Z: -3488}},
			{29026, script.Loc{X: 56094, Y: 219113, Z: -3488}},
			{29023, script.Loc{X: 56364, Y: 218967, Z: -3488}},
			{29027, script.Loc{X: 57113, Y: 218079, Z: -3488}},
			{29023, script.Loc{X: 56186, Y: 217153, Z: -3488}},
			{29027, script.Loc{X: 55440, Y: 218081, Z: -3488}},
			{29026, script.Loc{X: 55202, Y: 217940, Z: -3488}},
			{29027, script.Loc{X: 55225, Y: 218236, Z: -3488}},
			{29027, script.Loc{X: 54973, Y: 218075, Z: -3488}},
			{29026, script.Loc{X: 53412, Y: 218077, Z: -3488}},
			{29024, script.Loc{X: 54226, Y: 218797, Z: -3488}},
			{29024, script.Loc{X: 54394, Y: 219067, Z: -3488}},
			{29027, script.Loc{X: 54139, Y: 219253, Z: -3488}},
			{29023, script.Loc{X: 54262, Y: 219480, Z: -3488}},
		}
	case 6:
		return []private{
			{29027, script.Loc{X: 53412, Y: 218077, Z: -3488}},
			{29024, script.Loc{X: 54413, Y: 217132, Z: -3488}},
			{29023, script.Loc{X: 54841, Y: 217132, Z: -3488}},
			{29023, script.Loc{X: 55372, Y: 217128, Z: -3343}},
			{29023, script.Loc{X: 55893, Y: 217122, Z: -3488}},
			{29026, script.Loc{X: 56282, Y: 217237, Z: -3216}},
			{29024, script.Loc{X: 56963, Y: 218080, Z: -3216}},
			{29027, script.Loc{X: 56267, Y: 218826, Z: -3216}},
			{29023, script.Loc{X: 56294, Y: 219482, Z: -3216}},
			{29026, script.Loc{X: 56094, Y: 219113, Z: -3216}},
			{29023, script.Loc{X: 56364, Y: 218967, Z: -3216}},
			{29024, script.Loc{X: 56276, Y: 220783, Z: -3216}},
			{29024, script.Loc{X: 57173, Y: 220234, Z: -3216}},
			{29027, script.Loc{X: 54885, Y: 220144, Z: -3216}},
			{29027, script.Loc{X: 55264, Y: 219860, Z: -3216}},
			{29026, script.Loc{X: 55399, Y: 220263, Z: -3216}},
			{29027, script.Loc{X: 55679, Y: 220129, Z: -3216}},
			{29026, script.Loc{X: 54236, Y: 220948, Z: -3216}},
			{29026, script.Loc{X: 54464, Y: 219095, Z: -3216}},
			{29024, script.Loc{X: 54226, Y: 218797, Z: -3216}},
			{29024, script.Loc{X: 54394, Y: 219067, Z: -3216}},
			{29027, script.Loc{X: 54139, Y: 219253, Z: -3216}},
			{29023, script.Loc{X: 54262, Y: 219480, Z: -3216}},
			{29026, script.Loc{X: 53412, Y: 218077, Z: -3216}},
			{29027, script.Loc{X: 55440, Y: 218081, Z: -3216}},
			{29026, script.Loc{X: 55202, Y: 217940, Z: -3216}},
			{29027, script.Loc{X: 55225, Y: 218236, Z: -3216}},
			{29027, script.Loc{X: 54973, Y: 218075, Z: -3216}},
		}
	case 7:
		return []private{
			{29027, script.Loc{X: 54228, Y: 217504, Z: -3216}},
			{29024, script.Loc{X: 54181, Y: 217168, Z: -3216}},
			{29023, script.Loc{X: 54714, Y: 217123, Z: -3168}},
			{29023, script.Loc{X: 55298, Y: 217127, Z: -3073}},
			{29023, script.Loc{X: 55787, Y: 217130, Z: -2993}},
			{29026, script.Loc{X: 56284, Y: 217216, Z: -2944}},
			{29024, script.Loc{X: 56963, Y: 218080, Z: -2944}},
			{29027, script.Loc{X: 56267, Y: 218826, Z: -2944}},
			{29023, script.Loc{X: 56294, Y: 219482, Z: -2944}},
			{29026, script.Loc{X: 56094, Y: 219113, Z: -2944}},
			{29023, script.Loc{X: 56364, Y: 218967, Z: -2944}},
			{29024, script.Loc{X: 56276, Y: 220783, Z: -2944}},
			{29024, script.Loc{X: 57173, Y: 220234, Z: -2944}},
			{29027, script.Loc{X: 54885, Y: 220144, Z: -2944}},
			{29027, script.Loc{X: 55264, Y: 219860, Z: -2944}},
			{29026, script.Loc{X: 55399, Y: 220263, Z: -2944}},
			{29027, script.Loc{X: 55679, Y: 220129, Z: -2944}},
			{29026, script.Loc{X: 54236, Y: 220948, Z: -2944}},
			{29026, script.Loc{X: 54464, Y: 219095, Z: -2944}},
			{29024, script.Loc{X: 54226, Y: 218797, Z: -2944}},
			{29024, script.Loc{X: 54394, Y: 219067, Z: -2944}},
			{29027, script.Loc{X: 54139, Y: 219253, Z: -2944}},
			{29023, script.Loc{X: 54262, Y: 219480, Z: -2944}},
			{29026, script.Loc{X: 53412, Y: 218077, Z: -2944}},
			{29026, script.Loc{X: 54280, Y: 217200, Z: -2944}},
			{29027, script.Loc{X: 55440, Y: 218081, Z: -2944}},
			{29026, script.Loc{X: 55202, Y: 217940, Z: -2944}},
			{29027, script.Loc{X: 55225, Y: 218236, Z: -2944}},
			{29027, script.Loc{X: 54973, Y: 218075, Z: -2944}},
		}
	}
	return nil
}
