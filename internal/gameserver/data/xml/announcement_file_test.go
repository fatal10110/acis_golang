package xml

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/rs/zerolog"
)

// TestAnnouncementFileRoundTrip pins the rewritten file reading back as the
// list it was written from, an automatic announcement's negative limit
// reading back as none.
func TestAnnouncementFileRoundTrip(t *testing.T) {
	t.Parallel()
	file := AnnouncementFile{Path: filepath.Join(t.TempDir(), "announcements.xml"), Log: zerolog.Nop()}
	list := []admin.Announcement{
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

// TestAnnouncementFileUnreadable pins a missing file, and one a message with
// a quote left malformed, holding no announcement, while a malformed
// schedule value fails the load.
func TestAnnouncementFileUnreadable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := AnnouncementFile{Path: filepath.Join(dir, "missing.xml"), Log: zerolog.Nop()}
	if got, err := file.Load(); err != nil || got != nil {
		t.Fatalf("Load(missing) = %+v, %v, want none", got, err)
	}

	file.Path = filepath.Join(dir, "quoted.xml")
	if err := file.Save([]admin.Announcement{{Message: `Say "hi"`}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got, err := file.Load(); err != nil || got != nil {
		t.Fatalf("Load(quoted) = %+v, %v, want none", got, err)
	}

	file.Path = filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(file.Path, []byte(`<list><announcement message="m" auto="true" initial_delay="x" delay="1" limit="1"/></list>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Load(); err == nil {
		t.Fatal("Load(bad schedule) succeeded, want an error")
	}
}
