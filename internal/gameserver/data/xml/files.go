package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

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

// coord is a world-coordinate attribute. It parses itself with strconv.Atoi
// so the accepted input set matches the attribute bag it replaces exactly:
// the decoder's own int conversion accepts an empty value as 0 and trims
// surrounding space, both of which the bag rejected.
type coord int

func (c *coord) UnmarshalXMLAttr(attr xml.Attr) error {
	n, err := strconv.Atoi(attr.Value)
	if err != nil {
		return fmt.Errorf("%s: %w", attr.Name.Local, err)
	}
	*c = coord(n)
	return nil
}

// pointElement is an element whose x/y attributes are a 2D world point, and
// locationElement one whose x/y/z attributes are a world location. Both are
// embedded into the element types that carry coordinates alongside their own
// attributes, so the decoder converts them like any other tagged field.
//
// The coordinates are pointers because they are required and zero is a legal
// coordinate: a non-pointer cannot tell an absent attribute from x="0", and a
// missing coordinate silently reading as the world origin is exactly the
// data-file corruption the loaders are meant to reject. A malformed value is
// rejected by coord itself, so nil here means only "attribute absent".
type pointElement struct {
	X *coord `xml:"x,attr"`
	Y *coord `xml:"y,attr"`
}

func (e pointElement) point() (location.Point, error) {
	if e.X == nil || e.Y == nil {
		return location.Point{}, fmt.Errorf("x and y are required")
	}
	return location.Point{X: int(*e.X), Y: int(*e.Y)}, nil
}

type locationElement struct {
	X *coord `xml:"x,attr"`
	Y *coord `xml:"y,attr"`
	Z *coord `xml:"z,attr"`
}

func (e locationElement) loc() (location.Location, error) {
	if e.X == nil || e.Y == nil || e.Z == nil {
		return location.Location{}, fmt.Errorf("x, y and z are required")
	}
	return location.Location{X: int(*e.X), Y: int(*e.Y), Z: int(*e.Z)}, nil
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
