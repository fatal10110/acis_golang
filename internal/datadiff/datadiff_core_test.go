package datadiff

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// ---- from compare_test.go ----
func TestCompare_Equal(t *testing.T) {
	want := []Record{
		{ID: "1", Fields: map[string]string{"name": "a", "level": "5"}},
		{ID: "2", Fields: map[string]string{"name": "b", "level": "6"}},
	}
	got := []Record{
		{ID: "2", Fields: map[string]string{"name": "b", "level": "6"}},
		{ID: "1", Fields: map[string]string{"name": "a", "level": "5"}},
	}

	report, err := Compare(want, got)
	if err != nil {
		t.Fatalf("Compare() error: %v", err)
	}
	if !report.Equal() {
		t.Fatalf("Equal() = false, want true; report = %+v", report)
	}
	if report.CountWant != 2 || report.CountGot != 2 {
		t.Fatalf("counts = %d/%d, want 2/2", report.CountWant, report.CountGot)
	}
}

func TestCompare_OnlyInOneSide(t *testing.T) {
	want := []Record{
		{ID: "1", Fields: map[string]string{"name": "a"}},
		{ID: "2", Fields: map[string]string{"name": "b"}},
	}
	got := []Record{
		{ID: "2", Fields: map[string]string{"name": "b"}},
		{ID: "3", Fields: map[string]string{"name": "c"}},
	}

	report, err := Compare(want, got)
	if err != nil {
		t.Fatalf("Compare() error: %v", err)
	}
	if report.Equal() {
		t.Fatal("Equal() = true, want false")
	}
	if !reflect.DeepEqual(report.OnlyInWant, []string{"1"}) {
		t.Errorf("OnlyInWant = %v, want [1]", report.OnlyInWant)
	}
	if !reflect.DeepEqual(report.OnlyInGot, []string{"3"}) {
		t.Errorf("OnlyInGot = %v, want [3]", report.OnlyInGot)
	}
	if len(report.Mismatches) != 0 {
		t.Errorf("Mismatches = %v, want none", report.Mismatches)
	}
}

func TestCompare_FieldMismatch(t *testing.T) {
	want := []Record{
		{ID: "1", Fields: map[string]string{"name": "a", "level": "5"}},
	}
	got := []Record{
		{ID: "1", Fields: map[string]string{"name": "a", "level": "6"}},
	}

	report, err := Compare(want, got)
	if err != nil {
		t.Fatalf("Compare() error: %v", err)
	}
	if report.Equal() {
		t.Fatal("Equal() = true, want false")
	}
	if len(report.Mismatches) != 1 {
		t.Fatalf("Mismatches = %v, want 1 entry", report.Mismatches)
	}
	m := report.Mismatches[0]
	if m.ID != "1" {
		t.Errorf("Mismatch.ID = %q, want \"1\"", m.ID)
	}
	want1 := []FieldDiff{{Field: "level", Want: "5", Got: "6"}}
	if !reflect.DeepEqual(m.Diffs, want1) {
		t.Errorf("Diffs = %+v, want %+v", m.Diffs, want1)
	}
}

func TestCompare_FieldPresentOnOneSideOnly(t *testing.T) {
	want := []Record{
		{ID: "1", Fields: map[string]string{"name": "a", "extra": "x"}},
	}
	got := []Record{
		{ID: "1", Fields: map[string]string{"name": "a"}},
	}

	report, err := Compare(want, got)
	if err != nil {
		t.Fatalf("Compare() error: %v", err)
	}
	if len(report.Mismatches) != 1 {
		t.Fatalf("Mismatches = %v, want 1 entry", report.Mismatches)
	}
	diffs := report.Mismatches[0].Diffs
	wantDiffs := []FieldDiff{{Field: "extra", Want: "x", Got: absent}}
	if !reflect.DeepEqual(diffs, wantDiffs) {
		t.Errorf("Diffs = %+v, want %+v", diffs, wantDiffs)
	}
}

func TestCompare_EmptyStringFieldNotConfusedWithAbsent(t *testing.T) {
	want := []Record{{ID: "1", Fields: map[string]string{"name": ""}}}
	got := []Record{{ID: "1", Fields: map[string]string{"name": ""}}}

	report, err := Compare(want, got)
	if err != nil {
		t.Fatalf("Compare() error: %v", err)
	}
	if !report.Equal() {
		t.Fatalf("Equal() = false, want true; report = %+v", report)
	}
}

func TestCompare_DuplicateIDIsAnError(t *testing.T) {
	dup := []Record{
		{ID: "1", Fields: map[string]string{}},
		{ID: "1", Fields: map[string]string{}},
	}
	ok := []Record{{ID: "1", Fields: map[string]string{}}}

	if _, err := Compare(dup, ok); err == nil {
		t.Error("Compare(dup, ok) error = nil, want duplicate-id error")
	}
	if _, err := Compare(ok, dup); err == nil {
		t.Error("Compare(ok, dup) error = nil, want duplicate-id error")
	}
}

func TestCompare_MultipleMismatchesSortedByID(t *testing.T) {
	want := []Record{
		{ID: "3", Fields: map[string]string{"v": "a"}},
		{ID: "1", Fields: map[string]string{"v": "a"}},
		{ID: "2", Fields: map[string]string{"v": "a"}},
	}
	got := []Record{
		{ID: "3", Fields: map[string]string{"v": "b"}},
		{ID: "1", Fields: map[string]string{"v": "b"}},
		{ID: "2", Fields: map[string]string{"v": "b"}},
	}

	report, err := Compare(want, got)
	if err != nil {
		t.Fatalf("Compare() error: %v", err)
	}
	var ids []string
	for _, m := range report.Mismatches {
		ids = append(ids, m.ID)
	}
	if !sort.StringsAreSorted(ids) {
		t.Errorf("Mismatches not sorted by ID: %v", ids)
	}
	if !reflect.DeepEqual(ids, []string{"1", "2", "3"}) {
		t.Errorf("ids = %v, want [1 2 3]", ids)
	}
}

// ---- from dump_test.go ----
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

// ---- from flatten_test.go ----
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

// ---- from format_test.go ----
func TestFormatFloat(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{100, "100"},
		{1.5, "1.5"},
		{1.1, "1.1"},
		{1.123456, "1.123456"},
		{-1.5, "-1.5"},
		{-0.0, "0"},
		{0.05, "0.05"},
		{132.6, "132.6"},
		// A value whose 7th decimal digit sits exactly on a rounding tie: a
		// fixed-precision format would force a round-half-up-vs-even
		// choice here; the shortest round-trip form has no such ambiguity
		// and reproduces every digit the source literal actually needs.
		{244.2552175, "244.2552175"},
		{2889.881883, "2889.881883"},
	}
	for _, c := range cases {
		if got := FormatFloat(c.in); got != c.want {
			t.Errorf("FormatFloat(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
