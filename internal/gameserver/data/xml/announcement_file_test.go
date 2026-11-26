package xml

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/announcement"
	"github.com/rs/zerolog"
)

// TestAnnouncementFileRoundTrip pins the rewritten file reading back as the
// list it was written from, an automatic announcement's negative limit
// reading back as none.
func TestAnnouncementFileRoundTrip(t *testing.T) {
	t.Parallel()
	file := AnnouncementFile{Path: filepath.Join(t.TempDir(), "announcements.xml"), Log: zerolog.Nop()}
	list := []announcement.Announcement{
		{Message: "Welcome", Critical: true},
		{Message: "Vote", Auto: true, InitialDelay: 5, Delay: 60, Limit: 3},
		{Message: "Forever", Auto: true, InitialDelay: -1, Delay: 10, Limit: -2},
	}
	if err := file.Save(list); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := file.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	list[2].Limit = 0
	if !reflect.DeepEqual(got, list) {
		t.Fatalf("Load = %+v, want %+v", got, list)
	}
}

// TestAnnouncementFileEscapes pins a message with characters that end or
// break an attribute value reading back as typed, the announcements around
// it surviving, while a plain message is written exactly as before.
func TestAnnouncementFileEscapes(t *testing.T) {
	t.Parallel()
	file := AnnouncementFile{Path: filepath.Join(t.TempDir(), "announcements.xml"), Log: zerolog.Nop()}
	list := []announcement.Announcement{
		{Message: "Welcome"},
		{Message: "Say \"hi\" & <b>bold</b> it's &amp; a\ttab\r\nline"},
		{Message: "Vote", Auto: true, InitialDelay: 5, Delay: 60, Limit: 3},
	}
	if err := file.Save(list); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := file.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, list) {
		t.Fatalf("Load = %+v, want %+v", got, list)
	}

	// A character XML cannot hold is saved as U+FFFD rather than breaking
	// the file.
	if err := file.Save([]announcement.Announcement{{Message: "bell\x07"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got, err := file.Load(); err != nil || len(got) != 1 || got[0].Message != "bell\uFFFD" {
		t.Fatalf("Load(control character) = %+v, %v, want bell\uFFFD", got, err)
	}

	plain := []announcement.Announcement{{Message: "Server  news $1 it's > 5", Critical: true}}
	want := announcementHeader + "<list> \n" +
		`<announcement message="Server  news $1 it's > 5" critical="true" auto="false" initial_delay="0" delay="0" limit="0" /> ` + "\n</list>"
	if got := FormatAnnouncements(plain); got != want {
		t.Fatalf("FormatAnnouncements(plain) =\n%s\nwant\n%s", got, want)
	}
	if err := file.Save(plain); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if raw, err := os.ReadFile(file.Path); err != nil || string(raw) != want {
		t.Fatalf("saved file = %q, %v, want %q", raw, err, want)
	}
	if entries, err := os.ReadDir(filepath.Dir(file.Path)); err != nil || len(entries) != 1 {
		t.Fatalf("directory after saves = %v, %v, want the file alone", entries, err)
	}
}

// TestAnnouncementFileUnreadable pins a missing file holding no
// announcement, while a file that is not well-formed, or holds a malformed
// schedule value, fails the load.
func TestAnnouncementFileUnreadable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := AnnouncementFile{Path: filepath.Join(dir, "missing.xml"), Log: zerolog.Nop()}
	if got, err := file.Load(); err != nil || got != nil {
		t.Fatalf("Load(missing) = %+v, %v, want none", got, err)
	}

	file.Path = filepath.Join(dir, "quoted.xml")
	if err := os.WriteFile(file.Path, []byte(`<list><announcement message="Say "hi"" /></list>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := file.Load(); err == nil {
		t.Fatalf("Load(malformed) = %+v, nil, want an error", got)
	}

	file.Path = filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(file.Path, []byte(`<list><announcement message="m" auto="true" initial_delay="x" delay="1" limit="1"/></list>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Load(); err == nil {
		t.Fatal("Load(bad schedule) succeeded, want an error")
	}
}
