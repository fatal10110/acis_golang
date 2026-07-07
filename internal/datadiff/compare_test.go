package datadiff

import (
	"reflect"
	"sort"
	"testing"
)

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
