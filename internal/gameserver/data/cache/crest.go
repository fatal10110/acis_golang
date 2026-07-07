package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CrestType identifies a crest file family and its exact byte size.
type CrestType int

const (
	// PledgeCrest is the small clan crest format.
	PledgeCrest CrestType = iota
	// LargePledgeCrest is the large clan crest format.
	LargePledgeCrest
	// AllyCrest is the alliance crest format.
	AllyCrest
)

var crestTypes = []CrestType{PledgeCrest, LargePledgeCrest, AllyCrest}

type crestSpec struct {
	prefix string
	size   int
}

// Crests keeps loaded .dds crest blobs keyed by crest id.
type Crests struct {
	byID map[int][]byte
}

// LoadCrests reads valid crest .dds files from dir into memory.
func LoadCrests(dir string) (*Crests, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("load crests from %q: %w", dir, err)
	}

	c := &Crests{byID: make(map[int][]byte)}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		typ, id, ok, err := parseCrestName(name)
		if err != nil {
			return nil, fmt.Errorf("load crest %q: %w", name, err)
		}
		if !ok {
			continue
		}

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("load crest %q: %w", path, err)
		}
		spec, _ := typ.spec()
		if len(data) != spec.size {
			return nil, fmt.Errorf("load crest %q: got %d bytes, want %d", path, len(data), spec.size)
		}
		c.byID[id] = data
	}

	return c, nil
}

// Get returns a copy of id's crest data when it exists and matches typ.
func (c *Crests) Get(typ CrestType, id int) ([]byte, bool) {
	spec, ok := typ.spec()
	if c == nil || !ok {
		return nil, false
	}
	data, ok := c.byID[id]
	if !ok || len(data) != spec.size {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

// Len returns the number of crest ids currently loaded.
func (c *Crests) Len() int {
	if c == nil {
		return 0
	}
	return len(c.byID)
}

func parseCrestName(name string) (CrestType, int, bool, error) {
	if !strings.HasSuffix(name, ".dds") {
		return 0, 0, false, nil
	}
	for _, typ := range crestTypes {
		spec, _ := typ.spec()
		if !strings.HasPrefix(name, spec.prefix) {
			continue
		}

		rawID := strings.TrimSuffix(strings.TrimPrefix(name, spec.prefix), ".dds")
		id, err := strconv.Atoi(rawID)
		if err != nil {
			return 0, 0, false, fmt.Errorf("parse crest id %q: %w", rawID, err)
		}
		return typ, id, true, nil
	}
	return 0, 0, false, nil
}

func (t CrestType) spec() (crestSpec, bool) {
	switch t {
	case PledgeCrest:
		return crestSpec{prefix: "Crest_", size: 256}, true
	case LargePledgeCrest:
		return crestSpec{prefix: "LargeCrest_", size: 2176}, true
	case AllyCrest:
		return crestSpec{prefix: "AllyCrest_", size: 192}, true
	default:
		return crestSpec{}, false
	}
}
