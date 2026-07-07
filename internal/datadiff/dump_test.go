package datadiff

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestWriteDump_SortedByID(t *testing.T) {
	records := []Record{
		{ID: "20", Fields: map[string]string{"name": "b"}},
		{ID: "3", Fields: map[string]string{"name": "a"}},
	}

	var buf bytes.Buffer
	if err := WriteDump(&buf, records); err != nil {
		t.Fatalf("WriteDump() error: %v", err)
	}

	const want = "20\tname=b\n3\tname=a\n"
	if buf.String() != want {
		t.Fatalf("WriteDump() =\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestWriteDump_FieldsSortedByName(t *testing.T) {
	records := []Record{
		{ID: "1", Fields: map[string]string{"zeta": "1", "alpha": "2"}},
	}

	var buf bytes.Buffer
	if err := WriteDump(&buf, records); err != nil {
		t.Fatalf("WriteDump() error: %v", err)
	}

	const want = "1\talpha=2\tzeta=1\n"
	if buf.String() != want {
		t.Fatalf("WriteDump() = %q, want %q", buf.String(), want)
	}
}

func TestWriteDump_RejectsReservedCharacters(t *testing.T) {
	cases := []struct {
		name    string
		records []Record
	}{
		{"tab in id", []Record{{ID: "1\t2", Fields: map[string]string{}}}},
		{"newline in id", []Record{{ID: "1\n2", Fields: map[string]string{}}}},
		{"equals in field name", []Record{{ID: "1", Fields: map[string]string{"a=b": "v"}}}},
		{"tab in field value", []Record{{ID: "1", Fields: map[string]string{"a": "v\tw"}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteDump(&buf, c.records); err == nil {
				t.Error("WriteDump() error = nil, want error")
			}
		})
	}
}

func TestReadDump_RoundTrip(t *testing.T) {
	records := []Record{
		{ID: "1", Fields: map[string]string{"name": "a", "level": "5"}},
		{ID: "2", Fields: map[string]string{"name": "b", "level": "6"}},
	}

	var buf bytes.Buffer
	if err := WriteDump(&buf, records); err != nil {
		t.Fatalf("WriteDump() error: %v", err)
	}

	got, err := ReadDump(&buf)
	if err != nil {
		t.Fatalf("ReadDump() error: %v", err)
	}

	sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })
	want := append([]Record(nil), records...)
	sort.Slice(want, func(i, j int) bool { return want[i].ID < want[j].ID })

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadDump() round trip = %+v, want %+v", got, want)
	}
}

func TestReadDump_SkipsBlankLines(t *testing.T) {
	input := "1\tname=a\n\n2\tname=b\n"
	records, err := ReadDump(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ReadDump() error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("ReadDump() returned %d records, want 2", len(records))
	}
}

func TestReadDump_RecordWithNoFields(t *testing.T) {
	records, err := ReadDump(strings.NewReader("1\n"))
	if err != nil {
		t.Fatalf("ReadDump() error: %v", err)
	}
	want := []Record{{ID: "1", Fields: map[string]string{}}}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("ReadDump() = %+v, want %+v", records, want)
	}
}

func TestReadDump_MalformedLines(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty id", "\tname=a\n"},
		{"field missing separator", "1\tnoequalssign\n"},
		{"field with empty name", "1\t=v\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ReadDump(strings.NewReader(c.input)); err == nil {
				t.Error("ReadDump() error = nil, want error")
			}
		})
	}
}
