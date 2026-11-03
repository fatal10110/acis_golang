package serverpackets

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// oraclePrimitiveCases are the debug drawing and the zone outlines drawn
// into it (testdata/oracle/ProbePrimitive.java). Each case is one frame
// a drawing sends: name/n is its n-th frame.
func oraclePrimitiveCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	sent := func(name string, frames int, build func() []wire.Frame) {
		for i := range frames {
			m[name+"/"+strconv.Itoa(i)] = func() wire.Frame {
				all := build()
				for j, f := range all {
					if j != i {
						f.Release()
					}
				}
				return all[i]
			}
		}
	}
	anchored := func() *Drawing { return NewDrawing("ZONE", -71338, 258271, -16000) }
	outline := func(name, info string, form func() (zone.Form, error)) {
		sent("ExServerPrimitive/"+name, 1, func() []wire.Frame {
			f, err := form()
			if err != nil {
				panic(err)
			}
			d := anchored()
			d.AddZoneOutline(info, zone.Outline(f))
			return d.Frames()
		})
	}

	sent("ExServerPrimitive/empty", 1, func() []wire.Frame { return anchored().Frames() })
	sent("ExServerPrimitive/point-line", 1, func() []wire.Frame {
		d := NewDrawing("dbg", 1, -2, 3)
		d.AddLine(PrimitiveLine{PrimitivePoint: PrimitivePoint{Color: PrimitiveYellow, X: -10, Y: 20, Z: -30}, X2: 40, Y2: -50, Z2: 60})
		d.AddPoint(PrimitivePoint{Name: "Pté", Color: 0x123456, NameColored: true, X: 7, Y: -8, Z: 9})
		return d.Frames()
	})
	outline("cuboid", "PeaceZone[2]", func() (zone.Form, error) { return zone.NewCuboid(200, -100, 400, -300, -3000, -3500) })
	outline("cylinder", "TownZone[11]", func() (zone.Form, error) { return zone.NewCylinder(-71338, 258271, -3700, -3100, 300) })
	outline("cylinder-small", "ArenaZone[3]", func() (zone.Form, error) { return zone.NewCylinder(10, 20, 0, 100, 7) })
	outline("npoly", "SwampZone[40]", func() (zone.Form, error) {
		return zone.NewPolygon([]location.Point{{X: 0, Y: 0}, {X: 1000, Y: 0}, {X: 1500, Y: 900}, {X: -200, Y: 700}}, 100, -100)
	})

	// A wide outline continues in a second frame; emptied, both go out
	// blank and the second is dropped.
	wide := func() *Drawing {
		f, err := zone.NewCylinder(1000, -2000, -500, 500, 1000)
		if err != nil {
			panic(err)
		}
		d := anchored()
		d.AddZoneOutline("PeaceZone[2]", zone.Outline(f))
		return d
	}
	sent("ExServerPrimitive/chain", 2, func() []wire.Frame { return wide().Frames() })
	sent("ExServerPrimitive/chain-reset", 2, func() []wire.Frame {
		d := wide()
		releaseFrames(d.Frames())
		d.Reset()
		return d.Frames()
	})
	sent("ExServerPrimitive/chain-reset-again", 1, func() []wire.Frame {
		d := wide()
		releaseFrames(d.Frames())
		d.Reset()
		releaseFrames(d.Frames())
		return d.Frames()
	})
	return m
}

func releaseFrames(frames []wire.Frame) {
	for _, f := range frames {
		f.Release()
	}
}

// TestDrawingSplitsAtCapacity pins where a drawing continues in a new
// frame: a stroke is added to the first frame below 16000 bytes of
// strokes, so the stroke crossing that size stays in its frame.
func TestDrawingSplitsAtCapacity(t *testing.T) {
	d := NewDrawing("D", 0, 0, 0)
	for range 516 { // 516 * 31 = 15996
		d.AddPoint(PrimitivePoint{})
	}
	d.AddLine(PrimitiveLine{}) // 16039: crosses, stays
	d.AddPoint(PrimitivePoint{Name: "x"})
	frames := d.Frames()
	if len(frames) != 2 {
		releaseFrames(frames)
		t.Fatalf("frames = %d, want 2", len(frames))
	}
	// The second frame: a 33-byte header naming it D1, then one point
	// named x.
	frames[0].Release()
	if got := framePayload(t, frames[1]); len(got) != 33+33 || got[3] != 'D' || got[5] != '1' {
		t.Fatalf("second frame = %x, want D1 holding one point", got)
	}
}
