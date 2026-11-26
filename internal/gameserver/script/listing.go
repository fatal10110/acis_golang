package script

import (
	"encoding/xml"
	"fmt"
	"os"

	"github.com/rs/zerolog"
)

type listFile struct {
	XMLName xml.Name      `xml:"list"`
	Scripts []listElement `xml:"script"`
}

type listElement struct {
	Path     *string `xml:"path,attr"`
	Schedule string  `xml:"schedule,attr"`
	Start    string  `xml:"start,attr"`
	End      string  `xml:"end,attr"`
}

// LoadList reads scripts.xml: the scripts to load, in file order, with
// their schedule attributes. An entry without a path is logged and
// skipped.
func LoadList(path string, log zerolog.Logger) ([]Listing, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scripts: xml: read %s: %w", path, err)
	}
	var doc listFile
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("scripts: xml: parse %s: %w", path, err)
	}
	out := make([]Listing, 0, len(doc.Scripts))
	for _, el := range doc.Scripts {
		if el.Path == nil {
			log.Warn().Str("file", path).Msg("scripts: an entry has no path; skipped")
			continue
		}
		out = append(out, Listing{Path: *el.Path, Schedule: el.Schedule, Start: el.Start, End: el.End})
	}
	return out, nil
}
