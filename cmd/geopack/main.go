// Command geopack builds the packed geodata cache the game server maps at
// boot from the region files geoengine.properties lists, and reports the
// stored size by block kind. Run it at deploy so the first boot does not
// build the cache itself.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/probe"
	"github.com/rs/zerolog"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("geopack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "config/geoengine.properties", "geoengine.properties listing the regions to pack")
	geodataDir := fs.String("geodata", "", "directory of geodata region files; defaults to -config's GeoDataPath")
	out := fs.String("out", "geodata.cache", "cache file to write; the game server's -geodata-cache")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	props, err := config.LoadFile(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "geopack: %v\n", err)
		return 1
	}
	dir := *geodataDir
	if dir == "" {
		dir = props.String("GeoDataPath", "")
	}
	geoType := probe.GeoType(props.String("GeoDataType", string(probe.L2OFF)))
	regions := probe.ListedRegions(props)

	stats, err := probe.WriteCache(*out, dir, geoType, regions, zerolog.New(stderr))
	if err != nil {
		fmt.Fprintf(stderr, "geopack: %v\n", err)
		return 1
	}
	total := stats.IndexBytes + stats.ComplexBytes + stats.MultilayerBytes
	fmt.Fprintf(stdout, "regions %d: flat %d, complex %d, multilayer %d blocks\n", len(regions), stats.FlatBlocks, stats.ComplexBlocks, stats.MultilayerBlocks)
	fmt.Fprintf(stdout, "stored bytes: index %d, complex %d, multilayer %d, total %d (%.1f MB)\n",
		stats.IndexBytes, stats.ComplexBytes, stats.MultilayerBytes, total, float64(total)/1e6)
	fmt.Fprintf(stdout, "wrote %s\n", *out)
	return 0
}
