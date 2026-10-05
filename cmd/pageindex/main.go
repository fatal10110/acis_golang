// Command pageindex writes the script page index: one record per page under
// the datapack's data/html/script tree, with its bypass commands,
// placeholders and content hash, but no page text. The committed copy is
// internal/testsupport/scriptpages/index.jsonl; a test fails when it no
// longer matches the datapack.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("pageindex", flag.ContinueOnError)
	fs.SetOutput(stderr)
	datapackDir := fs.String("datapack", "", "path to an unmodified aCis_datapack checkout (required)")
	outPath := fs.String("o", "", "file to write the index to; omit to write it to stdout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *datapackDir == "" || fs.NArg() != 0 {
		fs.Usage()
		return 2
	}

	index, err := scriptpages.Build(*datapackDir)
	if err != nil {
		fmt.Fprintln(stderr, "pageindex:", err)
		return 1
	}
	if *outPath == "" {
		_, err = stdout.Write(index)
	} else {
		err = os.WriteFile(*outPath, index, 0o644)
	}
	if err != nil {
		fmt.Fprintln(stderr, "pageindex: write:", err)
		return 1
	}
	return 0
}
