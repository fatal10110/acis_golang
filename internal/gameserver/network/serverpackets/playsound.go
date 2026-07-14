package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// OpcodePlaySound is the wire opcode for PlaySound.
const OpcodePlaySound = 0x98

// Sound describes a client-side sound playback request.
type Sound struct {
	Type         int32
	File         string
	BindToObject bool
	ObjectID     int32
	Location     location.Location
	Delay        int32
}

// FramePlaySound builds a static sound playback packet.
func FramePlaySound(file string) wire.Frame {
	return FramePlaySoundAt(Sound{File: file})
}

// FramePlaySoundAt builds a sound playback packet with all fields supplied.
func FramePlaySoundAt(sound Sound) wire.Frame {
	w := newFrameWriter(OpcodePlaySound)
	w.WriteInt32(sound.Type)
	w.WriteString(sound.File)
	w.WriteInt32(boolInt32(sound.BindToObject))
	w.WriteInt32(sound.ObjectID)
	w.WriteInt32(int32(sound.Location.X))
	w.WriteInt32(int32(sound.Location.Y))
	w.WriteInt32(int32(sound.Location.Z))
	w.WriteInt32(sound.Delay)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
