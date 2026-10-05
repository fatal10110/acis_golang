package xml

import (
	"encoding/xml"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/rs/zerolog"
)

type scriptListFile struct {
	XMLName xml.Name            `xml:"list"`
	Scripts []scriptListElement `xml:"script"`
}

type scriptListElement struct {
	Path     *string `xml:"path,attr"`
	Schedule string  `xml:"schedule,attr"`
	Start    string  `xml:"start,attr"`
	End      string  `xml:"end,attr"`
}

// LoadScriptList reads scripts.xml: the scripts to load, in file order,
// with their schedule attributes. An entry without a path is logged and
// skipped.
func LoadScriptList(path string, log zerolog.Logger) ([]script.Listing, error) {
	var doc scriptListFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("scripts: %w", err)
	}
	out := make([]script.Listing, 0, len(doc.Scripts))
	for _, el := range doc.Scripts {
		if el.Path == nil {
			log.Warn().Str("file", path).Msg("scripts: an entry has no path; skipped")
			continue
		}
		out = append(out, script.Listing{Path: *el.Path, Schedule: el.Schedule, Start: el.Start, End: el.End})
	}
	return out, nil
}
