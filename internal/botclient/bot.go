package botclient

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Behavior is one kind of action a bot takes per tick.
type Behavior string

// Behaviors a bot cycles through.
const (
	// BehaviorMove clicks a random point within the move radius of the
	// bot's entry position.
	BehaviorMove Behavior = "move"
	// BehaviorValidate reports the bot's last known position.
	BehaviorValidate Behavior = "validate"
	// BehaviorChat says a line on the all-chat channel.
	BehaviorChat Behavior = "chat"
	// BehaviorAttack targets the nearest attackable NPC the bot has seen
	// and attacks it; with none in sight it does nothing that tick.
	BehaviorAttack Behavior = "attack"
	// BehaviorIdle sends nothing that tick.
	BehaviorIdle Behavior = "idle"
)

// DefaultBehaviors is the cycle a bot runs unless configured otherwise.
var DefaultBehaviors = []Behavior{BehaviorMove, BehaviorValidate, BehaviorChat, BehaviorAttack, BehaviorIdle}

// botState is what the bot's reader goroutine learns from the server and
// its action loop acts on; mu guards every field.
type botState struct {
	mu      sync.Mutex
	self    Self
	dead    bool
	targets map[int32]npcSighting
	// loggingOut is set once Logout is sent. An ActionFailed after it (a
	// refused logout, as in combat, or one already in flight) is then
	// signaled on refused; LeaveWorld sets leftWorld.
	loggingOut bool
	leftWorld  bool
	refused    chan struct{}
}

// observe folds one server frame into the state.
func (s *botState) observe(raw []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch raw[0] {
	case serverpackets.OpcodeUserInfo:
		u := decodeUserInfo(raw)
		s.self.X, s.self.Y, s.self.Z, s.self.Heading = u.X, u.Y, u.Z, u.Heading
	case serverpackets.OpcodeNPCInfo:
		if n := decodeNPCInfo(raw); n.attackable {
			s.targets[n.objectID] = n
		}
	case serverpackets.OpcodeMoveToLocation, serverpackets.OpcodeStopMove,
		serverpackets.OpcodeValidateLocation, serverpackets.OpcodeTeleportToLocation:
		id, x, y, z := decodeObjectPosition(raw)
		if id == s.self.ObjectID {
			s.self.X, s.self.Y, s.self.Z = x, y, z
		} else if n, ok := s.targets[id]; ok {
			n.x, n.y, n.z = x, y, z
			s.targets[id] = n
		}
	case serverpackets.OpcodeDie:
		if id := decodeObjectID(raw); id == s.self.ObjectID {
			s.dead = true
		} else {
			delete(s.targets, id)
		}
	case serverpackets.OpcodeRevive:
		if decodeObjectID(raw) == s.self.ObjectID {
			s.dead = false
		}
	case serverpackets.OpcodeDeleteObject:
		delete(s.targets, decodeObjectID(raw))
	case serverpackets.OpcodeLeaveWorld:
		if s.loggingOut {
			s.leftWorld = true
		}
	case serverpackets.OpcodeActionFailed:
		if s.loggingOut {
			select {
			case s.refused <- struct{}{}:
			default:
			}
		}
	}
}

// nearest returns the closest attackable NPC the bot knows of.
func (s *botState) nearest() (npcSighting, bool) {
	var best npcSighting
	var bestDist int64 = -1
	for _, n := range s.targets {
		dx, dy := int64(n.x-s.self.X), int64(n.y-s.self.Y)
		if d := dx*dx + dy*dy; bestDist < 0 || d < bestDist {
			best, bestDist = n, d
		}
	}
	return best, bestDist >= 0
}

// bot is one simulated player.
type bot struct {
	cfg     *Config
	index   int
	account string
	rec     *recorder
	rng     *rand.Rand
}

// run plays one bot from login to logout, recording its metrics.
func (b *bot) run(ctx context.Context) {
	start := time.Now()
	res, err := Login(ctx, b.cfg.LoginAddr, b.account, b.cfg.Password, b.cfg.ServerID, b.cfg.Timeout)
	if err != nil {
		b.rec.fail(ctx, "login", err)
		return
	}
	b.rec.latency(&b.rec.connect, res.Connected.Sub(start))
	b.rec.latency(&b.rec.login, time.Since(res.Connected))

	addr := b.cfg.GameAddr
	if addr == "" {
		addr = res.GameAddr
	}
	start = time.Now()
	g, err := DialGame(ctx, addr, b.cfg.Revision, b.cfg.Timeout, &b.rec.counters)
	if err != nil {
		b.rec.fail(ctx, "game", err)
		return
	}
	defer g.Close()
	self, err := EnterGame(ctx, g, b.account, res.Keys, b.account)
	if err != nil {
		b.rec.fail(ctx, "game", err)
		return
	}
	b.rec.latency(&b.rec.enterWorld, time.Since(start))
	b.rec.entered.Add(1)

	b.play(ctx, g, self)
}

// play runs the behavior loop until ctx ends, the action budget is spent or
// the server closes the connection, then logs out.
func (b *bot) play(ctx context.Context, g *GameConn, self Self) {
	state := &botState{self: self, targets: make(map[int32]npcSighting), refused: make(chan struct{}, 1)}
	for _, raw := range self.Burst {
		state.observe(raw)
	}
	readErr := make(chan error, 1)
	go func() {
		for {
			raw, err := g.Read(time.Time{})
			if err != nil {
				readErr <- err
				return
			}
			if raw[0] == serverpackets.OpcodeServerClose {
				readErr <- errServerClose
				return
			}
			state.observe(raw)
		}
	}()

	var tick <-chan time.Time
	if b.cfg.ActionInterval > 0 {
		t := time.NewTicker(b.cfg.ActionInterval)
		defer t.Stop()
		tick = t.C
	}
	for n := 0; b.cfg.Actions == 0 || n < b.cfg.Actions; n++ {
		select {
		case err := <-readErr:
			b.rec.disconnect(err)
			return
		default:
		}
		if tick != nil {
			select {
			case <-ctx.Done():
			case err := <-readErr:
				b.rec.disconnect(err)
				return
			case <-tick:
			}
		}
		if ctx.Err() != nil {
			break
		}
		behavior := b.cfg.Behaviors[(b.index+n)%len(b.cfg.Behaviors)]
		if err := b.act(g, state, behavior, n); err != nil {
			b.rec.disconnect(err)
			g.Close()
			<-readErr
			return
		}
	}

	b.logout(g, state, readErr)
}

// logoutRefusalGrace is how long a logout waits for LeaveWorld after an
// ActionFailed: one already in flight before the Logout is followed by
// LeaveWorld and the close, a refusal by nothing.
const logoutRefusalGrace = time.Second

// logout sends Logout and waits for the server to close the connection.
// The server refuses a logout in combat and keeps the connection open, so a
// refusal, like a silent server, ends in closing the connection. The outcome
// is counted as action "logout" (LeaveWorld then the close),
// "logout_refused" or "logout_timeout".
func (b *bot) logout(g *GameConn, state *botState, readErr <-chan error) {
	state.mu.Lock()
	state.loggingOut = true
	state.mu.Unlock()
	if err := g.Send(encodeLogout()); err != nil {
		g.Close()
		<-readErr
		return
	}
	t := time.NewTimer(b.cfg.Timeout)
	defer t.Stop()
	refused := false
	for {
		select {
		case <-readErr:
			state.mu.Lock()
			left := state.leftWorld
			state.mu.Unlock()
			if left {
				b.rec.action("logout")
			} else {
				b.rec.disconnect(errors.New("closed after Logout without LeaveWorld"))
			}
			return
		case <-state.refused:
			if !refused {
				refused = true
				t.Reset(min(logoutRefusalGrace, b.cfg.Timeout))
			}
			continue
		case <-t.C:
			if refused {
				b.rec.action("logout_refused")
			} else {
				b.rec.action("logout_timeout")
			}
		}
		g.Close()
		<-readErr
		return
	}
}

// act sends the packets of one behavior tick.
func (b *bot) act(g *GameConn, state *botState, behavior Behavior, n int) error {
	state.mu.Lock()
	self, dead := state.self, state.dead
	target, haveTarget := state.nearest()
	state.mu.Unlock()

	if dead {
		b.rec.action("restart")
		return g.Send(encodeRequestRestartPoint())
	}
	switch behavior {
	case BehaviorMove:
		r := b.cfg.MoveRadius
		tx := self.X + b.rng.Int32N(2*r+1) - r
		ty := self.Y + b.rng.Int32N(2*r+1) - r
		b.rec.action(string(behavior))
		return g.Send(encodeMove(tx, ty, self.Z, self.X, self.Y, self.Z))
	case BehaviorValidate:
		b.rec.action(string(behavior))
		return g.Send(encodeValidatePosition(self.X, self.Y, self.Z, self.Heading))
	case BehaviorChat:
		b.rec.action(string(behavior))
		return g.Send(encodeSay(b.account + " says " + strconv.Itoa(n)))
	case BehaviorAttack:
		if !haveTarget {
			b.rec.action("attack_no_target")
			return nil
		}
		b.rec.action(string(behavior))
		if err := g.Send(encodeTargetAction(clientpackets.OpcodeAction, target.objectID, self.X, self.Y, self.Z)); err != nil {
			return err
		}
		return g.Send(encodeTargetAction(clientpackets.OpcodeAttackRequest, target.objectID, self.X, self.Y, self.Z))
	case BehaviorIdle:
		b.rec.action(string(behavior))
		return nil
	}
	return fmt.Errorf("unknown behavior %q", behavior)
}

// ParseBehaviors reads a comma-separated behavior list.
func ParseBehaviors(list []string) ([]Behavior, error) {
	if len(list) == 0 {
		return nil, errors.New("no behaviors")
	}
	out := make([]Behavior, 0, len(list))
	for _, s := range list {
		switch b := Behavior(s); b {
		case BehaviorMove, BehaviorValidate, BehaviorChat, BehaviorAttack, BehaviorIdle:
			out = append(out, b)
		default:
			return nil, fmt.Errorf("unknown behavior %q", s)
		}
	}
	return out, nil
}
