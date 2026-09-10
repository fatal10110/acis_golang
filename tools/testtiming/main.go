// Command testtiming summarizes Go test -json output without double-counting
// subtests. Pipe a complete test stream into it after a run.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type event struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
}

type result struct {
	packageName string
	test        string
	elapsed     float64
}

func main() {
	limit := flag.Int("n", 20, "number of slow packages and top-level tests to show")
	flag.Parse()
	output, err := summarize(os.Stdin, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fprint(os.Stdout, output)
}

func summarize(input io.Reader, limit int) (string, error) {
	if limit < 1 {
		limit = 1
	}
	var packages, tests []result
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		var e event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return "", fmt.Errorf("decode test event: %w", err)
		}
		if e.Action != "pass" || e.Elapsed == 0 {
			continue
		}
		if e.Test == "" {
			packages = append(packages, result{packageName: e.Package, elapsed: e.Elapsed})
		} else if !strings.Contains(e.Test, "/") {
			tests = append(tests, result{packageName: e.Package, test: e.Test, elapsed: e.Elapsed})
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read test events: %w", err)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].elapsed > packages[j].elapsed })
	sort.Slice(tests, func(i, j int) bool { return tests[i].elapsed > tests[j].elapsed })
	var out strings.Builder
	for _, r := range packages[:min(limit, len(packages))] {
		fmt.Fprintf(&out, "package %s %.3fs\n", r.packageName, r.elapsed)
	}
	for _, r := range tests[:min(limit, len(tests))] {
		fmt.Fprintf(&out, "test %s %s %.3fs\n", r.packageName, r.test, r.elapsed)
	}
	return out.String(), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func fprint(w io.Writer, text string) { _, _ = io.WriteString(w, text) }
