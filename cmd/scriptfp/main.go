// Command scriptfp writes the static fingerprints of the reference server's
// script classes (reference.golden) and the census of the engine calls they
// make (census.golden, with the Go names of apimap.txt) into the scriptfp
// package directory. A test fails when the committed copies no longer
// match the reference source.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("scriptfp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	javaRoot := fs.String("java", "", "path to the reference server's java source directory (required)")
	outDir := fs.String("o", "", "directory to write reference.golden and census.golden to (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *javaRoot == "" || *outDir == "" || fs.NArg() != 0 {
		fs.Usage()
		return 2
	}

	fps, err := scriptfp.Build(*javaRoot)
	if err != nil {
		fmt.Fprintln(stderr, "scriptfp:", err)
		return 1
	}
	apimap, err := scriptfp.APIMap()
	if err != nil {
		fmt.Fprintln(stderr, "scriptfp:", err)
		return 1
	}
	for name, data := range map[string][]byte{
		"reference.golden": scriptfp.Render(fps),
		"census.golden":    scriptfp.Census(fps, apimap),
	} {
		if err := os.WriteFile(filepath.Join(*outDir, name), data, 0o644); err != nil {
			fmt.Fprintln(stderr, "scriptfp: write:", err)
			return 1
		}
	}
	return 0
}
