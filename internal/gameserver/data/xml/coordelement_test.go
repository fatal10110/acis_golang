package xml

import (
	"encoding/xml"
	"os"
	"strconv"
	"testing"
)

// TestCoordinateElementsDecodeEveryShippedNode guards the typed coordinate
// elements against a silent decoding regression. pointElement and
// locationElement replaced a generic "capture every attribute" element, so a
// mistyped struct tag would not fail the build or the model constructors —
// the list would simply decode empty and the loader would build a zone with
// no nodes or a castle with no spawns. Each case decodes a shipped datapack
// file through the production element type and through a raw attribute
// capture of the same path, then requires the coordinates to agree.
func TestCoordinateElementsDecodeEveryShippedNode(t *testing.T) {
	type rawNode struct {
		Attrs []xml.Attr `xml:",any,attr"`
	}

	cases := []struct {
		name    string
		rel     string
		want    []string
		decode  func(t *testing.T, data []byte) [][3]string
		rawPath func(t *testing.T, data []byte) [][3]string
	}{
		{
			name: "zone nodes and spawns",
			rel:  "data/xml/zones/OlympiadStadiumZone.xml",
			decode: func(t *testing.T, data []byte) [][3]string {
				var doc zoneFile
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, z := range doc.Zones {
					for _, n := range z.Nodes {
						p, err := n.point()
						if err != nil {
							t.Fatalf("node.point(): %v", err)
						}
						out = append(out, [3]string{itoa(p.X), itoa(p.Y), ""})
					}
					for _, s := range z.Spawns {
						l, err := s.loc()
						if err != nil {
							t.Fatalf("spawn.loc(): %v", err)
						}
						out = append(out, [3]string{itoa(l.X), itoa(l.Y), itoa(l.Z)})
					}
				}
				return out
			},
			rawPath: func(t *testing.T, data []byte) [][3]string {
				var doc struct {
					Zones []struct {
						Nodes  []rawNode `xml:"node"`
						Spawns []rawNode `xml:"spawn"`
					} `xml:"zone"`
				}
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, z := range doc.Zones {
					for _, n := range z.Nodes {
						out = append(out, [3]string{attr(n.Attrs, "x"), attr(n.Attrs, "y"), ""})
					}
					for _, s := range z.Spawns {
						out = append(out, [3]string{attr(s.Attrs, "x"), attr(s.Attrs, "y"), attr(s.Attrs, "z")})
					}
				}
				return out
			},
		},
		{
			name: "castle zone nodes and spawns",
			rel:  "data/xml/castles.xml",
			decode: func(t *testing.T, data []byte) [][3]string {
				var doc castleFile
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, c := range doc.Castles {
					for _, z := range c.Zones {
						for _, n := range z.Nodes {
							p, err := n.point()
							if err != nil {
								t.Fatalf("node.point(): %v", err)
							}
							out = append(out, [3]string{itoa(p.X), itoa(p.Y), ""})
						}
					}
					for _, s := range c.Spawns {
						l, err := s.loc()
						if err != nil {
							t.Fatalf("spawn.loc(): %v", err)
						}
						out = append(out, [3]string{itoa(l.X), itoa(l.Y), itoa(l.Z)})
					}
				}
				return out
			},
			rawPath: func(t *testing.T, data []byte) [][3]string {
				var doc struct {
					Castles []struct {
						Zones []struct {
							Nodes []rawNode `xml:"node"`
						} `xml:"zones>zone"`
						Spawns []rawNode `xml:"spawns>spawn"`
					} `xml:"castle"`
				}
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, c := range doc.Castles {
					for _, z := range c.Zones {
						for _, n := range z.Nodes {
							out = append(out, [3]string{attr(n.Attrs, "x"), attr(n.Attrs, "y"), ""})
						}
					}
					for _, s := range c.Spawns {
						out = append(out, [3]string{attr(s.Attrs, "x"), attr(s.Attrs, "y"), attr(s.Attrs, "z")})
					}
				}
				return out
			},
		},
		{
			name: "door coordinates",
			rel:  "data/xml/doors.xml",
			decode: func(t *testing.T, data []byte) [][3]string {
				var doc doorFile
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, d := range doc.Doors {
					for _, c := range d.Coordinates {
						p, err := c.point()
						if err != nil {
							t.Fatalf("coord.point(): %v", err)
						}
						out = append(out, [3]string{itoa(p.X), itoa(p.Y), ""})
					}
				}
				return out
			},
			rawPath: func(t *testing.T, data []byte) [][3]string {
				var doc struct {
					Doors []struct {
						Coordinates []rawNode `xml:"coordinates>loc"`
					} `xml:"door"`
				}
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, d := range doc.Doors {
					for _, c := range d.Coordinates {
						out = append(out, [3]string{attr(c.Attrs, "x"), attr(c.Attrs, "y"), ""})
					}
				}
				return out
			},
		},
		{
			name: "instant teleport locations",
			rel:  "data/xml/instantTeleports.xml",
			decode: func(t *testing.T, data []byte) [][3]string {
				var doc instantTeleportFile
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, l := range doc.Lists {
					for _, loc := range l.Locs {
						v, err := loc.loc()
						if err != nil {
							t.Fatalf("loc.loc(): %v", err)
						}
						out = append(out, [3]string{itoa(v.X), itoa(v.Y), itoa(v.Z)})
					}
				}
				return out
			},
			rawPath: func(t *testing.T, data []byte) [][3]string {
				var doc struct {
					Lists []struct {
						Locs []rawNode `xml:"loc"`
					} `xml:"telPosList"`
				}
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, l := range doc.Lists {
					for _, loc := range l.Locs {
						out = append(out, [3]string{attr(loc.Attrs, "x"), attr(loc.Attrs, "y"), attr(loc.Attrs, "z")})
					}
				}
				return out
			},
		},
		{
			name: "manor area nodes",
			rel:  "data/xml/manorAreas.xml",
			decode: func(t *testing.T, data []byte) [][3]string {
				var doc manorAreaFile
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, a := range doc.Areas {
					for _, n := range a.Nodes {
						p, err := n.point()
						if err != nil {
							t.Fatalf("node.point(): %v", err)
						}
						out = append(out, [3]string{itoa(p.X), itoa(p.Y), ""})
					}
				}
				return out
			},
			rawPath: func(t *testing.T, data []byte) [][3]string {
				var doc struct {
					Areas []struct {
						Nodes []rawNode `xml:"node"`
					} `xml:"area"`
				}
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, a := range doc.Areas {
					for _, n := range a.Nodes {
						out = append(out, [3]string{attr(n.Attrs, "x"), attr(n.Attrs, "y"), ""})
					}
				}
				return out
			},
		},
		{
			name: "restart area nodes",
			rel:  "data/xml/restartPointAreas.xml",
			decode: func(t *testing.T, data []byte) [][3]string {
				var doc restartFile
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, a := range doc.Areas {
					for _, n := range a.Nodes {
						p, err := n.point()
						if err != nil {
							t.Fatalf("node.point(): %v", err)
						}
						out = append(out, [3]string{itoa(p.X), itoa(p.Y), ""})
					}
				}
				return out
			},
			rawPath: func(t *testing.T, data []byte) [][3]string {
				var doc struct {
					Areas []struct {
						Nodes []rawNode `xml:"node"`
					} `xml:"area"`
				}
				mustUnmarshal(t, data, &doc)
				var out [][3]string
				for _, a := range doc.Areas {
					for _, n := range a.Nodes {
						out = append(out, [3]string{attr(n.Attrs, "x"), attr(n.Attrs, "y"), ""})
					}
				}
				return out
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(datapackPath(t, tc.rel))
			if err != nil {
				t.Fatalf("read %s: %v", tc.rel, err)
			}
			got := tc.decode(t, data)
			want := tc.rawPath(t, data)
			if len(want) == 0 {
				t.Fatalf("%s decoded no coordinates through the raw path; the test is not exercising anything", tc.rel)
			}
			if len(got) != len(want) {
				t.Fatalf("%s decoded %d coordinates through the typed elements, %d through raw attributes", tc.rel, len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%s coordinate %d = %v through the typed elements, %v through raw attributes", tc.rel, i, got[i], want[i])
				}
			}
		})
	}
}

func mustUnmarshal(t *testing.T, data []byte, dst any) {
	t.Helper()
	if err := xml.Unmarshal(data, dst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

// attr returns the last value of the named attribute, normalized through
// strconv so it compares equal to the decoder's own conversion. Taking the
// last value matches how the decoder folds a repeated attribute.
func attr(attrs []xml.Attr, name string) string {
	out := ""
	for _, a := range attrs {
		if a.Name.Local == name {
			out = a.Value
		}
	}
	if out == "" {
		return ""
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return out
	}
	return strconv.Itoa(n)
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

// TestCoordinateAttrRejectsMalformedValues pins the accepted input set of a
// coordinate attribute to the one the attribute bag enforced before the
// typed elements replaced it: absent is distinguishable, a value that is not
// a bare base-10 integer is rejected. The decoder's own int conversion does
// neither — it stores an empty value as 0 and trims surrounding space — so
// without coord's UnmarshalXMLAttr an empty coordinate would silently place
// the point at the world origin.
func TestCoordinateAttrRejectsMalformedValues(t *testing.T) {
	for _, tc := range []struct {
		name    string
		doc     string
		wantErr bool
		wantNil bool
		wantX   int
	}{
		{name: "plain value", doc: `<n><node x="12" y="5"/></n>`, wantX: 12},
		{name: "zero is a legal coordinate", doc: `<n><node x="0" y="5"/></n>`, wantX: 0},
		{name: "negative value", doc: `<n><node x="-12" y="5"/></n>`, wantX: -12},
		{name: "absent stays distinguishable", doc: `<n><node y="5"/></n>`, wantNil: true},
		{name: "empty value is rejected", doc: `<n><node x="" y="5"/></n>`, wantErr: true},
		{name: "padded value is rejected", doc: `<n><node x=" 12 " y="5"/></n>`, wantErr: true},
		{name: "non-numeric value is rejected", doc: `<n><node x="abc" y="5"/></n>`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var doc struct {
				Nodes []pointElement `xml:"node"`
			}
			err := xml.Unmarshal([]byte(tc.doc), &doc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Unmarshal(%s) error = nil, want a rejection", tc.doc)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", tc.doc, err)
			}
			got := doc.Nodes[0]
			if tc.wantNil {
				if got.X != nil {
					t.Fatalf("X = %d, want nil for an absent attribute", int(*got.X))
				}
				if _, err := got.point(); err == nil {
					t.Fatal("point() error = nil, want a required-attribute failure")
				}
				return
			}
			if got.X == nil {
				t.Fatalf("X = nil, want %d", tc.wantX)
			}
			if int(*got.X) != tc.wantX {
				t.Fatalf("X = %d, want %d", int(*got.X), tc.wantX)
			}
		})
	}
}
