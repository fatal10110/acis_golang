package datadiff

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

type flattenMode uint8

const (
	flattenModeZero flattenMode = iota
	flattenModeOne
)

func (m flattenMode) String() string {
	switch m {
	case flattenModeZero:
		return "ZERO"
	case flattenModeOne:
		return "ONE"
	default:
		return "UNKNOWN"
	}
}

func TestFlatten_StructSlicesMapsAndStringers(t *testing.T) {
	type child struct {
		Name string
	}
	type sample struct {
		Title string
		Mode  flattenMode
		Score float32
		Flags []bool
		ByKey map[string]child
		Ptr   *child
	}

	got, err := Flatten(sample{
		Title: "alpha",
		Mode:  flattenModeOne,
		Score: 1.25,
		Flags: []bool{true, false},
		ByKey: map[string]child{
			"beta":  {Name: "b"},
			"alpha": {Name: "a"},
		},
		Ptr: &child{Name: "ptr"},
	})
	if err != nil {
		t.Fatalf("Flatten() error: %v", err)
	}

	want := map[string]string{
		"Title":             "alpha",
		"Mode":              "ONE",
		"Score":             "1.25",
		"Flags.len":         "2",
		"Flags[0]":          "true",
		"Flags[1]":          "false",
		"ByKey.len":         "2",
		"ByKey[alpha].Name": "a",
		"ByKey[beta].Name":  "b",
		"Ptr.Name":          "ptr",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Flatten() = %#v, want %#v", got, want)
	}
}

func TestFlatten_WriteDumpIsDeterministicForMaps(t *testing.T) {
	fields, err := Flatten(map[string]int{"z": 1, "a": 2})
	if err != nil {
		t.Fatalf("Flatten() error: %v", err)
	}

	var buf bytes.Buffer
	if err := WriteDump(&buf, []Record{{ID: "record", Fields: fields}}); err != nil {
		t.Fatalf("WriteDump() error: %v", err)
	}
	if got, want := buf.String(), "record\t[a]=2\t[z]=1\tlen=2\n"; got != want {
		t.Fatalf("WriteDump() = %q, want %q", got, want)
	}
}

func TestFlatten_RejectsUnsupportedMapKeyKinds(t *testing.T) {
	_, err := Flatten(map[struct{ ID int }]string{{ID: 1}: "x"})
	if err == nil {
		t.Fatal("Flatten() error = nil, want unsupported-map-key error")
	}
	if !strings.Contains(err.Error(), "unsupported map key kind") {
		t.Fatalf("Flatten() error = %v, want unsupported map key kind", err)
	}
}
