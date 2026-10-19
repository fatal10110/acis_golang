package quest

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// subclassProbeID is the NPC id the subclass probe answers the first talk
// of.
const subclassProbeID = 30003

// subclassProbe answers its NPC's first talk with a page naming what the
// scenario's class and subclass lines set up: the class the talker plays,
// its base class, its level and whether a subclass is active, so a
// scenario sees those lines' effect through packets, before and after a
// relog.
func subclassProbe() script.Script {
	return script.Script{
		Bind: script.Bindings{script.EventFirstTalk: {subclassProbeID}},
		Hooks: script.Hooks{OnFirstTalk: func(_ *script.Script, e script.FirstTalk) string {
			p := e.Player
			return fmt.Sprintf("<html><body>class=%d base=%d level=%d subclass=%t</body></html>",
				p.ClassID(), p.BaseClass(), p.Level(), p.SubClassActive())
		}},
	}
}
