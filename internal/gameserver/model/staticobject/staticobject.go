// Package staticobject models static-object XML data loaded at boot.
package staticobject

import (
	"fmt"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Template is one static object entry from staticObjects.xml.
type Template struct {
	ID       int
	Location location.Location
	Type     int
	Texture  string
	MapX     int
	MapY     int
}

// Object is one live static object spawned into the world.
type Object struct {
	world.Presence

	objectID int32
	Template *Template
	busy     atomic.Bool
}

// NewObject creates a live static object from a static template.
func NewObject(objectID int32, tmpl *Template) (*Object, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("static object: nil template")
	}
	return &Object{objectID: objectID, Template: tmpl}, nil
}

// ObjectID returns the world object id assigned to this static object.
func (o *Object) ObjectID() int32 { return o.objectID }

// StaticObjectID returns the static object id from staticObjects.xml.
func (o *Object) StaticObjectID() int { return o.Template.ID }

// Type returns the static object type from staticObjects.xml.
func (o *Object) Type() int { return o.Template.Type }

// Busy reports whether this static object is currently occupied.
func (o *Object) Busy() bool {
	return o.busy.Load()
}

// SetBusy updates whether this static object is occupied and reports whether it changed.
func (o *Object) SetBusy(busy bool) bool {
	return o.busy.CompareAndSwap(!busy, busy)
}

// NewTemplate builds a static object template from XML attributes.
func NewTemplate(set *commons.StatSet) (*Template, error) {
	idf := commons.NewFields(set, "static object")
	id := idf.Int("id")
	if err := idf.Err(); err != nil {
		return nil, err
	}
	wrap := func(err error) error { return fmt.Errorf("static object %d: %w", id, err) }

	loc, err := location.NewLocation(set)
	if err != nil {
		return nil, wrap(err)
	}
	f := commons.NewFields(set, fmt.Sprintf("static object %d", id))
	t := &Template{
		ID:       id,
		Location: loc,
		Type:     f.Int("type"),
		Texture:  f.String("texture"),
		MapX:     f.Int("mapX"),
		MapY:     f.Int("mapY"),
	}
	if err := f.Err(); err != nil {
		return nil, err
	}
	return t, nil
}

// Table stores static object templates keyed by static object id.
type Table struct {
	objects map[int]*Template
	order   []*Template
}

// NewTable builds a static object table and rejects duplicate ids.
func NewTable(templates []*Template) (*Table, error) {
	t := &Table{
		objects: make(map[int]*Template, len(templates)),
		order:   make([]*Template, 0, len(templates)),
	}
	for _, tmpl := range templates {
		if tmpl == nil {
			return nil, fmt.Errorf("static object: nil template")
		}
		if _, exists := t.objects[tmpl.ID]; exists {
			return nil, fmt.Errorf("static object: duplicate template id %d", tmpl.ID)
		}
		t.objects[tmpl.ID] = tmpl
		t.order = append(t.order, tmpl)
	}
	return t, nil
}

// Len returns the number of loaded static object templates.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.order)
}

// Get returns the template for id.
func (t *Table) Get(id int) (*Template, bool) {
	if t == nil {
		return nil, false
	}
	tmpl, ok := t.objects[id]
	return tmpl, ok
}

// All returns the loaded templates in file order.
func (t *Table) All() []*Template {
	if t == nil {
		return nil
	}
	return append([]*Template(nil), t.order...)
}
