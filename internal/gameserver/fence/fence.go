// Package fence models the fences a game master places with //spawnfence:
// world objects shown to players with ExColosseumFenceInfo whose outline
// blocks movement through a dynamic geodata object.
package fence

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// layerHeight is how high one height layer of a fence blocks.
const layerHeight = 24

// Fence is one placed fence.
type Fence struct {
	world.Presence

	objectID int32
	typ      int
	sizeX    int
	sizeY    int
	// layers are the fence's extra height layers, spawned and removed with
	// it.
	layers []*Layer
	shape  dynamic.Object
}

// ObjectID returns the fence's world object id.
func (f *Fence) ObjectID() int32 { return f.objectID }

// Kind reports KindFence.
func (f *Fence) Kind() actor.Kind { return actor.KindFence }

// Type returns the fence type: 1 shows only its corner columns, 2 the
// columns joined by fences.
func (f *Fence) Type() int { return f.typ }

// Size returns the fence's width and length in world units.
func (f *Fence) Size() (sizeX, sizeY int) { return f.sizeX, f.sizeY }

// Shown returns the fence itself, whose data its info packet carries.
func (f *Fence) Shown() *Fence { return f }

// Layer is one extra height layer of a fence two or three layers high: a
// world object of its own, shown as its fence under its own object id.
type Layer struct {
	world.Presence

	objectID int32
	fence    *Fence
}

// ObjectID returns the layer's world object id.
func (l *Layer) ObjectID() int32 { return l.objectID }

// Kind reports KindFence.
func (l *Layer) Kind() actor.Kind { return actor.KindFence }

// Shown returns the fence this layer belongs to, whose data its info
// packet carries.
func (l *Layer) Shown() *Fence { return l.fence }
