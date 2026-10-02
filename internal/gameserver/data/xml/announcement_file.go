package xml

import (
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/rs/zerolog"
)

// announcementHeader opens a rewritten announcement file.
const announcementHeader = "<?xml version='1.0' encoding='utf-8'?> \n<!-- \n@param String message - the message to be announced \n@param Boolean critical - type of announcement (true = critical,false = normal) \n@param Boolean auto - when the announcement will be displayed (true = auto,false = on player login) \n@param Integer initial_delay - time delay for the first announce (used only if auto=true;value in seconds) \n@param Integer delay - time delay for the announces following the first announce (used only if auto=true;value in seconds) \n@param Integer limit - limit of announces (used only if auto=true, 0 = unlimited) \n--> \n"

// AnnouncementFile is the announcement file at Path: read at boot and
// rewritten whole on every change a game master makes.
type AnnouncementFile struct {
	Path string
	Log  zerolog.Logger
}

// Load reads the file's announcements. A file that is missing or not
// well-formed XML holds none, with a warning; a malformed schedule value is
// an error.
func (f AnnouncementFile) Load() ([]admin.Announcement, error) {
	list, err := LoadAnnouncements(f.Path)
	if err == nil {
		return list, nil
	}
	if _, malformed := errors.AsType[*xml.SyntaxError](err); malformed || errors.Is(err, fs.ErrNotExist) {
		f.Log.Warn().Err(err).Str("path", f.Path).Msg("Could not parse file")
		return nil, nil
	}
	return nil, err
}

// Save rewrites the file with list, in order. Each message is written as
// it is, unescaped.
func (f AnnouncementFile) Save(list []admin.Announcement) error {
	return os.WriteFile(f.Path, []byte(FormatAnnouncements(list)), 0o644)
}

// FormatAnnouncements is the announcement file holding list.
func FormatAnnouncements(list []admin.Announcement) string {
	var b strings.Builder
	b.WriteString(announcementHeader)
	b.WriteString("<list> \n")
	for _, a := range list {
		b.WriteString(`<announcement message="` + a.Message +
			`" critical="` + strconv.FormatBool(a.Critical) +
			`" auto="` + strconv.FormatBool(a.Auto) +
			`" initial_delay="` + strconv.Itoa(a.InitialDelay) +
			`" delay="` + strconv.Itoa(a.Delay) +
			`" limit="` + strconv.Itoa(a.Limit) + "\" /> \n")
	}
	b.WriteString("</list>")
	return b.String()
}
