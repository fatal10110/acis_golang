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
// directory new crests are saved to. mu guards byKey, touched and partial:
// crest uploads write them while every client's crest requests read them.
// reloadMu lets one reload at a time read the directory.
type Crests struct {
	dir      string
	reloadMu sync.Mutex
	mu       sync.RWMutex
	byKey    map[crestKey][]byte
	// touched is non-nil while a reload reads the directory; it holds every
	// key Save and Remove changed meanwhile, whose in-memory state outranks
	// what the read found.
	touched map[crestKey]struct{}
	// partial marks a load that may have missed crests the directory holds.
	partial bool
}

// NewCrestsIn returns an empty crest cache saving new crests to dir; with
// an empty dir it saves no crest.
func NewCrestsIn(dir string) *Crests {
	return &Crests{dir: dir, byKey: make(map[crestKey][]byte)}
}

// LoadCrests reads valid crest .dds files from dir into memory; new crests
// are saved there too. A crest file whose size does not match its family is
// deleted and skipped, and its name is returned in deleted.
//
// A file the load cannot handle (an unreadable file, an id that is not a
// number, an undeletable invalid file) is skipped: every other crest still
// loads, and the returned error names each skipped file. When a skipped file
// might hold a crest a clan points at (an unreadable file with a valid name),
// or dir does not exist, the cache is marked partial (see [Crests.Partial]).
// A missing dir gives an empty partial cache with the error; a nil cache with
// an error means dir exists but could not be read.
func LoadCrests(dir string) (c *Crests, deleted []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		err = fmt.Errorf("load crests from %q: %w", dir, err)
		if errors.Is(err, os.ErrNotExist) {
			c = NewCrestsIn(dir)
			c.partial = true
			return c, nil, err
		}
		return nil, nil, err
	}

	c = NewCrestsIn(dir)
	var skipped []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		typ, ok := crestTypeOf(name)
		if !ok {
			continue
		}
		spec, _ := typ.spec()

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if _, idErr := parseCrestID(name, spec); idErr == nil {
				c.partial = true
			}
			skipped = append(skipped, fmt.Errorf("crest file %s: %w", name, err))
			continue
		}
		// The size is checked before the id, so a wrong-size file is deleted
		// whatever its name.
		if len(data) != spec.size {
			if err := os.Remove(path); err != nil {
				skipped = append(skipped, fmt.Errorf("delete invalid crest %s: %w", name, err))
				continue
			}
			deleted = append(deleted, name)
			continue
		}
		id, err := parseCrestID(name, spec)
		if err != nil {
			skipped = append(skipped, fmt.Errorf("crest file %s: %w", name, err))
			continue
		}
		c.byKey[crestKey{typ, id}] = data
	}
	if len(skipped) > 0 {
		err = fmt.Errorf("load crests from %q: %w", dir, errors.Join(skipped...))
	}
	return c, deleted, err
}

// Partial reports whether the load behind c may have missed crests its
// directory holds (an unreadable crest file, a missing directory). A partial
// cache cannot tell which crest ids are gone.
func (c *Crests) Partial() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.partial
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
// exactly the family's size or the file cannot be written. The file is
// written before c's lock is taken, so a slow disk holds up no reader; the
// crest is in memory once it is on disk.
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
	if err := os.WriteFile(filepath.Join(c.dir, name), data, 0o644); err != nil {
		return fmt.Errorf("save crest %s: %w", name, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := crestKey{typ, id}
	c.byKey[key] = data
	c.touchLocked(key)
	return nil
}

// Remove deletes the typ crest id from disk and memory; nothing happens
// when it does not exist. The file is deleted before c's lock is taken, so
// a slow disk holds up no reader, and a reload meanwhile cannot read it back.
// A file that cannot be deleted keeps the crest in memory too.
func (c *Crests) Remove(typ CrestType, id int) error {
	spec, ok := typ.spec()
	if c == nil || !ok {
		return nil
	}
	key := crestKey{typ, id}
	c.mu.RLock()
	_, ok = c.byKey[key]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	if c.dir != "" {
		name := spec.prefix + strconv.Itoa(id) + ".dds"
		if err := os.Remove(filepath.Join(c.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete crest %s: %w", name, err)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byKey, key)
	c.touchLocked(key)
	return nil
}

// Reload swaps c's crests for the ones load reads from c's directory, as a
// crest reload does; c keeps its directory. A crest saved or removed while
// load runs keeps that change through the swap. When load fails, c is left
// as it was.
func (c *Crests) Reload(load func(dir string) (*Crests, error)) error {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	c.mu.Lock()
	c.touched = make(map[crestKey]struct{})
	c.mu.Unlock()

	// fresh is not shared until it is swapped in, so it is read unlocked.
	fresh, err := load(c.dir)

	c.mu.Lock()
	defer c.mu.Unlock()
	touched := c.touched
	c.touched = nil
	if err != nil {
		return err
	}
	for key := range touched {
		if data, ok := c.byKey[key]; ok {
			fresh.byKey[key] = data
		} else {
			delete(fresh.byKey, key)
		}
	}
	c.byKey = fresh.byKey
	c.partial = fresh.partial
	return nil
}

// touchLocked records key as changed during a running reload; c.mu is held.
func (c *Crests) touchLocked(key crestKey) {
	if c.touched != nil {
		c.touched[key] = struct{}{}
	}
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

// crestTypeOf is the crest family a .dds file name's prefix names.
func crestTypeOf(name string) (CrestType, bool) {
	if !strings.HasSuffix(name, ".dds") {
		return 0, false
	}
	for _, typ := range crestTypes {
		if spec, _ := typ.spec(); strings.HasPrefix(name, spec.prefix) {
			return typ, true
		}
	}
	return 0, false
}

// parseCrestID is the crest id a spec file name carries between its prefix
// and its .dds suffix.
func parseCrestID(name string, spec crestSpec) (int, error) {
	rawID := strings.TrimSuffix(strings.TrimPrefix(name, spec.prefix), ".dds")
	id, err := strconv.ParseInt(rawID, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse crest id %q: %w", rawID, err)
	}
	return int(id), nil
}

// Fits reports whether data is an image of a typ crest's size, the only
// size Save stores.
func (t CrestType) Fits(data []byte) bool {
	spec, ok := t.spec()
	return ok && len(data) == spec.size
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
