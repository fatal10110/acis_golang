package xml

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/fatal10110/acis_golang/internal/gameserver/announcement"
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

// Load reads the file's announcements. A missing file holds none, with a
// warning; a file that is not well-formed XML, or holds a malformed
// schedule value, is an error, so that a broken file is never read as an
// empty one.
func (f AnnouncementFile) Load() ([]announcement.Announcement, error) {
	list, err := LoadAnnouncements(f.Path)
	if err == nil {
		return list, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		f.Log.Warn().Err(err).Str("path", f.Path).Msg("Could not parse file")
		return nil, nil
	}
	return nil, err
}

// Save rewrites the file with list, in order. The new content is written
// to a temporary file beside it and then renamed over it, so a save cut
// short leaves the previous file whole.
func (f AnnouncementFile) Save(list []announcement.Announcement) error {
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), "."+filepath.Base(f.Path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(FormatAnnouncements(list))
	if err == nil {
		err = tmp.Chmod(0o644)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.Path)
}

// FormatAnnouncements is the announcement file holding list.
func FormatAnnouncements(list []announcement.Announcement) string {
	var b strings.Builder
	b.WriteString(announcementHeader)
	b.WriteString("<list> \n")
	for _, a := range list {
		b.WriteString(`<announcement message="`)
		writeAttrValue(&b, a.Message)
		b.WriteString(`" critical="` + strconv.FormatBool(a.Critical) +
			`" auto="` + strconv.FormatBool(a.Auto) +
			`" initial_delay="` + strconv.Itoa(a.InitialDelay) +
			`" delay="` + strconv.Itoa(a.Delay) +
			`" limit="` + strconv.Itoa(a.Limit) + "\" /> \n")
	}
	b.WriteString("</list>")
	return b.String()
}

// writeAttrValue writes s as the value of a double-quoted attribute: the
// characters that would end or break it, and the white space a reader
// folds into spaces, as references, and a character XML cannot hold at all
// as U+FFFD. Any other character, apostrophes and '>' included, is written
// as it is, so plain text is written unchanged.
func writeAttrValue(b *strings.Builder, s string) {
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\t':
			b.WriteString("&#x9;")
		case r == '\n':
			b.WriteString("&#xA;")
		case r == '\r':
			b.WriteString("&#xD;")
		case r >= 0x20 && r <= 0xD7FF, r >= 0xE000 && r <= 0xFFFD, r >= 0x10000 && r <= 0x10FFFF:
			b.WriteRune(r)
		default:
			b.WriteRune(utf8.RuneError)
		}
	}
}
