package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

type xmlDocument[T any] struct {
	Path string
	Data T
}

func loadXMLDocuments[T any](dir, kind string) ([]xmlDocument[T], error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("xml: list %s files in %s: %w", kind, dir, err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("xml: no %s files found in %s", kind, dir)
	}
	sort.Strings(paths)

	docs := make([]xmlDocument[T], 0, len(paths))
	for _, path := range paths {
		var doc T
		if err := readXML(path, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", kind, err)
		}
		docs = append(docs, xmlDocument[T]{Path: path, Data: doc})
	}
	return docs, nil
}

// buildAll parses each element in els into a T via ctor, wrapping any
// constructor error with path. It is the shared shape for a flat XML list:
// element attributes fold into a StatSet, then the domain constructor
// validates and builds the model value.
func buildAll[T any](path string, els []attrsElement, ctor func(*commons.StatSet) (T, error)) ([]T, error) {
	out := make([]T, 0, len(els))
	for _, el := range els {
		v, err := ctor(commons.StatSetFromXMLAttrs(el.Attrs))
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// attrLocation reads the required x/y/z attributes of set as a world
// location. Coordinates ride along on elements that describe something else
// (a spawn, a door, a teleport destination), so the loader parses them here
// rather than pushing an attribute bag into the location package.
func attrLocation(set *commons.StatSet) (location.Location, error) {
	f := commons.NewFields(set, "location")
	loc := location.Location{X: f.Int("x"), Y: f.Int("y"), Z: f.Int("z")}
	return loc, f.Err()
}

// attrPoint reads the required x/y attributes of set as a 2D world point.
func attrPoint(set *commons.StatSet) (location.Point, error) {
	f := commons.NewFields(set, "point")
	p := location.Point{X: f.Int("x"), Y: f.Int("y")}
	return p, f.Err()
}

func readXML(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("xml: read %s: %w", path, err)
	}
	if err := xml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("xml: parse %s: %w", path, err)
	}
	return nil
}
