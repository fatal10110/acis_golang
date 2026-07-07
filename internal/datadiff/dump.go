package datadiff

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
)

// fieldSep separates a record's id and its "name=value" fields on a dump
// line; kvSep separates a field's name from its value. Neither may appear
// inside an id, a field name, or a field value, since the format has no
// escaping: a value that needs a literal tab or newline can't be dumped
// with this format and must fail loudly instead of corrupting the line.
const (
	fieldSep = "\t"
	kvSep    = "="
)

// WriteDump writes records to w in the harness's canonical dump format:
// one line per record, sorted by ID, formatted as
// "id<TAB>name=value<TAB>name=value…" with fields sorted by name. Two
// dumps of the same records — regardless of which implementation or
// language produced them — are byte-identical, which is what lets
// ReadDump feed both sides of a comparison. It returns an error, without
// writing a partial line, if any id, field name, or field value contains a
// tab or newline, or a field name contains '='.
func WriteDump(w io.Writer, records []Record) error {
	sorted := make([]Record, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	bw := bufio.NewWriter(w)
	for _, r := range sorted {
		line, err := formatRecordLine(r)
		if err != nil {
			return err
		}
		if _, err := bw.WriteString(line); err != nil {
			return fmt.Errorf("datadiff: write dump: %w", err)
		}
	}
	return bw.Flush()
}

// formatRecordLine renders one record as a single dump line, including its
// trailing newline.
func formatRecordLine(r Record) (string, error) {
	if strings.ContainsAny(r.ID, "\t\n") {
		return "", fmt.Errorf("datadiff: record id %q contains a tab or newline", r.ID)
	}

	names := make([]string, 0, len(r.Fields))
	for name := range r.Fields {
		names = append(names, name)
	}
	sort.Strings(names)

	var line strings.Builder
	line.WriteString(r.ID)
	for _, name := range names {
		val := r.Fields[name]
		if strings.ContainsAny(name, "\t\n=") {
			return "", fmt.Errorf("datadiff: record %q: field name %q contains a reserved character", r.ID, name)
		}
		if strings.ContainsAny(val, "\t\n") {
			return "", fmt.Errorf("datadiff: record %q: field %q value contains a tab or newline", r.ID, name)
		}
		line.WriteString(fieldSep)
		line.WriteString(name)
		line.WriteString(kvSep)
		line.WriteString(val)
	}
	line.WriteString("\n")
	return line.String(), nil
}

// ReadDump parses the canonical dump format written by WriteDump — or an
// equivalent dump produced by another implementation following the same
// format — into a slice of Records. A dump file is external input: a
// malformed line returns an error identifying it rather than a partial
// result or a panic. Blank lines are skipped.
func ReadDump(r io.Reader) ([]Record, error) {
	var records []Record
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if line == "" {
			continue
		}
		rec, err := parseRecordLine(line)
		if err != nil {
			return nil, fmt.Errorf("datadiff: dump line %d: %w", lineNo, err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("datadiff: read dump: %w", err)
	}
	return records, nil
}

// parseRecordLine parses one non-empty dump line (without its trailing
// newline) into a Record.
func parseRecordLine(line string) (Record, error) {
	parts := strings.Split(line, fieldSep)

	id := parts[0]
	if id == "" {
		return Record{}, fmt.Errorf("empty record id")
	}

	fields := make(map[string]string, len(parts)-1)
	for _, part := range parts[1:] {
		name, val, ok := strings.Cut(part, kvSep)
		if !ok {
			return Record{}, fmt.Errorf("field %q: missing %q separator", part, kvSep)
		}
		if name == "" {
			return Record{}, fmt.Errorf("field %q: empty field name", part)
		}
		fields[name] = val
	}
	return Record{ID: id, Fields: fields}, nil
}
