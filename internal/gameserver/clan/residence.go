package clan

// HallOwner is one clanhall row's owner: the clan that owns hall HallID.
type HallOwner struct {
	HallID int32
	ClanID int32
}

// CastleID is the castle the clan owns, 0 when it owns none.
func (cl *Clan) CastleID() int32 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.castleID
}

// HallID is the clan hall the clan owns, 0 when it owns none.
func (cl *Clan) HallID() int32 {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.hallID
}

// RestoreHalls gives each owning clan its clan hall, once at boot after
// Restore. A row whose hall is not a known clan hall (exists reports false;
// nil accepts every hall) or whose clan no longer exists is skipped: the id
// factory already reset the stored owner of a vanished clan. When two rows
// name the same clan, the later one wins.
func (t *Table) RestoreHalls(owners []HallOwner, exists func(hallID int32) bool) {
	for _, o := range owners {
		if o.HallID <= 0 || o.ClanID <= 0 || (exists != nil && !exists(o.HallID)) {
			continue
		}
		cl, ok := t.Get(o.ClanID)
		if !ok {
			continue
		}
		cl.mu.Lock()
		cl.hallID = o.HallID
		cl.mu.Unlock()
	}
}

// SetCastleID sets the castle the clan owns, 0 for none. The castle
// manager is its only writer and stores the change itself.
func (cl *Clan) SetCastleID(id int32) {
	cl.mu.Lock()
	cl.castleID = id
	cl.mu.Unlock()
}
