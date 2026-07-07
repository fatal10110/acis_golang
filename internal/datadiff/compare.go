package datadiff

import (
	"fmt"
	"sort"
)

// absent renders a field that is present on only one side of a comparison,
// distinguishing a genuinely missing field from one explicitly set to an
// empty string.
const absent = "<absent>"

// Report is the result of comparing two record sets for the same
// category: a "want" side (e.g. an oracle-generated dump) and a "got" side
// (e.g. a loader's own dump).
type Report struct {
	CountWant, CountGot int

	// OnlyInWant/OnlyInGot list, sorted ascending, the ids present in only
	// one of the two compared sets.
	OnlyInWant []string
	OnlyInGot  []string

	// Mismatches lists, sorted by ID, every record present in both sets
	// whose field values differ.
	Mismatches []Mismatch
}

// Mismatch is one record present in both compared sets whose field values
// differ.
type Mismatch struct {
	ID    string
	Diffs []FieldDiff // sorted by field name
}

// FieldDiff is one field whose value differs between the two sets, or that
// is present on only one side.
type FieldDiff struct {
	Field     string
	Want, Got string
}

// Equal reports whether the compared sets agreed completely: equal counts,
// no ids unique to either side, and no field mismatches.
func (r Report) Equal() bool {
	return len(r.OnlyInWant) == 0 && len(r.OnlyInGot) == 0 && len(r.Mismatches) == 0
}

// Compare diffs want against got, each taken to be a category's full
// record set. It returns an error only for malformed input — a duplicate
// ID within one of the sets — since the sets genuinely differing is the
// expected, reportable case, not an error.
func Compare(want, got []Record) (Report, error) {
	wantByID, err := index(want)
	if err != nil {
		return Report{}, fmt.Errorf("datadiff: want set: %w", err)
	}
	gotByID, err := index(got)
	if err != nil {
		return Report{}, fmt.Errorf("datadiff: got set: %w", err)
	}

	report := Report{CountWant: len(wantByID), CountGot: len(gotByID)}

	for id := range wantByID {
		if _, ok := gotByID[id]; !ok {
			report.OnlyInWant = append(report.OnlyInWant, id)
		}
	}
	for id := range gotByID {
		if _, ok := wantByID[id]; !ok {
			report.OnlyInGot = append(report.OnlyInGot, id)
		}
	}
	sort.Strings(report.OnlyInWant)
	sort.Strings(report.OnlyInGot)

	for id, w := range wantByID {
		g, ok := gotByID[id]
		if !ok {
			continue
		}
		if diffs := diffFields(w.Fields, g.Fields); len(diffs) > 0 {
			report.Mismatches = append(report.Mismatches, Mismatch{ID: id, Diffs: diffs})
		}
	}
	sort.Slice(report.Mismatches, func(i, j int) bool { return report.Mismatches[i].ID < report.Mismatches[j].ID })

	return report, nil
}

// index builds a lookup of records by ID, and errors on a duplicate ID
// rather than silently keeping the last one — a duplicate means the
// record set itself is malformed, and comparing it would just compare
// against whichever half survived.
func index(records []Record) (map[string]Record, error) {
	m := make(map[string]Record, len(records))
	for _, r := range records {
		if _, exists := m[r.ID]; exists {
			return nil, fmt.Errorf("duplicate id %q", r.ID)
		}
		m[r.ID] = r
	}
	return m, nil
}

// diffFields compares two records' field maps and returns every field that
// differs, sorted by name. A field present in only one map compares as
// differing against absent.
func diffFields(want, got map[string]string) []FieldDiff {
	names := make(map[string]struct{}, len(want)+len(got))
	for name := range want {
		names[name] = struct{}{}
	}
	for name := range got {
		names[name] = struct{}{}
	}

	sortedNames := make([]string, 0, len(names))
	for name := range names {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	var diffs []FieldDiff
	for _, name := range sortedNames {
		w, wantOK := want[name]
		g, gotOK := got[name]
		switch {
		case wantOK && gotOK:
			if w != g {
				diffs = append(diffs, FieldDiff{Field: name, Want: w, Got: g})
			}
		case wantOK:
			diffs = append(diffs, FieldDiff{Field: name, Want: w, Got: absent})
		case gotOK:
			diffs = append(diffs, FieldDiff{Field: name, Want: absent, Got: g})
		}
	}
	return diffs
}
