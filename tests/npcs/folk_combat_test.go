package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// objectFrame reports whether frame is an opcode frame about object id.
func objectFrame(frame []byte, opcode byte, id int32) bool {
	return frame[0] == opcode && wire.NewReader(frame[1:]).ReadInt32() == id
}

// TestForcedClickAttacksFolk pins a CTRL click on a selected civilian NPC
// (Creature.onAction: isCtrlPressed && isAttackableBy): the player walks to
// it and swings at it, and once a swing lands the NPC loses HP, which the
// player, its targeter, reads.
func TestForcedClickAttacksFolk(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 80)
	w.selectFolk(t, f)
	maxHP := f.MaxHP()

	w.c.Send(encodeAttackRequest(f.ObjectID(), w.at))
	var frames [][]byte
	w.srv.AdvanceUntil(t, "a swing landing on the merchant", func() bool {
		frames = append(frames, drainFrames(t, w.c)...)
		return f.CurrentHP() < maxHP
	})
	frames = append(frames, drainFrames(t, w.c)...)

	if len(frames) == 0 || frames[0][0] != serverpackets.OpcodeMoveToPawn {
		t.Fatalf("forced click frames = %x, want the walk to the merchant first", opcodes(frames))
	}
	if _, _, distance := moveToPawn(t, frames[0]); distance <= 0 {
		t.Fatalf("attack walk distance = %d, want the attack range", distance)
	}
	attack, hp := -1, -1
	for i, frame := range frames {
		if attack < 0 && objectFrame(frame, serverpackets.OpcodeAttack, w.player) {
			attack = i
		}
		if hp < 0 && objectFrame(frame, serverpackets.OpcodeStatusUpdate, f.ObjectID()) {
			hp = i
		}
		if frame[0] == serverpackets.OpcodeNpcHtmlMessage {
			t.Fatal("the forced click opened the merchant's chat page")
		}
	}
	if attack < 0 || hp < attack {
		t.Fatalf("forced click frames = %x: Attack at %d, merchant StatusUpdate at %d, want the swing then the HP", opcodes(frames), attack, hp)
	}
}

// TestHitWalkingFolkStartsRunning pins Npc.reduceCurrentHp's
// setWalkOrRun(true): a hit puts a civilian NPC walking its route in walk
// stance into run stance, shown with ChangeMoveType then its NpcInfo; a
// second hit changes nothing.
func TestHitWalkingFolkStartsRunning(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	a := location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 400, Y: w.at.Y, Z: w.at.Z}
	f, _ := w.srv.SpawnRouteFolkNPCAt(t, walkerTemplate(), a, route.WalkerRoutes{walkerAlias: {walkerAlias: {{Location: a}, {Location: b}}}}, true)
	drainUntilQuiet(t, w.c)
	if f.Running() {
		t.Fatal("route walker spawned running, want walk stance")
	}
	attacker := w.onlineCharacter(t)

	f.TakeDamage(10, attacker)
	frames := drainFrames(t, w.c)
	moveType, info := -1, -1
	for i, frame := range frames {
		if moveType < 0 && objectFrame(frame, serverpackets.OpcodeChangeMoveType, f.ObjectID()) {
			moveType = i
			r := wire.NewReader(frame[1:])
			r.ReadInt32()
			if running := r.ReadInt32(); running != 1 {
				t.Fatalf("ChangeMoveType running = %d, want 1", running)
			}
		}
		if info < 0 && objectFrame(frame, serverpackets.OpcodeNPCInfo, f.ObjectID()) {
			info = i
		}
	}
	if !f.Running() || moveType < 0 || info < moveType {
		t.Fatalf("hit walker: running %v, frames %x (ChangeMoveType at %d, NpcInfo at %d), want run stance shown then NpcInfo", f.Running(), opcodes(frames), moveType, info)
	}

	f.TakeDamage(10, attacker)
	for _, frame := range drainFrames(t, w.c) {
		if objectFrame(frame, serverpackets.OpcodeChangeMoveType, f.ObjectID()) {
			t.Fatal("a second hit showed the stance change again")
		}
	}
}

// TestMonsterNeverAutoAttacksFolk pins Npc.canAutoAttack's `target
// instanceof Npc` refusal: a monster never picks a civilian NPC as an
// automatic target, even one standing outside any peace zone.
func TestMonsterNeverAutoAttacksFolk(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	f := w.spawnFolk(t, folkTemplate("Folk", 30100), 40)
	monster := w.srv.SpawnHostileNPCAt(t, location.Location{X: w.at.X - 40, Y: w.at.Y, Z: w.at.Z})
	if f.InPeaceZone() {
		t.Fatal("fixture civilian NPC stands in a peace zone")
	}
	if monster.AutoAttackTargetValid(f, 1000, true) {
		t.Fatal("monster may auto-attack a civilian NPC")
	}
	if !monster.AutoAttackTargetValid(w.onlineCharacter(t), 1000, true) {
		t.Fatal("control: monster may not auto-attack the player")
	}
}

// cubicOwner is a cubic's owner standing at its position with target
// selected, whose every roll is 0, attacking as attacker.
type cubicOwner struct {
	location.Location
	target   world.Tracked
	attacker skilltarget.Actor
}

func (o cubicOwner) ObjectID() int32         { return 1 }
func (o cubicOwner) Position() (x, y, z int) { return o.X, o.Y, o.Z }
func (o cubicOwner) Target() world.Tracked   { return o.target }
func (o cubicOwner) Roll(int) int            { return 0 }
func (o cubicOwner) CurrentHP() int          { return 1 }
func (o cubicOwner) MaxHPValue() float64     { return 1 }
func (o cubicOwner) Attacker() skilltarget.Actor {
	return o.attacker
}

// TestCubicNeverFiresAtFolk pins Cubic.pickEnemyTarget's
// isAttackableWithoutForceBy gate: a cubic whose owner has a civilian NPC
// selected fires at nothing, while a selected monster is fired at.
func TestCubicNeverFiresAtFolk(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 40)
	monster := w.srv.SpawnHostileNPCAt(t, location.Location{X: w.at.X - 40, Y: w.at.Y, Z: w.at.Z})

	attacker := w.onlineCharacter(t)
	if _, _, ok := actorcast.DecideCubicFire(cubicOwner{Location: w.at, target: f, attacker: attacker}, []int{4049}, 100); ok {
		t.Fatal("a cubic fires at a selected civilian NPC")
	}
	if _, target, ok := actorcast.DecideCubicFire(cubicOwner{Location: w.at, target: monster, attacker: attacker}, []int{4049}, 100); !ok || target.ObjectID() != monster.ObjectID() {
		t.Fatalf("control: cubic target = %v, %v, want the selected monster", target, ok)
	}
}

// onlineCharacter returns the world's one player character.
func (w *folkWorld) onlineCharacter(t *testing.T) *player.Character {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("player missing from world state")
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return ch
}
