// Command geoprobe evaluates a sample of geodata queries (height, canMove,
// line-of-sight, path) against the Go geo engine, and reports how well its
// answers agree with a previously captured oracle dump.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/datadiff"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/probe"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// Exit codes: 0 means success — either a query sample was written, or a
// comparison against an oracle dump found no differences. 1 means a
// requested comparison found differences; this is the code a script gates
// on. 2 means the command couldn't run at all (bad flags, a load or I/O
// failure).
const (
	exitOK        = 0
	exitDiffFound = 1
	exitError     = 2
)

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("geoprobe", flag.ContinueOnError)
	fs.SetOutput(stderr)

	geodataDir := fs.String("geodata", "", "path to a directory of geodata region files; defaults to -config's GeoDataPath")
	geoTypeFlag := fs.String("geotype", "", "geodata file format, L2OFF or L2J; defaults to -config's GeoDataType, then L2OFF")
	configPath := fs.String("config", "", "path to geoengine.properties, for pathfinding weights and geodata defaults")
	n := fs.Int("queries", 1000, "number of random queries to generate; ignored when -expected-dump is given")
	seed := fs.Uint64("seed", 1, "seed for the random query generator, for a reproducible sample")
	dumpPath := fs.String("dump", "", "path to write the evaluated queries and Go's answers to; defaults to stdout when generating")
	expectedDumpPath := fs.String("expected-dump", "", "path to a captured oracle dump; re-evaluates its queries against Go and reports agreement, instead of generating a random sample")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s -geodata=<dir> [-geotype=L2OFF|L2J] [-config=<geoengine.properties>] {[-queries=N] [-seed=S] | -expected-dump=<file>} [-dump=<file>]\n\n", fs.Name())
		fmt.Fprintln(stderr, "Without -expected-dump, evaluates -queries random queries against the Go geo engine and writes them, with Go's answers, to -dump or stdout.")
		fmt.Fprintln(stderr, "With -expected-dump, re-evaluates that file's queries against the Go geo engine and reports agreement with its recorded answers.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return exitError
	}

	var props *config.Properties
	if *configPath != "" {
		p, err := config.LoadFile(*configPath)
		if err != nil {
			fmt.Fprintf(stderr, "geoprobe: load config: %v\n", err)
			return exitError
		}
		props = p
	}

	geoType := probe.GeoType(*geoTypeFlag)
	if geoType == "" {
		geoType = probe.L2OFF
		if props != nil {
			geoType = probe.GeoType(props.String("GeoDataType", string(probe.L2OFF)))
		}
	}

	dir := *geodataDir
	if dir == "" && props != nil {
		dir = props.String("GeoDataPath", "")
	}
	if dir == "" {
		fmt.Fprintln(stderr, "geoprobe: -geodata is required (or set GeoDataPath in -config)")
		return exitError
	}

	e, err := probe.LoadEngine(dir, geoType)
	if err != nil {
		fmt.Fprintf(stderr, "geoprobe: %v\n", err)
		return exitError
	}

	options := pathfind.DefaultOptions()
	if props != nil {
		if options, err = pathfind.OptionsFromProperties(props); err != nil {
			fmt.Fprintf(stderr, "geoprobe: %v\n", err)
			return exitError
		}
	}
	finder := pathfind.New(e, options)

	queries, expectedRecords, exitCode := resolveQueries(stderr, *expectedDumpPath, *n, *seed)
	if exitCode != exitOK {
		return exitCode
	}

	records := make([]datadiff.Record, len(queries))
	for i, q := range queries {
		records[i] = probe.Evaluate(e, finder, q)
	}

	if *expectedDumpPath == "" {
		if err := writeDump(*dumpPath, stdout, records); err != nil {
			fmt.Fprintf(stderr, "geoprobe: %v\n", err)
			return exitError
		}
		return exitOK
	}

	if *dumpPath != "" {
		if err := writeDump(*dumpPath, nil, records); err != nil {
			fmt.Fprintf(stderr, "geoprobe: %v\n", err)
			return exitError
		}
	}

	report, err := datadiff.Compare(expectedRecords, records)
	if err != nil {
		fmt.Fprintf(stderr, "geoprobe: compare: %v\n", err)
		return exitError
	}
	printReport(stdout, report, len(queries))
	if !report.Equal() {
		return exitDiffFound
	}
	return exitOK
}

// resolveQueries returns the queries to evaluate: parsed from
// expectedDumpPath's records when given, otherwise a fresh random sample.
// expectedRecords is non-nil only in the former case, for later
// comparison against Go's own evaluation of the same queries.
func resolveQueries(stderr io.Writer, expectedDumpPath string, n int, seed uint64) ([]probe.Query, []datadiff.Record, int) {
	if expectedDumpPath == "" {
		return probe.Random(n, seed), nil, exitOK
	}

	f, err := os.Open(expectedDumpPath)
	if err != nil {
		fmt.Fprintf(stderr, "geoprobe: open %s: %v\n", expectedDumpPath, err)
		return nil, nil, exitError
	}
	defer f.Close()

	expectedRecords, err := datadiff.ReadDump(f)
	if err != nil {
		fmt.Fprintf(stderr, "geoprobe: read %s: %v\n", expectedDumpPath, err)
		return nil, nil, exitError
	}

	queries := make([]probe.Query, len(expectedRecords))
	for i, r := range expectedRecords {
		q, err := probe.ParseQuery(r.ID)
		if err != nil {
			fmt.Fprintf(stderr, "geoprobe: %s: %v\n", expectedDumpPath, err)
			return nil, nil, exitError
		}
		queries[i] = q
	}
	return queries, expectedRecords, exitOK
}

// writeDump writes records in datadiff's canonical dump format to path, or
// to fallback when path is empty.
func writeDump(path string, fallback io.Writer, records []datadiff.Record) error {
	if path == "" {
		return datadiff.WriteDump(fallback, records)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	return datadiff.WriteDump(f, records)
}

// printReport writes a human-readable summary of a comparison against total
// queries evaluated, including the overall agreement percentage.
func printReport(w io.Writer, report datadiff.Report, total int) {
	disagreements := len(report.Mismatches) + len(report.OnlyInWant) + len(report.OnlyInGot)
	agreement := 100.0
	if total > 0 {
		agreement = 100 * float64(total-disagreements) / float64(total)
	}
	fmt.Fprintf(w, "queries=%d agreement=%.2f%%\n", total, agreement)

	if report.Equal() {
		fmt.Fprintln(w, "no differences")
		return
	}
	for _, id := range report.OnlyInWant {
		fmt.Fprintf(w, "only in expected dump: %s\n", id)
	}
	for _, id := range report.OnlyInGot {
		fmt.Fprintf(w, "only in evaluated queries: %s\n", id)
	}
	for _, m := range report.Mismatches {
		for _, d := range m.Diffs {
			fmt.Fprintf(w, "%s: field %s: expected=%q got=%q\n", m.ID, d.Field, d.Want, d.Got)
		}
	}
}
