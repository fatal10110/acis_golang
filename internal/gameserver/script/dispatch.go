package script

import (
	"runtime/debug"
	"strings"
)

// ResultKind is what a string hook's answer asks the engine to send.
type ResultKind uint8

// The result kinds.
const (
	// ResultNone: the hook answered "", nothing to show.
	ResultNone ResultKind = iota
	// ResultPageFile: Text names a page file of the script.
	ResultPageFile
	// ResultPage: Text is a whole page.
	ResultPage
	// ResultChat: Text is a chat line for the player.
	ResultChat
	// ResultAborted: the hook panicked; nothing is sent, not even what an
	// empty answer would cause.
	ResultAborted
)

// Result is a string hook's answer, classified.
type Result struct {
	Kind ResultKind
	Text string
}

// Classify sorts a string hook's answer: a name ending in .htm or .html is
// a page file, text starting with <html> a page, any other non-empty text a
// chat line, and "" nothing.
func Classify(answer string) Result {
	switch {
	case answer == "":
		return Result{Kind: ResultNone}
	case strings.HasSuffix(answer, ".htm") || strings.HasSuffix(answer, ".html"):
		return Result{Kind: ResultPageFile, Text: answer}
	case strings.HasPrefix(answer, "<html>"):
		return Result{Kind: ResultPage, Text: answer}
	default:
		return Result{Kind: ResultChat, Text: answer}
	}
}

// run calls fn, one invocation of s's hook h, and reports whether it
// returned. A panic is recovered and logged with its stack; nothing is
// disabled, so the next hook and the next invocation run as usual.
func (r *Registry) run(s *Script, h hook, fn func()) (ok bool) {
	defer func() {
		if p := recover(); p != nil {
			r.log.Error().Str("script", s.path).Str("hook", h.String()).Interface("panic", p).Str("stack", string(debug.Stack())).Msg("script: hook panicked")
			ok = false
		}
	}()
	fn()
	return true
}

// Invoke runs fn once with the registered script named name, found as a
// journal row's quest name is, as one invocation of that script: a panic
// is recovered and logged with its stack. It reports false when no script
// has that name or fn panicked.
func (r *Registry) Invoke(name string, fn func(s *Script)) bool {
	s := r.byName[strings.ToLower(name)]
	if s == nil {
		return false
	}
	return r.run(s, hookEvent, func() { fn(s) })
}

// answer calls fn, one invocation of s's string hook h, and classifies its
// answer; a panic gives ResultAborted.
func (r *Registry) answer(s *Script, h hook, fn func() string) Result {
	var text string
	if !r.run(s, h, func() { text = fn() }) {
		return Result{Kind: ResultAborted}
	}
	return Classify(text)
}

// FirstTalk runs the first-talk hook of the NPC with id npcID when exactly
// one script holds its first talk; bound is false otherwise, and the NPC
// answers with its own window.
func (r *Registry) FirstTalk(npcID int32, e FirstTalk) (res Result, bound bool) {
	list := r.scripts(npcID, EventFirstTalk)
	if len(list) != 1 {
		return Result{}, false
	}
	s := list[0]
	return r.answer(s, hookFirstTalk, func() string { return s.Hooks.FirstTalk(s, e) }), true
}

// AbnormalStatusChanged runs the abnormal-status hook of every script that
// sees spells on the NPC with id npcID, in list order: the hook is
// dispatched over the see-spell list, not over its own registrations.
func (r *Registry) AbnormalStatusChanged(npcID int32, e AbnormalStatusChanged) {
	for _, s := range r.scripts(npcID, EventSeeSpell) {
		r.run(s, hookAbnormalStatusChanged, func() { s.Hooks.AbnormalStatusChanged(s, e) })
	}
}
