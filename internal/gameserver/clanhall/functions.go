// Package clanhall runs the live state of the clan halls: the functions an
// owning clan rents for its hall (HP, MP and experience recovery,
// teleport, support magic, item creation and the two decorations), what
// they show inside the hall, and the fee each one charges the owning
// clan's warehouse when its term ends.
package clanhall

import (
	"context"
	"sync"
	"time"

	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// taskTimeout bounds one database read or write.
const taskTimeout = 10 * time.Second

// siegableLevelOffset is what a siegable hall adds to a rented function's
// decoration level.
const siegableLevelOffset = 10

// Function is one function a clan hall rents.
type Function struct {
	// Type is the function kind (hallmodel.FuncRestoreHP, ...).
	Type int
	// Level is the rented level: a percentage for the recovery functions,
	// a plain level for the others.
	Level int
	// Lease is the fee each term costs the owning clan's warehouse.
	Lease int
	// Rate is the length of one term, in milliseconds.
	Rate int64
	// EndTime is when the current term ends and the next fee is due, in
	// Unix milliseconds.
	EndTime int64
}

// StoredFunction is one clanhall_functions row.
type StoredFunction struct {
	HallID int32
	Function
}

// Store persists the rented functions.
type Store interface {
	// LoadFunctions returns every stored function.
	LoadFunctions(ctx context.Context) ([]StoredFunction, error)
	// SaveFunction stores f for hall hallID, replacing the row of the same
	// type.
	SaveFunction(ctx context.Context, hallID int32, f Function) error
	// DeleteFunction drops hall hallID's function of type funcType.
	DeleteFunction(ctx context.Context, hallID int32, funcType int) error
}

// Writer runs a database job later, on ownerID's persistence lane, so the
// fee timers never wait on the database.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// Owners answers which clan owns a hall.
type Owners interface {
	// HallOwner is the clan owning hall hallID, 0 when the hall is free.
	HallOwner(hallID int32) int32
}

// Treasury takes a function's fee out of a clan's warehouse.
type Treasury interface {
	// PayHallFee destroys adena from clan clanID's warehouse, reporting
	// false, with nothing taken, when it holds less.
	PayHallFee(clanID int32, adena int) bool
}

// Decoration is the decoration level of each function, as an owner's hall
// shows it to whoever enters.
type Decoration struct {
	HallID                           int32
	RestoreHP, RestoreMP, RestoreExp int
	Teleport, Curtains, SupportMagic int
	Fixtures, CreateItem             int
}

// rented is one live function and the timer of its next fee. gen counts
// the timers armed for it: a fee whose timer was replaced since it was
// armed carries an older gen and charges nothing.
type rented struct {
	Function
	timer *sim.Timer
	gen   uint64
}

// Functions holds the functions every clan hall rents. mu guards byHall,
// queue, treasury and every rented function; it is never held across an
// Owners or Treasury call, which take clan and item locks.
type Functions struct {
	halls  *hallmodel.Table
	decos  *hallmodel.DecoTable
	owners Owners
	store  Store
	writes Writer
	log    zerolog.Logger

	mu       sync.Mutex
	byHall   map[int32]map[int]*rented
	queue    *sim.Queue
	treasury Treasury
}

// New returns the clan hall functions, none rented. halls tells siegable
// halls apart, decos holds the decoration levels; owners answers who owns
// a hall; store, written through writes, persists the functions. A nil
// writes runs each write at once; a nil store writes nothing.
func New(halls *hallmodel.Table, decos *hallmodel.DecoTable, owners Owners, store Store, writes Writer, log zerolog.Logger) *Functions {
	return &Functions{
		halls:  halls,
		decos:  decos,
		owners: owners,
		store:  store,
		writes: writes,
		log:    log,
		byHall: map[int32]map[int]*rented{},
	}
}

// Restore loads the stored functions of every owned hall the hall data
// knows. A free hall's rows are left in the table and ignored. Call it
// once at boot, after the clans own their halls and before Start.
func (f *Functions) Restore(ctx context.Context) error {
	if f == nil || f.store == nil {
		return nil
	}
	rows, err := f.store.LoadFunctions(ctx)
	if err != nil {
		return err
	}
	kept := rows[:0]
	for _, row := range rows {
		if _, ok := f.halls.Get(int(row.HallID)); !ok {
			continue
		}
		if f.owners == nil || f.owners.HallOwner(row.HallID) == 0 {
			continue
		}
		kept = append(kept, row)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range kept {
		f.hallLocked(row.HallID)[row.Type] = &rented{Function: row.Function}
	}
	return nil
}

// Start charges the fees from now on, on queue, through treasury: every
// restored function whose term already ended pays at once, the others when
// their term ends.
func (f *Functions) Start(queue *sim.Queue, treasury Treasury) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue, f.treasury = queue, treasury
	nowMs := queue.Now().UnixMilli()
	for hallID, fns := range f.byHall {
		for _, fn := range fns {
			f.scheduleLocked(hallID, fn, fn.EndTime-nowMs)
		}
	}
}

// Level is the rented level of hall hallID's function of type funcType, 0
// when the hall rents none.
func (f *Functions) Level(hallID int32, funcType int) int {
	if f == nil {
		return 0
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if fn, ok := f.byHall[hallID][funcType]; ok {
		return fn.Level
	}
	return 0
}

// Decoration returns what hall hallID shows inside, or false when the hall
// data does not know the hall.
func (f *Functions) Decoration(hallID int32) (Decoration, bool) {
	if f == nil {
		return Decoration{}, false
	}
	hall, ok := f.halls.Get(int(hallID))
	if !ok {
		return Decoration{}, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	depth := func(funcType int) int {
		fn, ok := f.byHall[hallID][funcType]
		if !ok {
			return 0
		}
		return f.decos.Depth(funcType, DecoLevel(funcType, fn.Level, hall.IsSiegable()))
	}
	return Decoration{
		HallID:       hallID,
		RestoreHP:    depth(hallmodel.FuncRestoreHP),
		RestoreMP:    depth(hallmodel.FuncRestoreMP),
		RestoreExp:   depth(hallmodel.FuncRestoreExp),
		Teleport:     depth(hallmodel.FuncTeleport),
		Curtains:     depth(hallmodel.FuncDecoCurtains),
		SupportMagic: depth(hallmodel.FuncSupportMagic),
		Fixtures:     depth(hallmodel.FuncDecoFixtures),
		CreateItem:   depth(hallmodel.FuncCreateItem),
	}, true
}

// DecoLevel is the decoration level a function of type funcType rented at
// level shows: the HP recovery percentage over 20, the MP and experience
// recovery percentages over 5, the level itself for the rest. A siegable
// hall shows a rented one 10 levels higher. An unknown type shows none.
func DecoLevel(funcType, level int, siegable bool) int {
	var out int
	switch funcType {
	case hallmodel.FuncRestoreHP:
		out = level / 20
	case hallmodel.FuncRestoreMP, hallmodel.FuncRestoreExp:
		out = level / 5
	case hallmodel.FuncTeleport, hallmodel.FuncDecoCurtains, hallmodel.FuncSupportMagic,
		hallmodel.FuncDecoFixtures, hallmodel.FuncCreateItem:
		out = level
	default:
		return 0
	}
	if siegable && out > 0 {
		out += siegableLevelOffset
	}
	return out
}

func (f *Functions) hallLocked(hallID int32) map[int]*rented {
	fns := f.byHall[hallID]
	if fns == nil {
		fns = map[int]*rented{}
		f.byHall[hallID] = fns
	}
	return fns
}

// scheduleLocked arms fn's next fee after delayMs, at once when it is not
// positive. A function whose term has no length is never charged again: a
// zero-length term would charge without end.
func (f *Functions) scheduleLocked(hallID int32, fn *rented, delayMs int64) {
	if f.queue == nil {
		return
	}
	if fn.timer != nil {
		fn.timer.Stop()
		fn.timer = nil
	}
	fn.gen++
	gen := fn.gen
	fn.timer = f.queue.After(time.Duration(max(delayMs, 0))*time.Millisecond, func() {
		f.payFee(hallID, fn, gen)
	})
}

// payFee charges fn's fee to the clan owning hall hallID when its term
// ends. A free hall charges nothing and keeps the function as it is. Paid,
// the function's next term starts now; unpaid, the function is removed.
// gen is the generation the fee's timer was armed with; one replaced since
// charges nothing.
func (f *Functions) payFee(hallID int32, fn *rented, gen uint64) {
	f.mu.Lock()
	if fn.gen != gen || fn.timer == nil || f.byHall[hallID][fn.Type] != fn {
		f.mu.Unlock()
		return
	}
	fn.timer = nil
	lease, treasury := fn.Lease, f.treasury
	f.mu.Unlock()

	if f.owners == nil {
		return
	}
	clanID := f.owners.HallOwner(hallID)
	if clanID == 0 {
		return
	}
	paid := lease <= 0 || (treasury != nil && treasury.PayHallFee(clanID, lease))

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byHall[hallID][fn.Type] != fn {
		return
	}
	if !paid {
		delete(f.byHall[hallID], fn.Type)
		f.write("remove clan hall function", hallID, func(ctx context.Context, st Store) error {
			return st.DeleteFunction(ctx, hallID, fn.Type)
		})
		return
	}
	fn.EndTime = f.queue.Now().UnixMilli() + fn.Rate
	saved := fn.Function
	f.write("store clan hall function", hallID, func(ctx context.Context, st Store) error {
		return st.SaveFunction(ctx, hallID, saved)
	})
	if fn.Rate <= 0 {
		f.log.Warn().Int32("hall_id", hallID).Int("type", fn.Type).Msg("clanhall: function term has no length; its fee is not charged again")
		return
	}
	f.scheduleLocked(hallID, fn, fn.Rate)
}

// write queues fn on hallID's persistence lane, or runs it at once
// without a writer. Each write gets taskTimeout.
func (f *Functions) write(what string, hallID int32, fn func(context.Context, Store) error) {
	store, log := f.store, f.log
	if store == nil {
		return
	}
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Int32("hall_id", hallID).Msg("clanhall: " + what)
		}
	}
	if f.writes == nil {
		job()
		return
	}
	if !f.writes.Enqueue(hallID, job) {
		log.Error().Int32("hall_id", hallID).Msg("clanhall: " + what + ": write dropped")
	}
}
