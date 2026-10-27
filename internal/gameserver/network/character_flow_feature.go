package network

import "github.com/fatal10110/acis_golang/internal/gameserver/task"

// CharacterFlowConfig holds the collaborators of the character flow: the
// session check, the cipher choice and the clocks every character in the
// world is on.
type CharacterFlowConfig struct {
	Validator *SessionValidator
	// NoCipher keeps game packets cleartext after VersionCheck.
	NoCipher    bool
	PlayerClock *task.PlayerClock
	Autosave    *task.Autosave
}

// characterFlow is the character flow from AuthLogin through character
// selection and world entry to detach: the session check, the cipher
// choice, the clocks a character joins as it enters the world and leaves
// as it detaches, and the selections under way. NewGameClientLink builds it
// whole and nothing in it is replaced afterwards, so every client goroutine
// reads it without a lock; selections guards itself.
type characterFlow struct {
	validator   *SessionValidator
	noCipher    bool
	playerClock *task.PlayerClock
	autosave    *task.Autosave
	selections  selectingOwners
}

// newCharacterFlow builds the character flow from cfg.
func newCharacterFlow(cfg CharacterFlowConfig) characterFlow {
	return characterFlow{
		validator:   cfg.Validator,
		noCipher:    cfg.NoCipher,
		playerClock: cfg.PlayerClock,
		autosave:    cfg.Autosave,
	}
}
