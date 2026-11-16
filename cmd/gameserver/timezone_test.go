package main

import (
	"encoding/binary"
	"io/fs"
	"testing"
	"time"
)

// tzif returns a minimal zone file with one fixed offset.
func tzif(abbrev string, offset int32) []byte {
	data := append([]byte("TZif"), make([]byte, 16)...)
	for _, n := range []uint32{0, 0, 0, 0, 1, uint32(len(abbrev) + 1)} {
		data = binary.BigEndian.AppendUint32(data, n)
	}
	data = binary.BigEndian.AppendUint32(data, uint32(offset))
	data = append(data, 0, 0)
	return append(append(data, abbrev...), 0)
}

func readOnly(t *testing.T, path string, data []byte) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		if name != path {
			t.Fatalf("read %q, want %q", name, path)
		}
		return data, nil
	}
}

func zoneOffset(loc *time.Location) (string, int) {
	return time.Date(2026, 1, 15, 12, 0, 0, 0, loc).Zone()
}

// TestResolveZoneEmptyTZUsesHostZone: an empty TZ selects the host zone,
// as an unset one does, rather than UTC.
func TestResolveZoneEmptyTZUsesHostZone(t *testing.T) {
	loc, err := resolveZone("", readOnly(t, hostZoneFile, tzif("HST", -36000)))
	if err != nil {
		t.Fatal(err)
	}
	if abbrev, offset := zoneOffset(loc); abbrev != "HST" || offset != -36000 {
		t.Fatalf("zone = %s %d, want the host zone HST -36000", abbrev, offset)
	}

	loc, err = resolveZone("", func(string) ([]byte, error) { return nil, fs.ErrNotExist })
	if err != nil || loc != time.UTC {
		t.Fatalf("no host zone: resolveZone = (%v, %v), want (UTC, nil)", loc, err)
	}
}

// TestResolveZoneNamedTZ: a zone name, with or without a leading ':',
// loads from the zone database, the embedded copy when the host has none;
// an absolute path loads that file; an unknown name falls back to UTC.
func TestResolveZoneNamedTZ(t *testing.T) {
	noRead := func(name string) ([]byte, error) {
		t.Fatalf("read %q for a zone name", name)
		return nil, nil
	}
	for _, tz := range []string{"Asia/Tokyo", ":Asia/Tokyo"} {
		loc, err := resolveZone(tz, noRead)
		if err != nil {
			t.Fatalf("resolveZone(%q): %v", tz, err)
		}
		if _, offset := zoneOffset(loc); loc.String() != "Asia/Tokyo" || offset != 9*3600 {
			t.Errorf("resolveZone(%q) = %s offset %d, want Asia/Tokyo +9h", tz, loc, offset)
		}
	}

	loc, err := resolveZone("/zones/custom", readOnly(t, "/zones/custom", tzif("CST", 3600)))
	if err != nil {
		t.Fatal(err)
	}
	if abbrev, offset := zoneOffset(loc); abbrev != "CST" || offset != 3600 {
		t.Errorf("zone file = %s %d, want CST 3600", abbrev, offset)
	}

	if loc, err := resolveZone("No/Such_Zone", noRead); err == nil || loc != time.UTC {
		t.Errorf("unknown zone: resolveZone = (%v, %v), want (UTC, error)", loc, err)
	}
}
