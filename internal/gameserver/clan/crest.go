package clan

import (
	"context"
	"time"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// minCrestLevel is the clan level a clan needs to register a crest.
const minCrestLevel = 3

// crestIDTries bounds how many fresh ids a crest upload draws looking for
// one no stored crest of its family uses.
const crestIDTries = 8

// CrestResult is the outcome of a crest upload.
type CrestResult int

// The crest upload outcomes, in the order they are checked.
const (
	// CrestIgnored answers nothing: the uploader has no clan, there is no
	// crest to delete, or the image was not stored.
	CrestIgnored CrestResult = iota
	CrestDissolving
	CrestNotAuthorized
	CrestDeleted
	CrestLevelTooLow
	CrestRegistered
)

// SetCrest replaces c's clan's pledge or large pledge crest, typ, with
// data, or deletes it when data is empty. The new image is stored in files
// under a fresh id before the clan points at it; the replaced image is
// removed from files. The alliance crest is not set here.
func (s *Service) SetCrest(c *player.Character, typ datacache.CrestType, data []byte, files *datacache.Crests, now time.Time) (*Clan, CrestResult) {
	cl, ok := s.ClanOf(c)
	if !ok || (typ != datacache.PledgeCrest && typ != datacache.LargePledgeCrest) {
		return nil, CrestIgnored
	}
	if cl.Info().DissolvingExpiry > now.UnixMilli() {
		return cl, CrestDissolving
	}
	if !cl.HasPrivilege(c.ID, PrivEditCrest) {
		return cl, CrestNotAuthorized
	}
	if len(data) == 0 {
		if !s.changeCrest(cl, typ, 0, files) {
			return cl, CrestIgnored
		}
		return cl, CrestDeleted
	}
	if cl.Level() < minCrestLevel {
		return cl, CrestLevelTooLow
	}
	id, ok := s.newCrestID(typ, files)
	if !ok {
		return cl, CrestIgnored
	}
	if err := files.Save(typ, int(id), data); err != nil {
		s.log.Warn().Err(err).Int32("clan_id", cl.ID()).Msg("clan: crest not saved")
		return cl, CrestIgnored
	}
	s.changeCrest(cl, typ, id, files)
	return cl, CrestRegistered
}

// newCrestID draws a fresh object id for a typ crest. An id may come back
// that a crest stored before the last restart still uses, since crest ids
// are not reserved at boot; such an id is skipped so the upload never
// overwrites another clan's image.
func (s *Service) newCrestID(typ datacache.CrestType, files *datacache.Crests) (int32, bool) {
	if s.ids == nil {
		return 0, false
	}
	for range crestIDTries {
		id, err := s.ids.NextID()
		if err != nil {
			s.log.Error().Err(err).Msg("clan: allocate crest id")
			return 0, false
		}
		if !files.Has(typ, int(id)) {
			return id, true
		}
	}
	s.log.Error().Msg("clan: no free crest id")
	return 0, false
}

// changeCrest points cl's typ crest at id, queues the clan_data column and
// removes the image it pointed at before. Clearing a crest that is not set
// changes nothing and reports false.
func (s *Service) changeCrest(cl *Clan, typ datacache.CrestType, id int32, files *datacache.Crests) bool {
	cl.mu.Lock()
	field := cl.crestFieldLocked(typ)
	if field == nil || (id == 0 && *field == 0) {
		cl.mu.Unlock()
		return false
	}
	old := *field
	*field = id
	s.queueCrestLocked(cl, typ, id)
	cl.mu.Unlock()
	if old != 0 {
		if err := files.Remove(typ, int(old)); err != nil {
			s.log.Error().Err(err).Int32("clan_id", cl.id).Msg("clan: remove replaced crest")
		}
	}
	return true
}

// DropMissingCrests clears every clan crest id whose image files does not
// hold, as the clans are restored at boot before any player connects. With
// no crest cache at all nothing is cleared.
func (s *Service) DropMissingCrests(files *datacache.Crests) {
	if files == nil {
		return
	}
	for _, cl := range s.table.allClans() {
		cl.mu.Lock()
		for _, typ := range []datacache.CrestType{datacache.PledgeCrest, datacache.LargePledgeCrest, datacache.AllyCrest} {
			field := cl.crestFieldLocked(typ)
			if *field == 0 || files.Has(typ, int(*field)) {
				continue
			}
			s.log.Warn().Int32("clan_id", cl.id).Int("crest_type", int(typ)).Int32("crest_id", *field).Msg("clan: removing non-existent crest")
			*field = 0
			s.queueCrestLocked(cl, typ, 0)
		}
		cl.mu.Unlock()
	}
}

// crestFieldLocked is cl's crest id field of typ; cl.mu is held.
func (cl *Clan) crestFieldLocked(typ datacache.CrestType) *int32 {
	switch typ {
	case datacache.PledgeCrest:
		return &cl.crestID
	case datacache.LargePledgeCrest:
		return &cl.crestLargeID
	case datacache.AllyCrest:
		return &cl.allyCrestID
	}
	return nil
}

// queueCrestLocked queues cl's typ crest column as id; cl.mu is held.
func (s *Service) queueCrestLocked(cl *Clan, typ datacache.CrestType, id int32) {
	clanID := cl.id
	s.write(clanID, "update clan crest", func(ctx context.Context, st Store) error {
		return st.UpdateCrest(ctx, clanID, typ, id)
	})
}
