package event

// DerbyLanes is how many monsters run one derby race.
const DerbyLanes = 8

// DerbySegments is how many speed steps a derby runner's race is split in.
const DerbySegments = 20

// DerbyRunner is one monster of a derby race as the race packet shows it.
type DerbyRunner struct {
	ObjectID        int32
	NpcID           int
	CollisionHeight float64
	CollisionRadius float64
}

// DerbyRace is the derby race packet: its two phase codes, the eight
// runners in lane order and the speed of each runner over each segment of
// the track. The client reads the speeds only when Code1 is 0, the race
// start; every other phase carries zeros instead.
type DerbyRace struct {
	Code1, Code2 int32
	Runners      [DerbyLanes]DerbyRunner
	Speeds       [DerbyLanes][DerbySegments]uint8
}

// DerbyPart is one packet of a derby track announcement.
type DerbyPart interface{ derbyPart() }

// DerbyMessage is a system message with its number parameters, in order.
type DerbyMessage struct {
	ID      int
	Numbers []int32
}

// DerbySound is a sound played at no location.
type DerbySound struct {
	Type int32
	File string
}

// DerbyRunnersGone removes the race's runners from the client, in lane
// order.
type DerbyRunnersGone struct{ ObjectIDs [DerbyLanes]int32 }

func (DerbyMessage) derbyPart()     {}
func (DerbySound) derbyPart()       {}
func (DerbyRace) derbyPart()        {}
func (DerbyRunnersGone) derbyPart() {}

// DerbyAnnounced reports what the derby track tells every player standing
// inside a derby track zone: Parts in order, zone by zone, so a player
// inside two of the zones hears it twice.
type DerbyAnnounced struct{ Parts []DerbyPart }

func (DerbyAnnounced) event() {}
