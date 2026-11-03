package probe

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/rs/zerolog"
)

// A geodata cache file holds the stored form of every listed region:
//
//	header  magic [8], version u32, geodata type u32, region count u32,
//	        CRC-32C u32 of the region data followed by the region table
//	table   per region: x u32, y u32, source size u64, source mtime i64
//	        (Unix ns), stored offset u64, stored length u64
//	data    each region's stored form, in table order
//
// All integers are little-endian. The table pins the exact region list and
// each source file's size and modification time, so a cache built from any
// other list or file contents is stale and is rebuilt, never used.
const (
	cacheMagic      = "ACISGEO\x00"
	cacheVersion    = 1
	cacheHeaderSize = 24
	cacheEntrySize  = 40
)

var cacheCRC = crc32.MakeTable(crc32.Castagnoli)

// sourceFile is one listed region file as the cache records it.
type sourceFile struct {
	region  Region
	size    int64
	modTime int64
}

// LoadRegionsCached is LoadRegions served from the geodata cache file at
// cachePath. A missing, stale, or corrupt cache is rebuilt from the region
// files first (written to a temporary file, synced, then renamed over
// cachePath, so a cache mapped by a running server is never modified).
// The cache is mapped read-only where the platform allows, and the mapping
// stays for the rest of the process. When a listed region file is missing,
// or the cache cannot be built or opened, it falls back to LoadRegions,
// which reports every failed region and fails the same way.
func LoadRegionsCached(dir string, geoType GeoType, regions []Region, cachePath string, log zerolog.Logger, options ...engine.Options) (*engine.Engine, error) {
	if !validGeoType(geoType) {
		return nil, fmt.Errorf("probe: unknown geodata type %q", geoType)
	}
	sources, err := statSources(dir, geoType, regions)
	if err != nil {
		return LoadRegions(dir, geoType, regions, log, options...)
	}

	stored, err := openCache(cachePath, geoType, sources)
	if err != nil {
		log.Info().Err(err).Str("geodata_cache", cachePath).Msg("geodata cache unusable; rebuilding it")
		if _, err := writeCache(cachePath, dir, geoType, sources, log); err != nil {
			log.Warn().Err(err).Str("geodata_cache", cachePath).Msg("geodata cache build failed; loading region files instead")
			return LoadRegions(dir, geoType, regions, log, options...)
		}
		if stored, err = openCache(cachePath, geoType, sources); err != nil {
			log.Warn().Err(err).Str("geodata_cache", cachePath).Msg("geodata cache open failed; loading region files instead")
			return LoadRegions(dir, geoType, regions, log, options...)
		}
	}

	e := engine.New(options...)
	for i, r := range regions {
		if err := e.SetRegion(r.X, r.Y, stored[i]); err != nil {
			return nil, err
		}
	}
	log.Info().Int("region_files", len(regions)).Str("geodata_type", string(geoType)).Str("geodata_cache", cachePath).Msg("geodata region files loaded")
	return e, nil
}

// WriteCache decodes the listed region files in dir into a new geodata
// cache file at cachePath, replacing any previous one atomically, and
// returns the stored regions' summed statistics.
func WriteCache(cachePath, dir string, geoType GeoType, regions []Region, log zerolog.Logger) (block.RegionStats, error) {
	if !validGeoType(geoType) {
		return block.RegionStats{}, fmt.Errorf("probe: unknown geodata type %q", geoType)
	}
	sources, err := statSources(dir, geoType, regions)
	if err != nil {
		return block.RegionStats{}, err
	}
	return writeCache(cachePath, dir, geoType, sources, log)
}

func statSources(dir string, geoType GeoType, regions []Region) ([]sourceFile, error) {
	sources := make([]sourceFile, len(regions))
	for i, r := range regions {
		info, err := os.Stat(regionPath(dir, geoType, r.X, r.Y))
		if err != nil {
			return nil, err
		}
		sources[i] = sourceFile{region: r, size: info.Size(), modTime: info.ModTime().UnixNano()}
	}
	return sources, nil
}

func geoTypeCode(geoType GeoType) uint32 {
	if geoType == L2J {
		return 2
	}
	return 1
}

func writeCache(cachePath, dir string, geoType GeoType, sources []sourceFile, log zerolog.Logger) (stats block.RegionStats, err error) {
	f, err := os.CreateTemp(filepath.Dir(cachePath), filepath.Base(cachePath)+".tmp*")
	if err != nil {
		return stats, fmt.Errorf("probe: create geodata cache: %w", err)
	}
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}()

	head := make([]byte, cacheHeaderSize+len(sources)*cacheEntrySize)
	if _, err := f.Write(head); err != nil {
		return stats, fmt.Errorf("probe: write geodata cache: %w", err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	crc := uint32(0)
	off := uint64(len(head))
	for i, src := range sources {
		path := regionPath(dir, geoType, src.region.X, src.region.Y)
		region, trailing, err := readRegion(path, geoType)
		if err != nil {
			return stats, fmt.Errorf("probe: load region %s: %w", path, err)
		}
		if trailing > 0 {
			log.Warn().Int("region_x", src.region.X).Int("region_y", src.region.Y).Int("trailing_bytes", trailing).Msg("geodata region has trailing bytes")
		}
		n, err := region.WriteTo(crcWriter{w: w, crc: &crc})
		if err != nil {
			return stats, fmt.Errorf("probe: write geodata cache: %w", err)
		}
		e := head[cacheHeaderSize+i*cacheEntrySize:]
		binary.LittleEndian.PutUint32(e, uint32(src.region.X))
		binary.LittleEndian.PutUint32(e[4:], uint32(src.region.Y))
		binary.LittleEndian.PutUint64(e[8:], uint64(src.size))
		binary.LittleEndian.PutUint64(e[16:], uint64(src.modTime))
		binary.LittleEndian.PutUint64(e[24:], off)
		binary.LittleEndian.PutUint64(e[32:], uint64(n))
		off += uint64(n)
		stats.Add(region.Stats())
	}
	if err := w.Flush(); err != nil {
		return stats, fmt.Errorf("probe: write geodata cache: %w", err)
	}

	copy(head, cacheMagic)
	binary.LittleEndian.PutUint32(head[8:], cacheVersion)
	binary.LittleEndian.PutUint32(head[12:], geoTypeCode(geoType))
	binary.LittleEndian.PutUint32(head[16:], uint32(len(sources)))
	binary.LittleEndian.PutUint32(head[20:], crc32.Update(crc, cacheCRC, head[cacheHeaderSize:]))
	if _, err := f.WriteAt(head, 0); err != nil {
		return stats, fmt.Errorf("probe: write geodata cache: %w", err)
	}
	if err := f.Sync(); err != nil {
		return stats, fmt.Errorf("probe: sync geodata cache: %w", err)
	}
	if err := f.Close(); err != nil {
		return stats, fmt.Errorf("probe: close geodata cache: %w", err)
	}
	if err := os.Rename(f.Name(), cachePath); err != nil {
		return stats, fmt.Errorf("probe: install geodata cache: %w", err)
	}
	syncDir(filepath.Dir(cachePath))
	return stats, nil
}

// syncDir makes a rename into dir durable; a platform that cannot sync a
// directory still has the renamed file.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}

type crcWriter struct {
	w   *bufio.Writer
	crc *uint32
}

func (c crcWriter) Write(p []byte) (int, error) {
	*c.crc = crc32.Update(*c.crc, cacheCRC, p)
	return c.w.Write(p)
}

// openCache maps the cache file and returns its regions in sources order,
// or an error when the file is missing, built for other sources, or
// corrupt.
func openCache(cachePath string, geoType GeoType, sources []sourceFile) ([]*block.Region, error) {
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	tableEnd := cacheHeaderSize + len(sources)*cacheEntrySize
	if info.Size() < int64(tableEnd) {
		return nil, fmt.Errorf("probe: geodata cache %s is truncated", cachePath)
	}

	b, err := mapFile(f, int(info.Size()))
	if err != nil {
		return nil, fmt.Errorf("probe: map geodata cache %s: %w", cachePath, err)
	}
	regions, err := parseCache(b, geoType, sources)
	if err != nil {
		unmapFile(b)
		return nil, fmt.Errorf("probe: geodata cache %s: %w", cachePath, err)
	}
	adviseRandom(b)
	return regions, nil
}

func parseCache(b []byte, geoType GeoType, sources []sourceFile) ([]*block.Region, error) {
	if string(b[:8]) != cacheMagic {
		return nil, errors.New("not a geodata cache")
	}
	if v := binary.LittleEndian.Uint32(b[8:]); v != cacheVersion {
		return nil, fmt.Errorf("format version %d, want %d", v, cacheVersion)
	}
	if binary.LittleEndian.Uint32(b[12:]) != geoTypeCode(geoType) || binary.LittleEndian.Uint32(b[16:]) != uint32(len(sources)) {
		return nil, errors.New("built for another geodata type or region list")
	}
	tableEnd := cacheHeaderSize + len(sources)*cacheEntrySize
	type span struct{ off, n uint64 }
	spans := make([]span, len(sources))
	for i, src := range sources {
		e := b[cacheHeaderSize+i*cacheEntrySize:]
		if binary.LittleEndian.Uint32(e) != uint32(src.region.X) || binary.LittleEndian.Uint32(e[4:]) != uint32(src.region.Y) {
			return nil, errors.New("built for another region list")
		}
		if binary.LittleEndian.Uint64(e[8:]) != uint64(src.size) || binary.LittleEndian.Uint64(e[16:]) != uint64(src.modTime) {
			return nil, fmt.Errorf("region %d_%d source file changed", src.region.X, src.region.Y)
		}
		spans[i] = span{off: binary.LittleEndian.Uint64(e[24:]), n: binary.LittleEndian.Uint64(e[32:])}
		if spans[i].off < uint64(tableEnd) || spans[i].off > uint64(len(b)) || spans[i].n > uint64(len(b))-spans[i].off {
			return nil, fmt.Errorf("region %d_%d lies outside the file", src.region.X, src.region.Y)
		}
	}
	crc := crc32.Update(crc32.Checksum(b[tableEnd:], cacheCRC), cacheCRC, b[cacheHeaderSize:tableEnd])
	if crc != binary.LittleEndian.Uint32(b[20:]) {
		return nil, errors.New("checksum mismatch")
	}

	regions := make([]*block.Region, len(sources))
	for i, s := range spans {
		r, err := block.ParseRegion(b[s.off : s.off+s.n : s.off+s.n])
		if err != nil {
			return nil, fmt.Errorf("region %d_%d: %w", sources[i].region.X, sources[i].region.Y, err)
		}
		regions[i] = r
	}
	return regions, nil
}
