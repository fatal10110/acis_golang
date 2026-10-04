package fence

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var (
	// errOutOfWorld rejects a fence reaching past the world bounds.
	errOutOfWorld = errors.New("fence: coordinates are outside of world")
	// errUnknownSize rejects a fence wider or longer than the size table.
	errUnknownSize = errors.New("fence: unknown dimensions")
)

// Geo is the geodata a fence is placed on and blocks movement through.
type Geo interface {
	Height(worldX, worldY, worldZ int) int16
	AddObject(dynamic.Object)
	RemoveObject(dynamic.Object)
}

// IDs allocates world object ids.
type IDs interface {
	NextID() (int32, error)
}

// Manager places and removes fences and lists the placed ones.
//
// mu serializes placements and removals, so a fence's world presence, its
// geodata and its list entry change as one step, and guards fences.
type Manager struct {
	geo   Geo
	world *world.State
	ids   IDs

	mu     sync.Mutex
	fences []*Fence
}

// NewManager returns a manager placing fences into w on g.
func NewManager(g Geo, w *world.State, ids IDs) *Manager {
	return &Manager{geo: g, world: w, ids: ids}
}

// Fences returns the placed fences in placement order.
func (m *Manager) Fences() []*Fence {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.fences)
}

// Add places a fence of type typ, sizeX by sizeY world units and height
// layers high (one, two or three), centered at (x, y, z) aligned to the
// geodata grid: it is shown to the players around, then blocks movement.
func (m *Manager) Add(x, y, z, typ, sizeX, sizeY, height int) (*Fence, error) {
	if x < geo.WorldXMin || x+sizeX > geo.WorldXMax || y < geo.WorldYMin || y+sizeY > geo.WorldYMax {
		return nil, errOutOfWorld
	}
	fsx, okX := sizeOf(sizeX)
	fsy, okY := sizeOf(sizeY)
	if !okX || !okY {
		return nil, fmt.Errorf("%w: x=%d y=%d", errUnknownSize, sizeX, sizeY)
	}
	x, y = align(x, fsx), align(y, fsy)

	f := &Fence{typ: typ, sizeX: sizeX, sizeY: sizeY}
	var err error
	if f.objectID, err = m.ids.NextID(); err != nil {
		return nil, fmt.Errorf("fence: allocate id: %w", err)
	}
	for range min(height, 3) - 1 {
		id, err := m.ids.NextID()
		if err != nil {
			return nil, fmt.Errorf("fence: allocate layer id: %w", err)
		}
		f.layers = append(f.layers, &Layer{objectID: id, fence: f})
	}
	geoZ := int(m.geo.Height(x, y, z))
	f.shape = dynamic.NewObject(engine.GeoX(x)-fsx.cells/2, engine.GeoY(y)-fsy.cells/2, geoZ, height*layerHeight, dynamic.CalculateGeoObject(outline(typ, fsx.cells, fsy.cells)))

	m.mu.Lock()
	defer m.mu.Unlock()
	m.world.Spawn(f, x, y, z, 0)
	for _, layer := range f.layers {
		m.world.Spawn(layer, x, y, z, 0)
	}
	m.geo.AddObject(f.shape)
	m.fences = append(m.fences, f)
	return f, nil
}

// Remove takes f out of the world and the geodata: its layers vanish
// first, then the fence. It reports false when f was already removed.
func (m *Manager) Remove(f *Fence) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := slices.Index(m.fences, f)
	if i < 0 {
		return false
	}
	for _, layer := range f.layers {
		m.world.Despawn(layer)
	}
	m.world.Despawn(f)
	m.geo.RemoveObject(f.shape)
	m.fences = slices.Delete(m.fences, i, i+1)
	return true
}

// size is a fence's placement along one axis: the offset its position
// keeps below the geodata cell boundary, and how many geodata cells it
// spans.
type size struct {
	offset int
	cells  int
}

// sizes are the placements of fences 100 to 1000 units long, by 100-unit
// step.
var sizes = [...]size{{8, 11}, {0, 18}, {0, 24}, {0, 30}, {0, 36}, {0, 42}, {8, 49}, {8, 55}, {8, 61}, {0, 68}}

// sizeOf returns the placement of a fence n units long: under 199 is the
// 100-unit one, under 299 the 200-unit one, and so on up to under 1099.
func sizeOf(n int) (size, bool) {
	for i, s := range sizes {
		if n < (i+1)*100+99 {
			return s, true
		}
	}
	return size{}, false
}

// align snaps coordinate v to the geodata grid for placement s. The
// reference writes this as v & 0xFFFFFFF0 + offset, which Java's operator
// precedence evaluates as v & (0xFFFFFFF0 + offset): a mask clearing the
// low four bits, or only the low three when the offset is 8.
func align(v int, s size) int {
	return v & (s.offset - 16)
}

// outline marks the geodata cells a fence of type typ, cellsX by cellsY,
// occupies: a three-cell-thick ring one cell in from the edge for type 2,
// only its four corners for any other type.
func outline(typ, cellsX, cellsY int) [][]bool {
	inside := make([][]bool, cellsX)
	for ix := range inside {
		inside[ix] = make([]bool, cellsY)
	}
	for ix := 1; ix < cellsX-1; ix++ {
		for iy := 1; iy < cellsY-1; iy++ {
			edgeX := ix < 3 || ix >= cellsX-3
			edgeY := iy < 3 || iy >= cellsY-3
			if typ == 2 {
				inside[ix][iy] = edgeX || edgeY
			} else {
				inside[ix][iy] = edgeX && edgeY
			}
		}
	}
	return inside
}
