package script

// Hooks holds a script's handlers, one func field per hook; a nil field
// does not react. The field list, the invokers, With and set are kept by
// hand and checked against each other by a test.
type Hooks struct {
	OnAbnormalStatusChanged    func(*Script, AbnormalStatusChanged)
	OnAttacked                 func(*Script, Attacked)
	OnAttackFinished           func(*Script, AttackFinished)
	OnClanAttacked             func(*Script, ClanAttacked)
	OnClanDied                 func(*Script, ClanDied)
	OnCreated                  func(*Script, Created)
	OnDecayed                  func(*Script, Decayed)
	OnEvent                    func(*Script, Event) string
	OnFirstTalk                func(*Script, FirstTalk) string
	OnItemUse                  func(*Script, ItemUse)
	OnMoveToFinished           func(*Script, MoveToFinished)
	OnMyDying                  func(*Script, MyDying)
	OnNoDesire                 func(*Script, NoDesire)
	OnOutOfTerritory           func(*Script, OutOfTerritory)
	OnPartyAttacked            func(*Script, PartyAttacked)
	OnPartyDied                func(*Script, PartyDied)
	OnPickedItem               func(*Script, PickedItem)
	OnScriptEvent              func(*Script, ScriptEvent)
	OnSeeCreature              func(*Script, SeeCreature)
	OnSeeItem                  func(*Script, SeeItem)
	OnSeeSpell                 func(*Script, SeeSpell)
	OnSpelled                  func(*Script, Spelled)
	OnStart                    func(*Script, Start)
	OnStaticObjectClanAttacked func(*Script, StaticObjectClanAttacked)
	OnTalk                     func(*Script, Talk) string
	OnTimer                    func(*Script, Timer) string
	OnUseSkillFinished         func(*Script, UseSkillFinished)
	OnZoneEnter                func(*Script, ZoneEnter)

	// own is the set of hooks the last With overlaid; derived reports that
	// With built these hooks. Hooks written as a literal own every hook
	// they set.
	own     hookSet
	derived bool
}

// hook names one Hooks field, in field order.
type hook uint8

const (
	hookAbnormalStatusChanged hook = iota
	hookAttacked
	hookAttackFinished
	hookClanAttacked
	hookClanDied
	hookCreated
	hookDecayed
	hookEvent
	hookFirstTalk
	hookItemUse
	hookMoveToFinished
	hookMyDying
	hookNoDesire
	hookOutOfTerritory
	hookPartyAttacked
	hookPartyDied
	hookPickedItem
	hookScriptEvent
	hookSeeCreature
	hookSeeItem
	hookSeeSpell
	hookSpelled
	hookStart
	hookStaticObjectClanAttacked
	hookTalk
	hookTimer
	hookUseSkillFinished
	hookZoneEnter
	hookCount
)

// hookMethods name the hooks as the registration manifest
// (testdata/oracle/manifest.golden) writes them.
var hookMethods = [hookCount]string{
	hookAbnormalStatusChanged:    "onAbnormalStatusChanged",
	hookAttacked:                 "onAttacked",
	hookAttackFinished:           "onAttackFinished",
	hookClanAttacked:             "onClanAttacked",
	hookClanDied:                 "onClanDied",
	hookCreated:                  "onCreated",
	hookDecayed:                  "onDecayed",
	hookEvent:                    "onAdvEvent",
	hookFirstTalk:                "onFirstTalk",
	hookItemUse:                  "onItemUse",
	hookMoveToFinished:           "onMoveToFinished",
	hookMyDying:                  "onMyDying",
	hookNoDesire:                 "onNoDesire",
	hookOutOfTerritory:           "onOutOfTerritory",
	hookPartyAttacked:            "onPartyAttacked",
	hookPartyDied:                "onPartyDied",
	hookPickedItem:               "onPickedItem",
	hookScriptEvent:              "onScriptEvent",
	hookSeeCreature:              "onSeeCreature",
	hookSeeItem:                  "onSeeItem",
	hookSeeSpell:                 "onSeeSpell",
	hookSpelled:                  "onSpelled",
	hookStart:                    "onStart",
	hookStaticObjectClanAttacked: "onStaticObjectClanAttacked",
	hookTalk:                     "onTalk",
	hookTimer:                    "onTimer",
	hookUseSkillFinished:         "onUseSkillFinished",
	hookZoneEnter:                "onZoneEnter",
}

func (h hook) String() string { return hookMethods[h] }

// hookSet is a set of hooks, one bit per hook.
type hookSet uint32

func (s hookSet) has(h hook) bool { return s&(1<<h) != 0 }

// set returns the hooks h reacts with.
func (h *Hooks) set() hookSet {
	var s hookSet
	add := func(set bool, k hook) {
		if set {
			s |= 1 << k
		}
	}
	add(h.OnAbnormalStatusChanged != nil, hookAbnormalStatusChanged)
	add(h.OnAttacked != nil, hookAttacked)
	add(h.OnAttackFinished != nil, hookAttackFinished)
	add(h.OnClanAttacked != nil, hookClanAttacked)
	add(h.OnClanDied != nil, hookClanDied)
	add(h.OnCreated != nil, hookCreated)
	add(h.OnDecayed != nil, hookDecayed)
	add(h.OnEvent != nil, hookEvent)
	add(h.OnFirstTalk != nil, hookFirstTalk)
	add(h.OnItemUse != nil, hookItemUse)
	add(h.OnMoveToFinished != nil, hookMoveToFinished)
	add(h.OnMyDying != nil, hookMyDying)
	add(h.OnNoDesire != nil, hookNoDesire)
	add(h.OnOutOfTerritory != nil, hookOutOfTerritory)
	add(h.OnPartyAttacked != nil, hookPartyAttacked)
	add(h.OnPartyDied != nil, hookPartyDied)
	add(h.OnPickedItem != nil, hookPickedItem)
	add(h.OnScriptEvent != nil, hookScriptEvent)
	add(h.OnSeeCreature != nil, hookSeeCreature)
	add(h.OnSeeItem != nil, hookSeeItem)
	add(h.OnSeeSpell != nil, hookSeeSpell)
	add(h.OnSpelled != nil, hookSpelled)
	add(h.OnStart != nil, hookStart)
	add(h.OnStaticObjectClanAttacked != nil, hookStaticObjectClanAttacked)
	add(h.OnTalk != nil, hookTalk)
	add(h.OnTimer != nil, hookTimer)
	add(h.OnUseSkillFinished != nil, hookUseSkillFinished)
	add(h.OnZoneEnter != nil, hookZoneEnter)
	return s
}

// ownSet returns the hooks h itself sets, as opposed to the ones it took
// from the parent With was called on.
func (h *Hooks) ownSet() hookSet {
	if h.derived {
		return h.own
	}
	return h.set()
}

// With returns h with every hook o sets replaced by o's: a child behavior
// overriding its parent's handlers.
func (h Hooks) With(o Hooks) Hooks {
	pick := func(dst *Hooks) {
		if o.OnAbnormalStatusChanged != nil {
			dst.OnAbnormalStatusChanged = o.OnAbnormalStatusChanged
		}
		if o.OnAttacked != nil {
			dst.OnAttacked = o.OnAttacked
		}
		if o.OnAttackFinished != nil {
			dst.OnAttackFinished = o.OnAttackFinished
		}
		if o.OnClanAttacked != nil {
			dst.OnClanAttacked = o.OnClanAttacked
		}
		if o.OnClanDied != nil {
			dst.OnClanDied = o.OnClanDied
		}
		if o.OnCreated != nil {
			dst.OnCreated = o.OnCreated
		}
		if o.OnDecayed != nil {
			dst.OnDecayed = o.OnDecayed
		}
		if o.OnEvent != nil {
			dst.OnEvent = o.OnEvent
		}
		if o.OnFirstTalk != nil {
			dst.OnFirstTalk = o.OnFirstTalk
		}
		if o.OnItemUse != nil {
			dst.OnItemUse = o.OnItemUse
		}
		if o.OnMoveToFinished != nil {
			dst.OnMoveToFinished = o.OnMoveToFinished
		}
		if o.OnMyDying != nil {
			dst.OnMyDying = o.OnMyDying
		}
		if o.OnNoDesire != nil {
			dst.OnNoDesire = o.OnNoDesire
		}
		if o.OnOutOfTerritory != nil {
			dst.OnOutOfTerritory = o.OnOutOfTerritory
		}
		if o.OnPartyAttacked != nil {
			dst.OnPartyAttacked = o.OnPartyAttacked
		}
		if o.OnPartyDied != nil {
			dst.OnPartyDied = o.OnPartyDied
		}
		if o.OnPickedItem != nil {
			dst.OnPickedItem = o.OnPickedItem
		}
		if o.OnScriptEvent != nil {
			dst.OnScriptEvent = o.OnScriptEvent
		}
		if o.OnSeeCreature != nil {
			dst.OnSeeCreature = o.OnSeeCreature
		}
		if o.OnSeeItem != nil {
			dst.OnSeeItem = o.OnSeeItem
		}
		if o.OnSeeSpell != nil {
			dst.OnSeeSpell = o.OnSeeSpell
		}
		if o.OnSpelled != nil {
			dst.OnSpelled = o.OnSpelled
		}
		if o.OnStart != nil {
			dst.OnStart = o.OnStart
		}
		if o.OnStaticObjectClanAttacked != nil {
			dst.OnStaticObjectClanAttacked = o.OnStaticObjectClanAttacked
		}
		if o.OnTalk != nil {
			dst.OnTalk = o.OnTalk
		}
		if o.OnTimer != nil {
			dst.OnTimer = o.OnTimer
		}
		if o.OnUseSkillFinished != nil {
			dst.OnUseSkillFinished = o.OnUseSkillFinished
		}
		if o.OnZoneEnter != nil {
			dst.OnZoneEnter = o.OnZoneEnter
		}
	}
	out := h
	pick(&out)
	out.own = o.set()
	out.derived = true
	return out
}

// The invokers run a hook when it is set and do nothing otherwise, so a
// parent call ports as one call whether or not an ancestor reacts. A
// string hook that is not set answers "".

func (h *Hooks) AbnormalStatusChanged(s *Script, e AbnormalStatusChanged) {
	if h.OnAbnormalStatusChanged != nil {
		h.OnAbnormalStatusChanged(s, e)
	}
}

func (h *Hooks) Attacked(s *Script, e Attacked) {
	if h.OnAttacked != nil {
		h.OnAttacked(s, e)
	}
}

func (h *Hooks) AttackFinished(s *Script, e AttackFinished) {
	if h.OnAttackFinished != nil {
		h.OnAttackFinished(s, e)
	}
}

func (h *Hooks) ClanAttacked(s *Script, e ClanAttacked) {
	if h.OnClanAttacked != nil {
		h.OnClanAttacked(s, e)
	}
}

func (h *Hooks) ClanDied(s *Script, e ClanDied) {
	if h.OnClanDied != nil {
		h.OnClanDied(s, e)
	}
}

func (h *Hooks) Created(s *Script, e Created) {
	if h.OnCreated != nil {
		h.OnCreated(s, e)
	}
}

func (h *Hooks) Decayed(s *Script, e Decayed) {
	if h.OnDecayed != nil {
		h.OnDecayed(s, e)
	}
}

func (h *Hooks) Event(s *Script, e Event) string {
	if h.OnEvent != nil {
		return h.OnEvent(s, e)
	}
	return ""
}

func (h *Hooks) FirstTalk(s *Script, e FirstTalk) string {
	if h.OnFirstTalk != nil {
		return h.OnFirstTalk(s, e)
	}
	return ""
}

func (h *Hooks) ItemUse(s *Script, e ItemUse) {
	if h.OnItemUse != nil {
		h.OnItemUse(s, e)
	}
}

func (h *Hooks) MoveToFinished(s *Script, e MoveToFinished) {
	if h.OnMoveToFinished != nil {
		h.OnMoveToFinished(s, e)
	}
}

func (h *Hooks) MyDying(s *Script, e MyDying) {
	if h.OnMyDying != nil {
		h.OnMyDying(s, e)
	}
}

func (h *Hooks) NoDesire(s *Script, e NoDesire) {
	if h.OnNoDesire != nil {
		h.OnNoDesire(s, e)
	}
}

func (h *Hooks) OutOfTerritory(s *Script, e OutOfTerritory) {
	if h.OnOutOfTerritory != nil {
		h.OnOutOfTerritory(s, e)
	}
}

func (h *Hooks) PartyAttacked(s *Script, e PartyAttacked) {
	if h.OnPartyAttacked != nil {
		h.OnPartyAttacked(s, e)
	}
}

func (h *Hooks) PartyDied(s *Script, e PartyDied) {
	if h.OnPartyDied != nil {
		h.OnPartyDied(s, e)
	}
}

func (h *Hooks) PickedItem(s *Script, e PickedItem) {
	if h.OnPickedItem != nil {
		h.OnPickedItem(s, e)
	}
}

func (h *Hooks) ScriptEvent(s *Script, e ScriptEvent) {
	if h.OnScriptEvent != nil {
		h.OnScriptEvent(s, e)
	}
}

func (h *Hooks) SeeCreature(s *Script, e SeeCreature) {
	if h.OnSeeCreature != nil {
		h.OnSeeCreature(s, e)
	}
}

func (h *Hooks) SeeItem(s *Script, e SeeItem) {
	if h.OnSeeItem != nil {
		h.OnSeeItem(s, e)
	}
}

func (h *Hooks) SeeSpell(s *Script, e SeeSpell) {
	if h.OnSeeSpell != nil {
		h.OnSeeSpell(s, e)
	}
}

func (h *Hooks) Spelled(s *Script, e Spelled) {
	if h.OnSpelled != nil {
		h.OnSpelled(s, e)
	}
}

func (h *Hooks) Start(s *Script, e Start) {
	if h.OnStart != nil {
		h.OnStart(s, e)
	}
}

func (h *Hooks) StaticObjectClanAttacked(s *Script, e StaticObjectClanAttacked) {
	if h.OnStaticObjectClanAttacked != nil {
		h.OnStaticObjectClanAttacked(s, e)
	}
}

func (h *Hooks) Talk(s *Script, e Talk) string {
	if h.OnTalk != nil {
		return h.OnTalk(s, e)
	}
	return ""
}

func (h *Hooks) Timer(s *Script, e Timer) string {
	if h.OnTimer != nil {
		return h.OnTimer(s, e)
	}
	return ""
}

func (h *Hooks) UseSkillFinished(s *Script, e UseSkillFinished) {
	if h.OnUseSkillFinished != nil {
		h.OnUseSkillFinished(s, e)
	}
}

func (h *Hooks) ZoneEnter(s *Script, e ZoneEnter) {
	if h.OnZoneEnter != nil {
		h.OnZoneEnter(s, e)
	}
}
