// Package staticobject models static-object XML data loaded at boot.
package staticobject

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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

// NewTemplate builds a static object template from XML attributes.
func NewTemplate(set *commons.StatSet) (*Template, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return nil, fmt.Errorf("static object: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("static object %d: %w", id, err) }

	loc, err := location.NewLocation(set)
	if err != nil {
		return nil, wrap(err)
	}
	kind, err := set.GetInt("type")
	if err != nil {
		return nil, wrap(err)
	}
	texture, err := set.GetString("texture")
	if err != nil {
		return nil, wrap(err)
	}
	mapX, err := set.GetInt("mapX")
	if err != nil {
		return nil, wrap(err)
	}
	mapY, err := set.GetInt("mapY")
	if err != nil {
		return nil, wrap(err)
	}
	return &Template{
		ID:       id,
		Location: loc,
		Type:     kind,
		Texture:  texture,
		MapX:     mapX,
		MapY:     mapY,
	}, nil
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
