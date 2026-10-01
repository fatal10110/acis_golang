package cache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

// crestKey names one crest: its family and its id. Pledge, large pledge
// and alliance crests are separate families, so one id may name a crest in
// each.
type crestKey struct {
	typ CrestType
	id  int
}

// Crests keeps the .dds crest blobs keyed by crest family and id, and the
// directory new crests are saved to. mu guards byKey: crest uploads write it
// while every client's crest requests read it.
type Crests struct {
	dir   string
	mu    sync.RWMutex
	byKey map[crestKey][]byte
}

// NewCrests returns an empty crest cache with no directory: it saves no
// crest.
func NewCrests() *Crests { return NewCrestsIn("") }

// NewCrestsIn returns an empty crest cache saving new crests to dir.
func NewCrestsIn(dir string) *Crests {
	return &Crests{dir: dir, byKey: make(map[crestKey][]byte)}
}

// LoadCrests reads valid crest .dds files from dir into memory; new crests
// are saved there too. A crest file whose size does not match its family is
// deleted and skipped, and its name is returned in deleted.
//
// The load stops at the first file it cannot handle (an id that is not a
// number, an unreadable file, an undeletable invalid file), as the
// reference does. It then returns the cache holding the crests read before
// that file together with a non-nil error saying why it stopped, so the
// caller can keep the partial cache and report the failure. A nil cache
// with an error means the directory itself could not be read.
func LoadCrests(dir string) (c *Crests, deleted []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("load crests from %q: %w", dir, err)
	}

	c = NewCrestsIn(dir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		typ, id, ok, err := parseCrestName(name)
		if err != nil {
			return c, deleted, fmt.Errorf("load crests from %q: crest file %s: %w", dir, name, err)
		}
		if !ok {
			continue
		}

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return c, deleted, fmt.Errorf("load crests from %q: %w", dir, err)
		}
		spec, _ := typ.spec()
		if len(data) != spec.size {
			if err := os.Remove(path); err != nil {
				return c, deleted, fmt.Errorf("load crests from %q: delete invalid crest: %w", dir, err)
			}
			deleted = append(deleted, name)
			continue
		}
		c.byKey[crestKey{typ, id}] = data
	}

	return c, deleted, nil
}

// Get returns a copy of the typ crest id's data when it exists.
func (c *Crests) Get(typ CrestType, id int) ([]byte, bool) {
	spec, ok := typ.spec()
	if c == nil || !ok {
		return nil, false
	}
	c.mu.RLock()
	data, ok := c.byKey[crestKey{typ, id}]
	c.mu.RUnlock()
	if !ok || len(data) != spec.size {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

// Has reports whether the typ crest id exists.
func (c *Crests) Has(typ CrestType, id int) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.byKey[crestKey{typ, id}]
	return ok
}

// Save stores data as the typ crest id, on disk and in memory, replacing
// any crest of that family and id. Nothing is saved when data is not
// exactly the family's size or the file cannot be written.
func (c *Crests) Save(typ CrestType, id int, data []byte) error {
	spec, ok := typ.spec()
	if !ok {
		return fmt.Errorf("save crest %d: unknown crest type %d", id, typ)
	}
	name := spec.prefix + strconv.Itoa(id) + ".dds"
	if len(data) != spec.size {
		return fmt.Errorf("save crest %s: %d bytes, want %d", name, len(data), spec.size)
	}
	if c == nil || c.dir == "" {
		return fmt.Errorf("save crest %s: no crest directory", name)
	}
	data = append([]byte(nil), data...)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.WriteFile(filepath.Join(c.dir, name), data, 0o644); err != nil {
		return fmt.Errorf("save crest %s: %w", name, err)
	}
	c.byKey[crestKey{typ, id}] = data
	return nil
}

// Remove deletes the typ crest id from memory and disk; nothing happens
// when it does not exist.
func (c *Crests) Remove(typ CrestType, id int) error {
	spec, ok := typ.spec()
	if c == nil || !ok {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := crestKey{typ, id}
	if _, ok := c.byKey[key]; !ok {
		return nil
	}
	delete(c.byKey, key)
	if c.dir == "" {
		return nil
	}
	name := spec.prefix + strconv.Itoa(id) + ".dds"
	if err := os.Remove(filepath.Join(c.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete crest %s: %w", name, err)
	}
	return nil
}

// Len returns the number of crests currently loaded.
func (c *Crests) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.byKey)
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
