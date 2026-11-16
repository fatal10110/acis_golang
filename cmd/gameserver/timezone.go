package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	// A named TZ must resolve on a host without a zoneinfo database.
	_ "time/tzdata"

	"github.com/rs/zerolog"
)

// hostZoneFile holds the host's zone, used when TZ is unset or empty.
const hostZoneFile = "/etc/localtime"

// processZone is the zone the process clock runs in, resolved once in main
// before anything reads the clock.
type processZone struct {
	tz  string
	err error
}

// setProcessZone resolves the zone from TZ and installs it as time.Local.
// It runs first in main, before any goroutine starts or the clock is read.
func setProcessZone() processZone {
	tz := os.Getenv("TZ")
	loc, err := resolveZone(tz, os.ReadFile)
	time.Local = loc
	return processZone{tz: tz, err: err}
}

// resolveZone returns the zone for a TZ value. An empty TZ counts as unset
// and selects the host zone (UTC when the host has none). A name, with or
// without a leading ':', loads from the zone database, the embedded one
// when the host has none; an absolute path loads that file. A zone that
// cannot be loaded falls back to UTC and is returned with the error.
func resolveZone(tz string, readFile func(string) ([]byte, error)) (*time.Location, error) {
	name := strings.TrimPrefix(tz, ":")
	switch {
	case name == "":
		data, err := readFile(hostZoneFile)
		if errors.Is(err, fs.ErrNotExist) {
			return time.UTC, nil
		}
		return zoneFromData("Local", data, err)
	case filepath.IsAbs(name):
		data, err := readFile(name)
		return zoneFromData(name, data, err)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC, err
	}
	return loc, nil
}

func zoneFromData(name string, data []byte, readErr error) (*time.Location, error) {
	if readErr != nil {
		return time.UTC, readErr
	}
	loc, err := time.LoadLocationFromTZData(name, data)
	if err != nil {
		return time.UTC, err
	}
	return loc, nil
}

// logProcessZone logs the zone the boot timers run in.
func logProcessZone(zone processZone, log zerolog.Logger) {
	abbrev, offset := time.Now().Zone()
	if zone.err != nil {
		log.Warn().Err(zone.err).Str("tz", zone.tz).Msg("time zone not loadable; using UTC")
	}
	log.Info().Str("tz", zone.tz).Str("zone", time.Local.String()).Str("abbrev", abbrev).
		Dur("offset", time.Duration(offset)*time.Second).Msg("time zone")
}
