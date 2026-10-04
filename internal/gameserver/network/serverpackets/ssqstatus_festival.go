package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

// SSQFestivalBest is one cabal's best result in a festival this cycle: the
// score and the names of the party that set it.
type SSQFestivalBest struct {
	Score   int32
	Members []string
}

// SSQFestival is one festival's line on the record's second page: what the
// festival is worth and each cabal's best result in it.
type SSQFestival struct {
	MaxScore int32
	Dusk     SSQFestivalBest
	Dawn     SSQFestivalBest
}

// FrameSSQStatusFestival builds SSQStatus page 2, [h c (c d (d c S*)(d c
// S*))*]: a constant 1, the festival count, then each festival numbered
// from 1 with its worth and the dusk then dawn best result.
func FrameSSQStatusFestival(period byte, festivals []SSQFestival) wire.Frame {
	w := newSSQStatusWriter(2, period)
	w.WriteUint16(1)
	w.WriteUint8(byte(len(festivals)))
	for i, f := range festivals {
		w.WriteUint8(byte(i + 1))
		w.WriteInt32(f.MaxScore)
		for _, best := range [...]SSQFestivalBest{f.Dusk, f.Dawn} {
			w.WriteInt32(best.Score)
			w.WriteUint8(byte(len(best.Members)))
			for _, name := range best.Members {
				w.WriteString(name)
			}
		}
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
