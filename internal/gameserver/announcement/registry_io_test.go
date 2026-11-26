package announcement

import (
	"slices"
	"sync"
	"testing"
)

// fileStore is a store whose Save rewrites what Load reads, as the
// announcement file is. Each hook, when set, runs inside the matching call.
type fileStore struct {
	mu     sync.Mutex
	list   []Announcement
	saves  int
	onLoad func()
	onSave func()
}

func (s *fileStore) Load() ([]Announcement, error) {
	if s.onLoad != nil {
		s.onLoad()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.list), nil
}

func (s *fileStore) Save(list []Announcement) error {
	if s.onSave != nil {
		s.onSave()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = slices.Clone(list)
	s.saves++
	return nil
}

func (s *fileStore) held() ([]Announcement, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.list), s.saves
}

// A save writing the file holds none of the registry's state: the players
// entering the world, a listing and further changes go on meanwhile, and
// the change made then is written by the save after it, never undone by
// the one under way.
func TestSaveWritesOutsideTheLock(t *testing.T) {
	inSave, release := make(chan struct{}), make(chan struct{})
	store := &fileStore{}
	r, _, _ := newTestRegistry(t, store)
	r.Add(Announcement{Message: "first"})
	store.onSave = func() {
		store.onSave = nil
		close(inSave)
		<-release
	}
	saved := make(chan struct{})
	go func() {
		defer close(saved)
		r.Save()
	}()
	<-inSave

	if got := r.Login(); !slices.Equal(got, []Announcement{{Message: "first"}}) {
		t.Fatalf("Login() during a save = %+v, want first", got)
	}
	if err := r.Add(Announcement{Message: "second"}); err != nil {
		t.Fatalf("Add during a save: %v", err)
	}
	if got := len(r.List()); got != 2 {
		t.Fatalf("List() during a save holds %d, want 2", got)
	}
	close(release)
	<-saved
	if list, _ := store.held(); !slices.Equal(list, []Announcement{{Message: "first"}}) {
		t.Fatalf("file after the first save = %+v, want first", list)
	}

	r.Save()
	want := []Announcement{{Message: "first"}, {Message: "second"}}
	if list, saves := store.held(); !slices.Equal(list, want) || saves != 2 {
		t.Fatalf("file after the second save = %+v in %d saves, want %+v in 2", list, saves, want)
	}
	r.Save()
	if _, saves := store.held(); saves != 2 {
		t.Fatalf("saves with nothing changed = %d, want still 2", saves)
	}
}

// Load saves a change not yet written before it reads the file, and reads
// it again when a change comes while it reads, so it drops no change made
// before it returns.
func TestLoadKeepsChangesMadeBeforeIt(t *testing.T) {
	store := &fileStore{}
	r, _, _ := newTestRegistry(t, store)
	r.Add(Announcement{Message: "unsaved"})
	if err := r.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := r.Login(); !slices.Equal(got, []Announcement{{Message: "unsaved"}}) {
		t.Fatalf("Login() after Load = %+v, want the unsaved change", got)
	}

	inLoad, release := make(chan struct{}), make(chan struct{})
	store.onLoad = func() {
		store.onLoad = nil
		close(inLoad)
		<-release
	}
	loaded := make(chan error)
	go func() { loaded <- r.Load() }()
	<-inLoad
	r.Add(Announcement{Message: "during"})
	close(release)
	if err := <-loaded; err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	want := []Announcement{{Message: "unsaved"}, {Message: "during"}}
	if got := r.Login(); !slices.Equal(got, want) {
		t.Fatalf("Login() after a change during Load = %+v, want %+v", got, want)
	}
	if list, _ := store.held(); !slices.Equal(list, want) {
		t.Fatalf("file after Load = %+v, want %+v", list, want)
	}
}
