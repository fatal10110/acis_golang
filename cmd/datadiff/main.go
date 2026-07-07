// Command datadiff dumps a data category's loaded records in a canonical,
// diffable text form, or compares that form against an equivalent dump
// from another implementation: matching load counts and matching field
// values are the way loader parity gets proven rather than assumed.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/fatal10110/acis_golang/internal/datadiff"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// Exit codes: 0 means success — either a dump was written, or a comparison
// found no differences. 1 means a requested comparison found differences;
// this is the code a script gates on. 2 means the command couldn't run at
// all (bad flags, a load or I/O failure).
const (
	exitOK        = 0
	exitDiffFound = 1
	exitError     = 2
)

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("datadiff", flag.ContinueOnError)
	fs.SetOutput(stderr)

	categoryName := fs.String("category", "", "data category to dump or compare; see -list")
	datapackDir := fs.String("datapack", "", "path to an aCis_datapack checkout to load -category from")
	dumpPath := fs.String("dump", "", "path to a previously written dump file, instead of -datapack")
	expectedDumpPath := fs.String("expected-dump", "", "path to a previously captured dump file to compare against; omit to just print the dump")
	list := fs.Bool("list", false, "list registered categories and exit")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: %s -category=<name> {-datapack=<dir> | -dump=<file>} [-expected-dump=<file>]\n\n", fs.Name())
		fmt.Fprintln(stderr, "Without -expected-dump, writes the category's dump to stdout.")
		fmt.Fprintln(stderr, "With -expected-dump, compares it against that file and reports the diff.")
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return exitError
	}

	if *list {
		for _, name := range sortedCategoryNames() {
			fmt.Fprintln(stdout, name)
		}
		return exitOK
	}

	records, exitCode := loadRecords(stderr, *categoryName, *datapackDir, *dumpPath)
	if exitCode != exitOK {
		return exitCode
	}

	if *expectedDumpPath == "" {
		if err := datadiff.WriteDump(stdout, records); err != nil {
			fmt.Fprintf(stderr, "datadiff: write dump: %v\n", err)
			return exitError
		}
		return exitOK
	}

	expectedFile, err := os.Open(*expectedDumpPath)
	if err != nil {
		fmt.Fprintf(stderr, "datadiff: open %s: %v\n", *expectedDumpPath, err)
		return exitError
	}
	defer expectedFile.Close()

	expectedRecords, err := datadiff.ReadDump(expectedFile)
	if err != nil {
		fmt.Fprintf(stderr, "datadiff: read %s: %v\n", *expectedDumpPath, err)
		return exitError
	}

	report, err := datadiff.Compare(expectedRecords, records)
	if err != nil {
		fmt.Fprintf(stderr, "datadiff: compare: %v\n", err)
		return exitError
	}

	printReport(stdout, *categoryName, report)
	if !report.Equal() {
		return exitDiffFound
	}
	return exitOK
}

// loadRecords resolves -category and exactly one of -datapack/-dump into
// a record set, or writes a usage/load error to stderr and returns the
// exit code the caller should return immediately.
func loadRecords(stderr io.Writer, categoryName, datapackDir, dumpPath string) ([]datadiff.Record, int) {
	if categoryName == "" {
		fmt.Fprintln(stderr, "datadiff: -category is required (see -list)")
		return nil, exitError
	}
	cat, ok := categories[categoryName]
	if !ok {
		fmt.Fprintf(stderr, "datadiff: unknown category %q (see -list)\n", categoryName)
		return nil, exitError
	}

	switch {
	case datapackDir != "" && dumpPath != "":
		fmt.Fprintln(stderr, "datadiff: only one of -datapack or -dump may be given")
		return nil, exitError

	case datapackDir != "":
		records, err := cat.load(datapackDir)
		if err != nil {
			fmt.Fprintf(stderr, "datadiff: load %s: %v\n", categoryName, err)
			return nil, exitError
		}
		return records, exitOK

	case dumpPath != "":
		f, err := os.Open(dumpPath)
		if err != nil {
			fmt.Fprintf(stderr, "datadiff: open %s: %v\n", dumpPath, err)
			return nil, exitError
		}
		defer f.Close()

		records, err := datadiff.ReadDump(f)
		if err != nil {
			fmt.Fprintf(stderr, "datadiff: read %s: %v\n", dumpPath, err)
			return nil, exitError
		}
		return records, exitOK

	default:
		fmt.Fprintln(stderr, "datadiff: one of -datapack or -dump is required")
		return nil, exitError
	}
}

// printReport writes a human-readable summary of report for category to w.
func printReport(w io.Writer, category string, report datadiff.Report) {
	fmt.Fprintf(w, "%s: expected=%d loaded=%d\n", category, report.CountWant, report.CountGot)

	if report.Equal() {
		fmt.Fprintln(w, "no differences")
		return
	}

	for _, id := range report.OnlyInWant {
		fmt.Fprintf(w, "only in expected dump: %s\n", id)
	}
	for _, id := range report.OnlyInGot {
		fmt.Fprintf(w, "only in loaded records: %s\n", id)
	}
	for _, m := range report.Mismatches {
		for _, d := range m.Diffs {
			fmt.Fprintf(w, "%s: field %s: expected=%q loaded=%q\n", m.ID, d.Field, d.Want, d.Got)
		}
	}
}
