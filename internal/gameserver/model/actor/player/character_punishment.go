package player

import (
	"math"
	"sync/atomic"
)

// Punishment is a punishment a character serves. Its value is the one the
// characters row stores in punish_level.
type Punishment int32

const (
	// PunishNone is no punishment.
	PunishNone Punishment = iota
	// PunishChat refuses the character's chat.
	PunishChat
	// PunishJail keeps the character in jail, unable to chat.
	PunishJail
	// PunishCharacter is a character ban.
	PunishCharacter
	// PunishAccount is an account ban.
	PunishAccount
)

// Description is how messages name p: "chat banned", "jailed", "banned".
func (p Punishment) Description() string {
	switch p {
	case PunishChat:
		return "chat banned"
	case PunishJail:
		return "jailed"
	case PunishCharacter, PunishAccount:
		return "banned"
	default:
		return ""
	}
}

// PunishmentChange is what SetPunishment did.
type PunishmentChange int

const (
	// PunishmentKept left the punishment as it was.
	PunishmentKept PunishmentChange = iota
	// ChatBanLifted ended a chat ban.
	ChatBanLifted
	// JailLifted ended a jail term.
	JailLifted
	// ChatBanStarted started a chat ban, or restarted the one served.
	ChatBanStarted
	// JailStarted started a jail term, or restarted the one served.
	JailStarted
)

// punishmentState is the punishment a character serves and its timer, in
// milliseconds. The owner's queue writes it; any goroutine reads it (a
// whisper reads its target's).
type punishmentState struct {
	kind  atomic.Int32
	timer atomic.Int64
}

// Punishment returns the punishment c serves and its timer in milliseconds,
// 0 for one without end.
func (c *Character) Punishment() (Punishment, int64) {
	return Punishment(c.punishment.kind.Load()), c.punishment.timer.Load()
}

// ChatBanned reports whether c serves a chat ban.
func (c *Character) ChatBanned() bool {
	return Punishment(c.punishment.kind.Load()) == PunishChat
}

// Jailed reports whether c is in jail.
func (c *Character) Jailed() bool {
	return Punishment(c.punishment.kind.Load()) == PunishJail
}

// RestorePunishment gives c the punishment its row stores: level is the
// stored punish_level, timer the stored punish_timer. A level that names no
// punishment leaves c unpunished; an unpunished character keeps no timer.
func (c *Character) RestorePunishment(level int, timer int64) {
	kind := PunishNone
	if level >= 0 && level <= int(PunishAccount) {
		kind = Punishment(level)
	}
	c.punishment.kind.Store(int32(kind))
	if kind == PunishNone {
		timer = 0
	}
	c.punishment.timer.Store(timer)
}

// SetPunishment applies kind to c for minutes, without end when minutes is
// not positive, and reports what changed. PunishNone lifts a chat ban or a
// jail term; a chat ban never replaces a jail term; a new chat ban or jail
// term replaces the one served, timer included. A lift leaves the timer to
// SetPunishmentTimer.
func (c *Character) SetPunishment(kind Punishment, minutes int32) PunishmentChange {
	current := Punishment(c.punishment.kind.Load())
	switch kind {
	case PunishNone:
		switch current {
		case PunishChat:
			c.punishment.kind.Store(int32(PunishNone))
			return ChatBanLifted
		case PunishJail:
			c.punishment.kind.Store(int32(PunishNone))
			return JailLifted
		}
		return PunishmentKept
	case PunishChat:
		if current == PunishJail {
			return PunishmentKept
		}
		c.punishment.kind.Store(int32(PunishChat))
		c.punishment.timer.Store(PunishmentMillis(minutes))
		return ChatBanStarted
	case PunishJail:
		c.punishment.kind.Store(int32(PunishJail))
		c.punishment.timer.Store(PunishmentMillis(minutes))
		return JailStarted
	}
	c.punishment.kind.Store(int32(kind))
	return PunishmentKept
}

// SetPunishmentTimer sets c's punishment timer to ms milliseconds.
func (c *Character) SetPunishmentTimer(ms int64) {
	c.punishment.timer.Store(ms)
}

// PunishmentMillis is the timer, in milliseconds, of a punishment lasting
// minutes: 0, without end, when minutes is not positive.
func PunishmentMillis(minutes int32) int64 {
	if minutes <= 0 {
		return 0
	}
	return int64(minutes) * 60000
}

// PunishmentMinutesLeft is ms rounded to the nearest minute, as the
// remaining-time reminder prints it: the division is single precision and
// the result saturates at the int32 range.
func PunishmentMinutesLeft(ms int64) int32 {
	rounded := math.Floor(float64(float32(ms)/60000) + 0.5)
	switch {
	case rounded >= math.MaxInt32:
		return math.MaxInt32
	case rounded <= math.MinInt32:
		return math.MinInt32
	}
	return int32(rounded)
}
