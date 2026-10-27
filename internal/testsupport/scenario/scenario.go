// Package scenario reads and runs scenario files: a character's play
// through real client packets against a booted server and MariaDB, step by
// step, with the packets, quest journal statements, inventory, quest rows
// and quest states each step must leave. One generic test per suite
// (tests/quest, tests/ai) runs every scenario file of its testdata
// directory; a scenario needs no Go code of its own.
//
// # Format
//
// Line oriented. Blank lines and lines starting with # are comments. The
// header comes first:
//
//	trace <path below the module root>        optional: the reference trace the first steps replay
//	clock <hh:mm[:ss]>                        optional: the driven clock (the one an advance step moves, which the scripts'
//	                                          Script.Now and game tick read) starts at that time of day (UTC) on
//	                                          2026-01-01, so a script reading the minute finds a known one; without it
//	                                          the clock starts at the wall time of the run
//	character <name> <level>                  the character the client plays, with the experience its level starts at
//	class <class id>                          optional: the character's class, and so its race (default 0, a human
//	                                          fighter); every class then has a template with the fixture's
//	                                          stats, so the class's parents load and a script may change it
//	subclass <class id> <level>               optional: the character holds that class, a second or third occupation
//	                                          other than the base class, as its subclass of slot 1, at that level (40
//	                                          up to the highest level), and plays it as it enters the world, as a
//	                                          character that logged out on it: a relog keeps playing it. The
//	                                          character line's level and the class line's class stay the base class's
//	clan <level> [hall <id>]                  optional: the character leads a clan of that level, which owns that clan
//	                                          hall (siegable or sold at auction) when hall says so; a mate line may lead it
//	hall <id> <registering|waiting|siege> [date <unix ms>]
//	                                          optional, one per hall: the server runs the shipped clan halls
//	                                          (data/xml/clanHalls.xml) and the siegable hall's siege stands at that
//	                                          status as it boots (registrations open, closed and waiting for the
//	                                          battle, in progress), with the stored next siege date when date says so
//	                                          (default: the server schedules the next siege itself); a hallstatus step
//	                                          changes it
//	mate <name> <level> [leader] [sponsor|apprentice] [online] [hold <item id> <count>]... [<dx> <dy> <dz>]
//	                                          optional, with a clan line: a second character of the clan, of its own
//	                                          account, that is the clan's leader (the character is then a plain
//	                                          member), the character's academy sponsor or its apprentice, and holds
//	                                          those items. It stays out of the world until an enter step names it,
//	                                          or with online enters where the character first does, then stands at
//	                                          that offset from the character (default 0 0 0); unlike a member it
//	                                          joins no party. Its object id renders as its name
//	siege <castle id> <owner|defender|attacker>
//	                                          optional, with a clan line: the server runs the shipped castles
//	                                          (data/xml/castles.xml) and their sieges, and the clan is registered
//	                                          on that side of the castle's siege before the character enters the
//	                                          world; owner makes it the castle's owner, on its siege's owner side
//	quest <quest> <var>=<value> ...           character_quests rows of the character before it enters the world,
//	                                          one line per quest, as a rows check lists them (a value is one
//	                                          blank-free field; only a rows check quotes a value with blanks)
//	hold <item id> <count>                    optional: the character holds that many of the item before it enters
//	                                          the world, as one stack (that many single items of a template that
//	                                          does not stack); the template must be in the catalog (an item line or
//	                                          the shared one)
//	equip <item id>                           optional: the character wears one of the item, in the paperdoll slot of
//	                                          its body part, as if it logged out wearing it; the template must be in
//	                                          the catalog and equipable
//	standing <n>                              optional: the character's Ketra/Varka alliance standing, 1 to 5 for a
//	                                          Ketra ally of that level, -1 to -5 for a Varka one (default 0, none)
//	pk <n>                                    optional: the character's PK count (default 0)
//	noble                                     optional: the character is a noblesse
//	cabal <dawn|dusk> <avarice|gnosis|strife> optional: the character is signed up for that Seven Signs cabal,
//	                                          choosing that seal, with no stones turned in
//	festival <dawn score> <dusk score>        optional: the Seven Signs festival scores the server restores; with no
//	                                          stones turned in they are the cabals' scores, so the higher cabal is
//	                                          winning and the other losing, neither on a tie (default 0 0)
//	sealowner <avarice|gnosis|strife> <dawn|dusk>
//	                                          optional, one per seal: the cabal that owns the seal in the Seven Signs
//	                                          status the server restores (default none); the festival line makes a cabal
//	                                          win
//	memo <var> <value>                        optional, one per variable: a character_memo row of the character before
//	                                          it enters the world (a value is one blank-free field), which the memo
//	                                          writes of a step then update or delete
//	teleports                                 optional: the shipped gatekeeper teleport list (data/xml/teleports.xml) the
//	                                          server offers, so an NPC line of a shipped gatekeeper id (31078, 30080)
//	                                          opens its real window; prices are as in a run that is no weekend evening
//	                                          and not free of charge
//	random <draw> ...                         optional: the script random draws, in order; each must be inside
//	                                          [0, n) of its call, and every one must be drawn
//	random always <draw>                      optional: every script random draw is that one
//	script <scripts.xml path>                 a script of the suite's catalog; script and stub lines in scripts.xml order
//	stub <scripts.xml path>                   a script not ported yet that a dialog lists: registered from the reference
//	                                          registration manifest (quest id, title, items, bindings) with no hook
//	item <id> ...                             shipped item templates added to the fixture catalog
//	page <path below data/html>               a shipped page the server serves; a path ending in / serves its directory
//	npc <role> <npc id> <type> <dx> <dy> <dz> a fixture NPC template of that instance type, spawned at that offset from the character;
//	    [walks] [maker <territory>]           the templates a script may spawn more NPCs of; walks, the template can move: a
//	    [clan <name>[,<name>...] <range>]     civilian walks, a monster spawns moving and attacking (its spawn point its
//	    [skill <type> <skill id> <level>]...  home); with maker, the NPC is an npcmaker's whose territory is that territory
//	    [ai <name>=<value>[,...]]             line's (see below); clan, the template belongs to those clans, its clan
//	    [params <p1> <p2> <p3>]               calls reaching that far (door_clan: a hit door's call reaches it); skill, the
//	                                          template lists that shipped skill under that type (data/xml/npcs's
//	                                          <skills> type, PASSIVE excepted), which its behaviors cast; ai, the template's
//	                                          AI parameters (<ai> tuning values), which only a scenario with no trace
//	                                          line sets (a traced one plays the shipped template's); params, the three
//	                                          spawn parameters a master gives a private it creates (a script's _param1 to
//	                                          _param3), each a number, the word character (its object id), or the role of
//	                                          an NPC line above (that NPC's object id, as it stands when this one spawns)
//	spoil <role> [other]                      optional: the monster is spoiled, by the character or, with other, by another
//	                                          player, as every fresh monster of that role stands: the one a line spawns and
//	                                          each one a kill step replaces a dead one with
//	skill <skill id> <level>                  optional: the character knows that shipped skill, which a cast step casts
//	roll <role> <draw>                        optional: every combat draw of the NPC is that draw (n-1 for a draw of
//	                                          [0, n) it is not inside): its skill landing rolls, as its hit and
//	                                          critical rolls; with 0 every skill it casts lands (a landing rate is at
//	                                          least 1), with 99 none does (at most 99), and every blow lands (a hit
//	                                          rate is at least 300 of 1000). Role player pins the character's, set
//	                                          each time it enters the world
//	territory <name> <dx1> <dy1> <dx2> <dy2> <dz1> <dz2>
//	                                          optional: an npcmaker territory, the box between those offsets from where the
//	                                          character first enters the world
//	member <name> <level> [dead] [<dx> <dy> <dz>]
//	                                          optional: a second character, of its own account, that enters the world where the
//	                                          character first does, right after it, joins its party, then stands at that
//	                                          offset from the character (default 0 0 0); dead, it is then marked dead where
//	                                          it stands. Its object id renders as its name
//	ground <role> <item id> <count> <dx> <dy> <dz> [cursed]
//	                                          optional: an item lying on the ground at that offset from the character, laid
//	                                          as the character first enters the world, after the NPCs; the template must be
//	                                          in the catalog. Its object id renders as its role. cursed: the item is a cursed
//	                                          weapon of the shipped data (data/xml/cursedWeapons.xml), count 1, whose life
//	                                          starts as a monster kill's drop starts it, then it is laid; the server runs
//	                                          the shipped cursed weapons when a ground line says cursed
//	door <role> <door id> <dx> <dy> <dz>      optional: a fixture door of that id (a 16 by 16 gate of 600 HP, under no
//	                                          siege: a hit takes none of its HP, but calls its door clan) standing at
//	                                          that offset from where the character first enters the world
//	summon <role> <npc id> [servitor <level>] optional: as it first enters the world, the character calls a siege summon
//	                                          of a fixture template of that id, which stays on the active siege
//	                                          battlefield the server then runs around where it enters the world; with
//	                                          servitor, a servitor (a pet) of that level instead, on no battlefield; a
//	                                          scenario with a summon line takes no relog or restart
//
// An npc line takes more options, in any order after the offsets: level <n>, the template's level (default 1);
// aggro <range>, its aggro range (default none: a raid boss sees only as far as that); mp <n>, the current MP
// each of its lives starts with once its created hook ran (default its maximum, 1000); mem <slot>=<value>[,...],
// the script memory slots its life starts with once its created hook ran (i_ai0 to i_ai4, i_quest0 to i_quest4,
// flag, weight (the weight point) and value (the script value) take a number; c_ai0 to c_ai4 and c_quest0 to
// c_quest4 take the character (player), the role of an NPC line above, or - for none); privates
// <npc id>[:<weight>[:<respawn ms>]][,...], the privates its template lists, which a script's CreatePrivates
// raises (each template needs an npc line of its own); master <role>, the NPC is raised as the private of the NPC
// line above, as a master's script raises one.
//
// Each NPC line's NPC runs the created hooks of the scripts bound to it as
// it appears, as the spawn pass runs them, and a civilian casts the shipped
// skills (data/xml/skills) a script or dialog asks of it on a later AI
// tick. An NPC walks when its line says walks: a walking monster also
// attacks, as a live one does. Without walks a monster stands parked,
// neither moving nor attacking, and a civilian's template cannot move. An
// NPC line with maker reads its maker's territory as its spawn territory
// (a script's InSpawnTerritory), as an NPC the population's npcmaker placed
// does; the maker spawns nothing itself.
// Without maker, an NPC's spawn territory is the sphere of its drift range
// (200) around its spawn point.
//
// Then the steps, each a verb line followed by its indented expectations:
//
//	action <role>          an Action packet on the NPC: the first selects it, the next talks
//	bypass <command>       a RequestBypassToServer; {role} stands for the NPC's object id
//	as <mate> <action|bypass|link> ...
//	                       the step is played by that mate, on its client: its packets are the S lines, and what
//	                       the character was sent the `to player` lines
//	enter <mate>           the mate is selected and enters the world (its EnterWorld burst is not compared)
//	event <role> <event id> <arg1> <arg2>
//	                       the NPC receives that script event, as a script's SendScriptEvent sends it, on the NPC's
//	                       own queue; each argument is a number, the word character (its object id) or an NPC
//	                       role (that NPC's object id)
//	hallstatus <id> <registering|waiting|siege>
//	                       the hall line's hall changes its siege status
//	link <file>            a RequestLinkHtml
//	hit <role> <damage> [by <role>] [skill <skill id> <level>]
//	                       the character, or the creature by names (an npc line's NPC, the summon or a member), lands a
//	                       hit of that damage on the NPC or the door, on the attacker's queue; with skill, the
//	                       hit is that shipped skill's (the attacked hook reads its id, level and whether it is
//	                       magic), and the scripts read the skill
//	hp <role|player> <hp>  the NPC's or the character's hit points are set to that many (clamped to its maximum;
//	                       a character at 0 is not dead: the die step kills it, and an NPC at 0 stays alive)
//	mp <role> <mp>         the hostile NPC's current MP is set to that many (clamped to its maximum)
//	want <role> <creature> <weight> [hold]
//	                       the hostile NPC is given an attack desire on the creature of that weight, as a
//	                       script's AddAttackDesire does (with hold, AddAttackDesireHold: it holds its ground
//	                       instead of walking to its target); a draw-pinned swing after it fights the creature
//	die [<role>]           the character dies, killed by that NPC (none by default), on its own queue
//	top <role> <creature|->
//	                       the NPC's top desire target is set to the character (player), an npc line's NPC, the
//	                       summon or a member, or to nothing, as a script's SetTopDesireTarget does
//	land <target> <skill id> <level> [by <creature>]
//	                       the effects of that shipped skill land on the target (player, an npc line's NPC or the
//	                       summon) from the creature by names (default: the target itself), on the target's
//	                       queue, outside any cast, with the landing rolls of a cast's effects (roll pins them);
//	                       time passes until the target holds one. A stun, a mute, a root or a hold lands so
//	                       on an NPC
//	stance <role> <on|off> the hostile NPC's attack stance is set or ended (the stance clock never ends one in a run)
//	swing <role>           the AI ticks and the position ticks run, time passing, until the hostile NPC's swing
//	                       is in flight (it attacks now), as a script's AttackingNow reads it
//	kill <role> [xN]       the character lands a lethal hit on the NPC, N times; a dead NPC is replaced by a fresh one first
//	advance <duration>     time passes, as time.ParseDuration reads it
//	approach <role> [<distance>]
//	                       the character steps to that distance (default 100, inside its see range) in front
//	                       of the NPC, and one AI tick runs, whose see-creature scan notices it
//	away                   the character steps 1500 behind where it first entered the world, out of the sight
//	                       of every NPC standing at an x offset of -1000 or more, and one AI tick runs, whose
//	                       see-creature scans forget it: its next approach is seen again
//	cast <role> <skill id> the character selects the NPC (unless it has it selected; the selection's packets
//	                       are not compared) and casts that skill of a skill line on it, forced; time passes
//	                       until it neither walks into cast range nor casts
//	jump <role> <dx> <dy> <dz>
//	                       the NPC jumps to that offset from where the character first entered the world,
//	                       its spawn point kept
//	tick [xN]              one AI tick runs (N ticks with xN), each let settle: what the NPCs' AI does
//	                       on its own, as casting what a script asked of them
//	pickup <item id>       an Action packet on the one ground item of that template, then time passes
//	                       until the item has left the ground
//	look <role> <radius> <quantity> [<item id> ...]
//	                       the NPC looks for ground items within radius (of those templates, any without),
//	                       on its own queue: its behavior's see-item hook runs, told at most quantity
//	walk <role>            position ticks run until the NPC's walk ends (a pick-up desire's walk to its item);
//	                       the run fails if the NPC is not walking as the step starts
//	mark <id>              a RequestTutorialQuestionMark: the character clicks that tutorial question mark
//	relog                  back to character select and into the world again
//	restart                a RequestRestart: back to character select
//	enter                  the character is selected and enters the world (the EnterWorld burst)
//	create                 the first step only: a RequestCharacterCreate of a human fighter named as the
//	                       character line says, instead of a seeded character; it gets the shipped human
//	                       fighter's starter items (data/xml/classes), whose templates join the catalog;
//	                       the character line's level must be 1, and the class, clan, quest, hold, equip,
//	                       standing, pk and cabal lines are left out (the creation writes its own rows)
//
//	  S <packet>           the packets of the step, in order, as scriptcontract.Packet renders them;
//	                       "page=<path>" ends an NpcHtmlMessage whose text is that page, %objectId% filled in
//	  to <name> <packet>   a packet another online character (player, or a mate) was sent in the step, rendered as an
//	                       S line is; a character with no to line was sent none of the rendered packets
//	  Q <statement>        the character_quests statements of the step, in order: kind quest | params
//	  M set <var> | <value>  a character_memo write of the step: the player memo var set to value;
//	  M unset <var>          or deleted. The M lines are the step's memo writes, in order
//	  queue <own|other[+n]> ...  a traced step: the queue whose hook teleported the character, or made it cast,
//	                         in each task of the trace's step that does, in order; other+n also names the n
//	                         lines after each of the task's casts that go out with it (see the trace header below)
//	  live <npc id> <n>    how many NPCs of that template stand alive in the world after the step
//	  spawned <npc id> <x>,<y>,<z> ... | spawned <npc id> -
//	                       where the NPCs of that template stand alive that no npc line placed (a script's
//	                       spawns), as world coordinates, in any order; - for none
//	  items <id>=<n> ...   the character's count of each item after the step (items <mate> <id>=<n> ... a mate's)
//	  rows <quest> <var>=<value> ... | -    the quest's character_quests rows after the step, all of them (a value
//	                                        with blanks is quoted: quiz="20 21")
//	  state <quest> <status> | -            the quest's state in memory after the step
//	  hate <role> <weight> [<creature>]     the NPC's hate of the character, or of that creature (an npc line's
//	                                        NPC or the summon), after the step
//	  desires <role> <desire> ... | -       the NPC's queued desires after the step, all of them, in any order,
//	                                        each <kind>(<args>)=<weight>: attack(<creature>), follow(<creature>),
//	                                        flee(<creature>,<distance>), cast(<creature>,<skill id>[,move]) (move: it
//	                                        walks to its target), move_to(<dx>,<dy>,<dz>) (an offset from where the
//	                                        character first entered the world), nothing(<timer>), any other kind ().
//	                                        A desire may be listed in full to pin more: cast(<creature>,<skill id>:<level>[,move])
//	                                        pins the skill's level, attack(<creature>,hold) that the NPC holds its ground
//	                                        (AddAttackDesireHold); a desire listed in the short form is compared without the
//	                                        level or the marker
//	  at <role> <dx> <dy> <dz>              where the NPC, or the character (player), stands after the step, as an
//	                                        offset from where the character first entered the world
//	  mem <role> <slot>=<value> ...         the NPC's script memory slots after the step (see the npc line's mem)
//	  hp <role|player> <hp>                 the hostile NPC's or the character's hit points after the step
//	  mp <role> <mp>                        the hostile NPC's current MP after the step
//	  reach <role> <yes|no>                 whether the hostile NPC may attack its top desire target now (its canAttack:
//	                                        a rooted or immobile NPC needs it within reach; the run fails if it has no top
//	                                        desire target)
//	  shows <role> rhand=<id> | lhand=<id> | chest=<id> | glow=<n> ...
//	                                        the appearance an NpcInfo of the NPC would show, as it stands after the
//	                                        step: the item ids in its hands and chest, and the weapon's glow (its
//	                                        enchant effect); only the fields listed are compared
//	  dead <role|player> <yes|no>           whether the hostile NPC or the character is dead (a deleted NPC is)
//	  inworld <role> <yes|no>               whether the NPC is still in the world: a killed one stays as a
//	                                        corpse, a deleted one (a chest whose key broke) is gone
//	  privates <role> <npc id>=<n> ... | -  the live privates of the NPC, by template (the privates of a master
//	                                        line included), all of them; <npc id>=<n>:<p1>,<p2>,<p3> counts
//	                                        that template's privates by the three spawn parameters a script
//	                                        raised them with (a number, or the role of the creature whose
//	                                        object id it holds)
//	  master <role> <role|->                the NPC's master, or - for none
//	  top <role> <creature|->               the hostile NPC's top desire target
//	  effect <role|player> <skill id> <held|none>
//	                                        whether the NPC or the character holds an effect of that skill
//	  stance <role> <on|off>                whether the hostile NPC holds an attack stance
//	  random <draw> ... | random always <draw>
//	                                        not a check but the step's own script draws, in place of the random
//	                                        line's for this step alone (every one must be drawn); the random line's
//	                                        resume at the next step, so a scenario draws differently per step
//
// A step other than create and enter acts in the world: a scenario starting
// with create, or a restart, is followed by an enter before any of them. The
// NPC lines' NPCs are spawned when the character first enters the world.
// The items, state, hate, desires and at checks read the world, so a create
// or restart step takes rows checks only. A weight renders in the fewest
// digits that read back as it.
//
// The S, Q and M lists are exact: a step without S lines must send none of
// the rendered packets (relog excepted: its enter burst is not compared),
// one without Q lines must write no journal statement, and one without M
// lines no memo. The rendered packets
// are the dialog's, the quest's, the tutorial's and the item messages:
// ActionFailed, NpcHtmlMessage, SystemMessage, PlaySound, QuestList,
// ExShowQuestMark, TutorialShowHtml, TutorialShowQuestionMark,
// TutorialEnableClientEvent, TutorialCloseHtml, RadarControl, DropItem,
// MagicSkillUse and NpcSay, the TeleportToLocation of the character's own
// teleport ("TeleportToLocation obj=player loc=<x>,<y>,<z> fast=<0|1>": where
// it lands), and the DeleteObject of a ground line's item leaving the world
// ("DeleteObject obj=<role>"). "page=<path>" also ends a TutorialShowHtml
// line.
//
// Without a random line the script draws are random. A draw outside
// [0, n), or past the listed ones, fails the step that drew it.
//
// A trace header ties the scenario to the reference server: before the run
// the trace's client packets, kills, hits, advances, approaches, ticks and
// pickups must be the scenario's steps (a kill step of N kills is N kills of
// the trace, a relog the trace's restart, character select and enter world),
// and their rendered packets, journal statements and memo writes must be
// those steps' S, Q and M lines (a relog's packets excepted, as in the run);
// a read, and a statement on another table than character_quests and
// character_memo, is not compared, an unknown write on one of them fails.
// A script's teleport of the character, or its cast with the character as
// caster, from a hook on another queue than the character's goes out once
// the hook's task is done, after the packets the rest of the hook sends
// (script engine plan, section 9, item 4), where the reference sends it
// inside the call; from a hook on the character's own queue (its talk, a
// timer whose home queue is the character's, an After closure, ...) it runs
// inside the call as the reference does. For a task of another queue the
// comparison moves each of the trace's TeleportToLocation lines of the
// character, with the ActionFailed lines right before it (its aborts'), and
// each MagicSkillUse line whose caster is the character, with its
// ActionFailed lines and TeleportToLocation of the character right after it
// (its skill's teleport), to the end of the packets that task sent. The trace
// does not say which queue a task's hook ran on: a client packet's own run
// is the character's queue, every other task (a tick's, a kill's, an
// advance's, ...) another, unless the step's queue line names the queue of
// each of its tasks that teleport the character or make it cast, in the
// trace's order. The cast's own effect packets (a system message, an
// effect's messages) go out with the cast in the Go run, which the trace
// cannot tell from the hook's own: other+n in the queue line moves the n
// lines right after each cast the task makes with the character as caster
// along with it, in place of its ActionFailed lines and teleport, which then
// count among the n.
// The script draws the trace pins (needing a random line) must be those the
// run's same steps draw: the same bounds, in order. A trace under the
// oracle's scenarios directory is the reference probe's play of this
// scenario itself (the oracle README). The trace format has no member,
// ground or look, so a traced scenario takes no member or ground line, and
// its one client takes no mate line but an offline mate that leads the clan
// (and no as or enter step of a mate); nor
// does the reference probe play a skill, door or summon line, an npc
// line's clan or skill, a hit by another creature, an approach to a
// distance, a cast, a jump or an away, a clock line, an npc line's ai,
// params, level, aggro, mp, mem, privates or master or an event, hp, mp, want, die, top,
// land, stance or swing step, a skill hit, or a step's own random line, so a
// traced scenario takes none.
//
// A step compares what a script does in the world: besides the dialog, the
// quest and the item packets, the camera shots (SpecialCamera) and the
// social animations (SocialAction) of the character, and of an NPC outside
// a step of the client talking to it (an NPC answers a talk with a random
// animation of its own), and the NPCs a script spawns and takes away. A
// script-spawned NPC (one no npc line placed) renders as NpcInfo and
// DeleteObject under the role of the one npc line of its template, and not
// at all when no line (or several) has that template; an NpcInfo repeating
// the one before it is compared once (the reference sends a spawned NPC's
// twice), a run of DeleteObject lines in sorted order (the order a teleport
// empties the character's view in is not a script's), and the DeleteObject
// of an NPC a Die packet showed dead (its corpse's decay) not at all. The
// character's teleports also delete the NPCs it leaves behind. A trace that
// lists no npc line's object (the hand-made trace_*.golden files) shows no
// NpcInfo or DeleteObject.
//
// Every NPC and the character hold an attack stance while they fight (the
// behaviors read it), which nothing ends during a run. A fixture monster
// has 440 HP once its stats apply: a hit of 22 is a twentieth of them.
package scenario

import (
	"bufio"
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// Scenario is one parsed scenario file.
type Scenario struct {
	// Name is the file name, for messages.
	Name string
	// Trace is the reference trace path below the module root, empty for
	// none.
	Trace string
	// Clock is the clock line's time of day, the driven clock's start;
	// HasClock tells a line was given.
	Clock     time.Duration
	HasClock  bool
	Character string
	Level     int
	// Class is the character's class id: its base class.
	Class int
	// Subclass is the subclass line's class, played from the start; 0 for
	// none. SubclassLevel is its level.
	Subclass      int
	SubclassLevel int
	// Clan is the level of the clan the character leads, -1 for none.
	Clan int
	// ClanHall is the id of the clan hall the clan owns, 0 for none. Halls
	// are the hall lines and Mates the mate lines.
	ClanHall int
	Halls    []Hall
	Mates    []Mate
	// Siege is the siege line, nil for none.
	Siege *Siege
	// Quests are the character_quests rows seeded before the character
	// enters the world.
	Quests []QuestRows
	// Holds are the hold lines: what the character holds before it enters
	// the world.
	Holds []Hold
	// Equips are the equip lines' items, worn before the character enters
	// the world.
	Equips []int32
	// Standing is the Ketra/Varka alliance standing: positive Ketra,
	// negative Varka, 0 none.
	Standing int
	// PK is the character's PK count.
	PK int
	// Cabal is the cabal the character is signed up for choosing Seal,
	// NoCabal for none.
	Cabal sevensigns.Cabal
	Seal  sevensigns.Seal
	// Festival is the festival line's Dawn and Dusk scores, nil for none.
	Festival *[2]int
	// SealOwners are the sealowner lines' owners, in sevensigns.Seals
	// order; NoCabal for a seal no line names.
	SealOwners [3]sevensigns.Cabal
	// Memos are the memo lines' character_memo rows, seeded before the
	// character enters the world.
	Memos []Memo
	// Noble tells the noble line is there: the character is a noblesse.
	Noble bool
	// Teleports tells the teleports line is there: the shipped teleport
	// list is offered.
	Teleports bool
	// Spoils are the spoil lines.
	Spoils []Spoil
	// Random is the script random source, nil for random draws.
	Random  *Random
	Scripts []ScriptLine
	Items   []int32
	Pages   []string
	NPCs    []NPC
	// Rolls are the roll lines' draws by role.
	Rolls map[string]int
	// Territories are the territory lines, Members the member lines and
	// Grounds the ground lines.
	Territories []Territory
	Members     []Member
	Grounds     []Ground
	// Skills are the skill lines: the shipped skills the character knows.
	// Doors are the door lines, Summon the summon line (nil for none).
	Skills []modelskill.Ref
	Doors  []Door
	Summon *Summon
	Steps  []Step
}

// Siege is the siege line: the clan's side of a castle's siege.
type Siege struct {
	Castle int
	Side   string
}

// siegeSides are the sides a siege line names, as the siege_clans rows
// write them; owner is the castle's owner instead.
var siegeSides = []string{"owner", "defender", "attacker"}

// QuestRows is one quest line: a quest's rows as var=value.
type QuestRows struct {
	Quest string
	Vars  [][2]string
}

// Hold is one hold line: Count of the item template Item.
type Hold struct {
	Item  int32
	Count int32
}

// Memo is one memo line: a character_memo row.
type Memo struct {
	Var   string
	Value string
}

// Spoil is one spoil line: the monster of Role stands spoiled, by another
// player when Other, else by the character.
type Spoil struct {
	Role  string
	Other bool
}

// parseRandom reads a random line's arguments: <draw> ... or always <draw>.
func parseRandom(args []string) (*Random, error) {
	rd := &Random{}
	if len(args) > 0 && args[0] == "always" {
		if len(args) != 2 {
			return nil, fmt.Errorf("random always <draw>")
		}
		rd.Always, args = true, args[1:]
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("random <draw> ... | random always <draw>")
	}
	for _, a := range args {
		v, err := strconv.Atoi(a)
		if err != nil || v < 0 {
			return nil, fmt.Errorf("draw %q is not a number of 0 or more", a)
		}
		rd.Draws = append(rd.Draws, v)
	}
	return rd, nil
}

// Random is the random line: the draws in order, or one draw always.
type Random struct {
	Draws  []int
	Always bool
}

// ScriptLine is one script or stub line.
type ScriptLine struct {
	Path string
	Stub bool
}

// NPC is one NPC the scenario spawns.
type NPC struct {
	Role       string
	ID         int32
	Type       string
	DX, DY, DZ int
	// Walks gives the template canMove: a civilian walks, a monster
	// spawns moving.
	Walks bool
	// Maker is the territory line of the NPC's npcmaker, empty for none.
	Maker string
	// Clans are the template's clans, nil for none, and ClanRange how far
	// its clan calls reach.
	Clans     []string
	ClanRange int
	// Skills are the template's skills by type, shipped skills it casts;
	// nil for none.
	Skills map[npc.SkillType]modelskill.Ref
	// AI are the template's AI parameters, nil for none.
	AI npc.AIParams
	// Params are the three spawn parameters, each "" (0), a number, a role
	// or character.
	Params [3]string
	// Level is the template's level, 0 for the fixture's 1; Aggro its aggro
	// range, 0 for none.
	Level, Aggro int
	// MP is the NPC's current MP each life starts with, when HasMP.
	MP    int
	HasMP bool
	// Mem are the script memory slots each life starts with, nil for none.
	Mem []MemSet
	// Privates are the template's privates, nil for none.
	Privates []npc.PrivateEntry
	// Master is the role of the NPC line above this NPC is raised as the
	// private of, "" for none.
	Master string
}

// Step is one client action and what it must leave.
type Step struct {
	// Line is the verb line's number in the file.
	Line int
	Verb string
	Args []string
	// Packets, Statements and Memos are the S, Q and M lines without their
	// prefix.
	Packets    []string
	Statements []string
	Memos      []string
	// As is the mate that plays the step, empty for the character. To are
	// the to lines' packets by the name of the character they were sent to.
	As string
	To map[string][]string
	// Lives are the live lines as "<npc id>=<count>": how many NPCs of that
	// template stand alive in the world when the step is over.
	Lives []string
	// Spawns are the spawned lines as "<npc id> <x>,<y>,<z> ..." (the places
	// sorted, "-" for none): where the NPCs of that template stand alive that
	// no npc line placed, a script's spawns.
	Spawns []string
	// Queues is the queue line: "own", "other" or "other+n" (the n lines
	// after each cast that go out with it) for each task of the traced step
	// that teleports the character or makes it cast, in order; nil without
	// one.
	Queues []string
	// Random are the script draws this step draws instead of the random
	// line's, nil for the random line's.
	Random *Random
	Checks []Check
}

// String is the step as the file writes it.
func (s Step) String() string { return strings.Join(append([]string{s.Verb}, s.Args...), " ") }

// Check is one items, rows or state line.
type Check struct {
	Line int
	Kind string
	Args []string
}

// maxClanLevel is the highest clan level.
const maxClanLevel = 8

// minSubclassLevel is the lowest level a subclass has.
const minSubclassLevel = 40

// maxStanding is the highest Ketra/Varka alliance level.
const maxStanding = 5

// argCount is, per verb, the arguments it takes: min and max.
var argCount = map[string][2]int{
	"action":     {1, 1},
	"bypass":     {1, -1},
	"link":       {1, 1},
	"hit":        {2, 7},
	"kill":       {1, 2},
	"advance":    {1, 1},
	"approach":   {1, 2},
	"cast":       {2, 2},
	"jump":       {4, 4},
	"away":       {0, 0},
	"pickup":     {1, 1},
	"look":       {3, -1},
	"walk":       {1, 1},
	"mark":       {1, 1},
	"tick":       {0, 1},
	"relog":      {0, 0},
	"restart":    {0, 0},
	"enter":      {0, 1},
	"hallstatus": {2, 2},
	"event":      {4, 4},
	"create":     {0, 0},
	"hp":         {2, 2},
	"mp":         {2, 2},
	"want":       {3, 4},
	"die":        {0, 1},
	"top":        {2, 2},
	"land":       {3, 5},
	"stance":     {2, 2},
	"swing":      {1, 1},
}

// Parse reads a scenario file.
func Parse(name string, data []byte) (*Scenario, error) {
	sc := &Scenario{Name: name, Clan: -1}
	roles := map[string]bool{}
	sb := bufio.NewScanner(bytes.NewReader(data))
	sb.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sb.Scan(); n++ {
		raw := sb.Text()
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%s:%d: %s", name, n, fmt.Sprintf(format, args...))
		}
		if raw[0] == ' ' || raw[0] == '\t' {
			if len(sc.Steps) == 0 {
				return nil, fail("expectation %q before any step", line)
			}
			if err := sc.Steps[len(sc.Steps)-1].expect(n, line); err != nil {
				return nil, fail("%v", err)
			}
			continue
		}
		fields := strings.Fields(line)
		verb, args := fields[0], fields[1:]
		as := ""
		if verb == "as" {
			// as <mate> <verb> ...: the step the verb names, played by the mate.
			if len(args) < 2 || !slices.Contains([]string{"action", "bypass", "link"}, args[1]) {
				return nil, fail("as <mate> <action|bypass|link> ...")
			}
			as = args[0]
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "as")), as))
			fields = strings.Fields(line)
			verb, args = fields[0], fields[1:]
		}
		if _, isStep := argCount[verb]; isStep || as != "" || len(sc.Steps) > 0 {
			c, ok := argCount[verb]
			if !ok {
				return nil, fail("unknown step %q", verb)
			}
			if len(args) < c[0] || (c[1] >= 0 && len(args) > c[1]) {
				return nil, fail("%s takes %d to %d arguments, got %d", verb, c[0], c[1], len(args))
			}
			if err := sc.checkStep(verb, args, roles); err != nil {
				return nil, fail("%v", err)
			}
			if verb == "bypass" {
				// The command keeps its own spacing.
				args = []string{strings.TrimSpace(strings.TrimPrefix(line, verb))}
			}
			sc.Steps = append(sc.Steps, Step{Line: n, Verb: verb, Args: args, As: as})
			continue
		}
		if err := sc.header(verb, args, roles); err != nil {
			return nil, fail("%v", err)
		}
	}
	if err := sb.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if sc.Character == "" {
		return nil, fmt.Errorf("%s: no character line", name)
	}
	if len(sc.Steps) == 0 {
		return nil, fmt.Errorf("%s: no step", name)
	}
	if sc.Siege != nil && sc.Clan < 0 {
		return nil, fmt.Errorf("%s: a siege line needs a clan line", name)
	}
	if sc.Subclass != 0 && sc.Subclass == sc.Class {
		return nil, fmt.Errorf("%s: the subclass is the base class %d", name, sc.Class)
	}
	if err := sc.checkHalls(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if sc.Trace != "" && (len(sc.Members) > 0 || len(sc.Grounds) > 0) {
		return nil, fmt.Errorf("%s: a traced scenario takes no member or ground line", name)
	}
	if what := sc.tracedFight(); sc.Trace != "" && what != "" {
		return nil, fmt.Errorf("%s: a traced scenario takes no %s", name, what)
	}
	if what := sc.tracedState(); sc.Trace != "" && what != "" {
		return nil, fmt.Errorf("%s: a traced scenario takes no %s", name, what)
	}
	if err := sc.checkFightChecks(roles); err != nil {
		return nil, fmt.Errorf("%s:%w", name, err)
	}
	if err := sc.checkStateRoles(roles); err != nil {
		return nil, fmt.Errorf("%s:%w", name, err)
	}
	if err := sc.checkFlow(); err != nil {
		return nil, fmt.Errorf("%s:%w", name, err)
	}
	return sc, nil
}

// Creates reports whether the scenario creates its character through the
// client: its first step is create.
func (sc *Scenario) Creates() bool { return sc.Steps[0].Verb == "create" }

// LaysCursed reports whether a ground line lays a cursed weapon.
func (sc *Scenario) LaysCursed() bool {
	return slices.ContainsFunc(sc.Grounds, func(g Ground) bool { return g.Cursed })
}

// checkFlow requires each step to find the character where it acts: create
// first and only on a character no precondition line seeds, enter at the
// character select, every other step in the world.
func (sc *Scenario) checkFlow() error {
	inWorld := true
	for i, st := range sc.Steps {
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%d: %s", st.Line, fmt.Sprintf(format, args...))
		}
		switch st.Verb {
		case "create":
			if i != 0 {
				return fail("create is the first step only")
			}
			switch {
			case sc.Level != 1:
				return fail("a created character starts at level 1, not %d", sc.Level)
			case sc.Class != 0 || sc.Subclass != 0 || sc.Clan >= 0 || len(sc.Quests) > 0 || len(sc.Memos) > 0:
				return fail("a created character takes no class, subclass, clan, quest or memo line")
			case len(sc.Holds) > 0 || len(sc.Equips) > 0 || sc.Standing != 0 || sc.PK != 0 || sc.Noble || sc.Cabal != sevensigns.NoCabal:
				return fail("a created character takes no hold, equip, standing, pk, noble or cabal line")
			case len(sc.Members) > 0:
				return fail("a created character takes no member line")
			case len(sc.Mates) > 0 || len(sc.Halls) > 0:
				return fail("a created character takes no mate or hall line")
			case len(sc.Skills) > 0 || len(sc.Doors) > 0 || sc.Summon != nil:
				return fail("a created character takes no skill, door or summon line")
			}
			inWorld = false
		case "enter":
			if len(st.Args) == 1 {
				// A mate enters the world beside the character in it.
				if !inWorld {
					return fail("enter %s needs the character in the world", st.Args[0])
				}
				break
			}
			if inWorld {
				return fail("enter needs the character select: after create or restart")
			}
			inWorld = true
		default:
			if !inWorld {
				return fail("%s needs the character in the world: enter first", st.Verb)
			}
			if (st.Verb == "restart" || st.Verb == "relog") && len(sc.Members) > 0 {
				// The character leaving the world breaks its party up.
				return fail("a scenario with member lines takes no %s", st.Verb)
			}
			if (st.Verb == "restart" || st.Verb == "relog") && sc.Summon != nil {
				// The character leaving the world takes its summon with it.
				return fail("a scenario with a summon line takes no %s", st.Verb)
			}
			if st.Verb == "restart" {
				inWorld = false
			}
		}
		for _, ck := range st.Checks {
			if !inWorld && ck.Kind != "rows" {
				return fmt.Errorf("%d: a %s check reads the character in the world: after %s, only rows", ck.Line, ck.Kind, st.Verb)
			}
		}
	}
	return nil
}

func (sc *Scenario) header(key string, args []string, roles map[string]bool) error {
	switch key {
	case "trace":
		if len(args) != 1 || sc.Trace != "" {
			return fmt.Errorf("one trace line with one path")
		}
		sc.Trace = args[0]
	case "character":
		if len(args) != 2 || sc.Character != "" {
			return fmt.Errorf("one character line: character <name> <level>")
		}
		level, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return fmt.Errorf("level %q: %w", args[1], err)
		}
		sc.Character, sc.Level = args[0], int(level)
	case "class":
		if len(args) != 1 || sc.Class != 0 {
			return fmt.Errorf("one class line: class <class id>")
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("class %q: %w", args[0], err)
		}
		if id == 0 {
			return fmt.Errorf("class 0 is the default: leave the line out")
		}
		if _, ok := player.ClassRace(id); !ok {
			return fmt.Errorf("class %d has no race", id)
		}
		sc.Class = id
	case "subclass":
		if len(args) != 2 || sc.Subclass != 0 {
			return fmt.Errorf("one subclass line: subclass <class id> <level>")
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("subclass %q: %w", args[0], err)
		}
		if lv, ok := player.ClassLevel(id); !ok || lv < 2 {
			// A subclass is taken as a second occupation, and the
			// third class quests move it on.
			return fmt.Errorf("subclass %d is not a second or third occupation", id)
		}
		level, err := strconv.Atoi(args[1])
		if err != nil || level < minSubclassLevel {
			return fmt.Errorf("subclass level %q is not %d or more", args[1], minSubclassLevel)
		}
		sc.Subclass, sc.SubclassLevel = id, level
	case "clan":
		return sc.clanLine(args)
	case "hall":
		return sc.hallLine(args)
	case "mate":
		return sc.mateLine(args, roles)
	case "siege":
		if len(args) != 2 || sc.Siege != nil {
			return fmt.Errorf("one siege line: siege <castle id> <owner|defender|attacker>")
		}
		id, err := strconv.Atoi(args[0])
		if err != nil || id < 1 {
			return fmt.Errorf("castle id %q is not a number of 1 or more", args[0])
		}
		if !slices.Contains(siegeSides, args[1]) {
			return fmt.Errorf("siege side %q is not owner, defender or attacker", args[1])
		}
		sc.Siege = &Siege{Castle: id, Side: args[1]}
	case "quest":
		if len(args) < 2 {
			return fmt.Errorf("quest <quest> <var>=<value>, one or more")
		}
		q := QuestRows{Quest: args[0]}
		for _, l := range sc.Quests {
			if l.Quest == q.Quest {
				return fmt.Errorf("quest %s listed twice", q.Quest)
			}
		}
		for _, a := range args[1:] {
			k, v, ok := strings.Cut(a, "=")
			if !ok || k == "" {
				return fmt.Errorf("row %q is not <var>=<value>", a)
			}
			q.Vars = append(q.Vars, [2]string{k, v})
		}
		sc.Quests = append(sc.Quests, q)
	case "clock":
		if len(args) != 1 || sc.HasClock {
			return fmt.Errorf("one clock line: clock <hh:mm[:ss]>")
		}
		d, err := parseClock(args[0])
		if err != nil {
			return err
		}
		sc.Clock, sc.HasClock = d, true
	case "random":
		if sc.Random != nil {
			return fmt.Errorf("one random line")
		}
		rd, err := parseRandom(args)
		if err != nil {
			return err
		}
		sc.Random = rd
	case "script", "stub":
		if len(args) != 1 {
			return fmt.Errorf("%s <path>", key)
		}
		for _, l := range sc.Scripts {
			if l.Path == args[0] {
				return fmt.Errorf("script %s listed twice", args[0])
			}
		}
		sc.Scripts = append(sc.Scripts, ScriptLine{Path: args[0], Stub: key == "stub"})
	case "hold", "equip", "standing", "pk", "noble", "cabal", "festival", "sealowner", "memo":
		return sc.holdings(key, args)
	case "teleports":
		if len(args) != 0 || sc.Teleports {
			return fmt.Errorf("one teleports line, with no argument")
		}
		sc.Teleports = true
	case "spoil":
		return sc.spoil(args, roles)
	case "item":
		if len(args) == 0 {
			return fmt.Errorf("item takes one or more ids")
		}
		for _, a := range args {
			id, err := strconv.ParseInt(a, 10, 32)
			if err != nil {
				return fmt.Errorf("item id %q: %w", a, err)
			}
			sc.Items = append(sc.Items, int32(id))
		}
	case "page":
		if len(args) != 1 {
			return fmt.Errorf("page <path>")
		}
		sc.Pages = append(sc.Pages, args[0])
	case "npc":
		if len(args) < 6 {
			return fmt.Errorf("npc <role> <npc id> <type> <dx> <dy> <dz> [options]")
		}
		opts, err := npcOptions(args[6:], sc.Territories)
		if err != nil {
			return err
		}
		id, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return fmt.Errorf("npc number %q: %w", args[1], err)
		}
		var offsets [3]int
		for i, a := range args[3:6] {
			v, err := strconv.ParseInt(a, 10, 32)
			if err != nil {
				return fmt.Errorf("npc number %q: %w", a, err)
			}
			offsets[i] = int(v)
		}
		role := args[0]
		if sc.named(role, roles) {
			return fmt.Errorf("role %q named twice", role)
		}
		for _, p := range opts.Params {
			if err := sc.checkObjectRef(p); err != nil {
				return fmt.Errorf("npc %s params: %w", role, err)
			}
		}
		roles[role] = true
		if err := sc.checkNPCState(opts, roles); err != nil {
			return err
		}
		opts.Role, opts.ID, opts.Type = role, int32(id), args[2]
		opts.DX, opts.DY, opts.DZ = offsets[0], offsets[1], offsets[2]
		sc.NPCs = append(sc.NPCs, opts)
	case "territory", "member", "ground", "roll":
		return sc.world(key, args, roles)
	case "skill", "door", "summon":
		return sc.fight(key, args, roles)
	default:
		return fmt.Errorf("unknown header line %q", key)
	}
	return nil
}

// holdings reads the lines of what the character holds and stands for:
// hold, equip, standing, pk, cabal and festival.
func (sc *Scenario) holdings(key string, args []string) error {
	switch key {
	case "hold":
		if len(args) != 2 {
			return fmt.Errorf("hold <item id> <count>")
		}
		id, err := strconv.ParseInt(args[0], 10, 32)
		if err != nil {
			return fmt.Errorf("hold item %q: %w", args[0], err)
		}
		count, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil || count < 1 {
			return fmt.Errorf("hold count %q is not a number of 1 or more", args[1])
		}
		for _, h := range sc.Holds {
			if h.Item == int32(id) {
				return fmt.Errorf("item %d held twice", id)
			}
		}
		sc.Holds = append(sc.Holds, Hold{Item: int32(id), Count: int32(count)})
	case "equip":
		if len(args) != 1 {
			return fmt.Errorf("equip <item id>")
		}
		id, err := strconv.ParseInt(args[0], 10, 32)
		if err != nil {
			return fmt.Errorf("equip item %q: %w", args[0], err)
		}
		if slices.Contains(sc.Equips, int32(id)) {
			return fmt.Errorf("item %d worn twice", id)
		}
		sc.Equips = append(sc.Equips, int32(id))
	case "standing":
		if len(args) != 1 || sc.Standing != 0 {
			return fmt.Errorf("one standing line: standing <n>")
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n == 0 || n < -maxStanding || n > maxStanding {
			return fmt.Errorf("standing %q is not 1 to %d (Ketra) or -1 to -%d (Varka)", args[0], maxStanding, maxStanding)
		}
		sc.Standing = n
	case "pk":
		if len(args) != 1 || sc.PK != 0 {
			return fmt.Errorf("one pk line: pk <n>")
		}
		n, err := strconv.ParseInt(args[0], 10, 32)
		if err != nil || n < 1 {
			return fmt.Errorf("pk count %q is not a number of 1 or more", args[0])
		}
		sc.PK = int(n)
	case "noble":
		if len(args) != 0 || sc.Noble {
			return fmt.Errorf("one noble line, with no argument")
		}
		sc.Noble = true
	case "cabal":
		if len(args) != 2 || sc.Cabal != sevensigns.NoCabal {
			return fmt.Errorf("one cabal line: cabal <dawn|dusk> <avarice|gnosis|strife>")
		}
		cabal, err := sevensigns.ParseCabal(strings.ToUpper(args[0]))
		if err != nil || cabal == sevensigns.NoCabal {
			return fmt.Errorf("cabal %q is not dawn or dusk", args[0])
		}
		seal, err := sevensigns.ParseSeal(strings.ToUpper(args[1]))
		if err != nil || seal == sevensigns.NoSeal {
			return fmt.Errorf("seal %q is not avarice, gnosis or strife", args[1])
		}
		sc.Cabal, sc.Seal = cabal, seal
	case "festival":
		if len(args) != 2 || sc.Festival != nil {
			return fmt.Errorf("one festival line: festival <dawn score> <dusk score>")
		}
		var scores [2]int
		for i, a := range args {
			n, err := strconv.ParseInt(a, 10, 32)
			if err != nil || n < 0 {
				return fmt.Errorf("festival score %q is not a number of 0 or more", a)
			}
			scores[i] = int(n)
		}
		sc.Festival = &scores
	case "sealowner":
		if len(args) != 2 {
			return fmt.Errorf("sealowner <avarice|gnosis|strife> <dawn|dusk>")
		}
		seal, err := sevensigns.ParseSeal(strings.ToUpper(args[0]))
		if err != nil || seal == sevensigns.NoSeal {
			return fmt.Errorf("seal %q is not avarice, gnosis or strife", args[0])
		}
		owner, err := sevensigns.ParseCabal(strings.ToUpper(args[1]))
		if err != nil || owner == sevensigns.NoCabal {
			return fmt.Errorf("owner %q is not dawn or dusk", args[1])
		}
		i := slices.Index(sevensigns.Seals[:], seal)
		if sc.SealOwners[i] != sevensigns.NoCabal {
			return fmt.Errorf("seal %s owned twice", args[0])
		}
		sc.SealOwners[i] = owner
	case "memo":
		if len(args) != 2 {
			return fmt.Errorf("memo <var> <value>")
		}
		for _, m := range sc.Memos {
			if m.Var == args[0] {
				return fmt.Errorf("memo %s seeded twice", args[0])
			}
		}
		sc.Memos = append(sc.Memos, Memo{Var: args[0], Value: args[1]})
	}
	return nil
}

// spoil reads a spoil line: spoil <role> [other].
func (sc *Scenario) spoil(args []string, roles map[string]bool) error {
	if len(args) < 1 || len(args) > 2 || (len(args) == 2 && args[1] != "other") {
		return fmt.Errorf("spoil <role> [other]")
	}
	if !roles[args[0]] {
		return fmt.Errorf("spoil: unknown role %q (name its npc line first)", args[0])
	}
	for _, s := range sc.Spoils {
		if s.Role == args[0] {
			return fmt.Errorf("role %q spoiled twice", args[0])
		}
	}
	sc.Spoils = append(sc.Spoils, Spoil{Role: args[0], Other: len(args) == 2})
	return nil
}

// checkStep validates a step's arguments against the declared roles.
func (sc *Scenario) checkStep(verb string, args []string, roles map[string]bool) error {
	if err := sc.checkFightStep(verb, args, roles); err != nil {
		return err
	}
	if slices.Contains(stateVerbs, verb) {
		return sc.checkStateStep(verb, args, roles)
	}
	switch verb {
	case "action", "kill", "approach", "look", "walk", "event":
		if !roles[args[0]] {
			return fmt.Errorf("unknown role %q", args[0])
		}
	}
	switch verb {
	case "event":
		if _, err := strconv.ParseInt(args[1], 10, 32); err != nil {
			return fmt.Errorf("event id %q: %w", args[1], err)
		}
		for _, a := range args[2:] {
			if err := sc.checkObjectRef(a); err != nil {
				return fmt.Errorf("event argument: %w", err)
			}
		}
	case "hit":
		if _, err := strconv.ParseInt(args[1], 10, 32); err != nil {
			return fmt.Errorf("damage %q: %w", args[1], err)
		}
	case "kill":
		if len(args) == 2 {
			if _, err := killCount(args[1]); err != nil {
				return err
			}
		}
	case "tick":
		if len(args) == 1 {
			if _, err := killCount(args[0]); err != nil {
				return fmt.Errorf("tick count %q is not xN", args[0])
			}
		}
	case "advance":
		if _, err := time.ParseDuration(args[0]); err != nil {
			return err
		}
	case "pickup", "mark":
		if _, err := strconv.ParseInt(args[0], 10, 32); err != nil {
			return fmt.Errorf("%s number %q: %w", verb, args[0], err)
		}
	case "look":
		if _, err := lookArgs(args); err != nil {
			return err
		}
	}
	return nil
}

// killCount reads a kill step's xN.
func killCount(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(s, "x"))
	if err != nil || !strings.HasPrefix(s, "x") || n < 1 {
		return 0, fmt.Errorf("kill count %q is not xN", s)
	}
	return n, nil
}

// expect adds one indented line to the step.
func (s *Step) expect(n int, line string) error {
	kind, rest, _ := strings.Cut(line, " ")
	switch kind {
	case "S":
		s.Packets = append(s.Packets, rest)
	case "to":
		name, packet, ok := strings.Cut(rest, " ")
		if !ok || packet == "" {
			return fmt.Errorf("to <name> <packet>")
		}
		if s.To == nil {
			s.To = map[string][]string{}
		}
		s.To[name] = append(s.To[name], packet)
	case "Q":
		s.Statements = append(s.Statements, rest)
	case "M":
		if err := checkMemo(rest); err != nil {
			return err
		}
		s.Memos = append(s.Memos, rest)
	case "live":
		args := strings.Fields(rest)
		if len(args) != 2 {
			return fmt.Errorf("live <npc id> <count>")
		}
		if _, err := strconv.ParseInt(args[0], 10, 32); err != nil {
			return fmt.Errorf("live npc id %q: %w", args[0], err)
		}
		if n, err := strconv.Atoi(args[1]); err != nil || n < 0 {
			return fmt.Errorf("live count %q is not a number of 0 or more", args[1])
		}
		s.Lives = append(s.Lives, args[0]+"="+args[1])
	case "spawned":
		spawn, err := parseSpawned(strings.Fields(rest))
		if err != nil {
			return err
		}
		s.Spawns = append(s.Spawns, spawn)
	case "queue":
		if s.Queues != nil {
			return fmt.Errorf("a second queue line")
		}
		queues := strings.Fields(rest)
		if len(queues) == 0 || slices.ContainsFunc(queues, func(q string) bool { _, _, ok := queueTask(q); return !ok }) {
			return fmt.Errorf("queue %q is not own or other[+n], one per task", rest)
		}
		s.Queues = queues
	case "random":
		if s.Random != nil {
			return fmt.Errorf("a second random line in a step")
		}
		rd, err := parseRandom(strings.Fields(rest))
		if err != nil {
			return err
		}
		s.Random = rd
	case "items", "rows", "state", "hate", "desires", "at", "mem", "hp", "mp", "privates", "master", "top", "effect", "dead", "inworld", "stance", "reach", "shows":
		args := strings.Fields(rest)
		if kind == "rows" {
			var err error
			if args, err = splitQuoted(rest); err != nil {
				return err
			}
		}
		if err := checkCheck(kind, args); err != nil {
			return err
		}
		s.Checks = append(s.Checks, Check{Line: n, Kind: kind, Args: args})
	default:
		return fmt.Errorf("unknown expectation %q", kind)
	}
	return nil
}

// splitQuoted splits line into fields on blanks, as strings.Fields does,
// except that a double quote opens a stretch in which blanks stay in the
// field; the quotes themselves are dropped, so quiz="20 21" is the one field
// quiz=20 21. An unclosed quote is an error.
func splitQuoted(line string) ([]string, error) {
	var fields []string
	var cur strings.Builder
	inField, quoted := false, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted, inField = !quoted, true
		case !quoted && (r == ' ' || r == '\t'):
			if inField {
				fields = append(fields, cur.String())
				cur.Reset()
				inField = false
			}
		default:
			cur.WriteRune(r)
			inField = true
		}
	}
	if quoted {
		return nil, fmt.Errorf("unclosed quote in %q", line)
	}
	if inField {
		fields = append(fields, cur.String())
	}
	return fields, nil
}

// parseSpawned reads a spawned line's <npc id> <x>,<y>,<z> ... | <npc id> -
// into its normal form: the id, then the places sorted, or - for none.
func parseSpawned(args []string) (string, error) {
	usage := fmt.Errorf("spawned <npc id> <x>,<y>,<z> ... | spawned <npc id> -")
	if len(args) < 2 {
		return "", usage
	}
	if _, err := strconv.ParseInt(args[0], 10, 32); err != nil {
		return "", fmt.Errorf("spawned npc id %q: %w", args[0], err)
	}
	if len(args) == 2 && args[1] == "-" {
		return args[0] + " -", nil
	}
	places := slices.Clone(args[1:])
	for _, p := range places {
		xyz := strings.Split(p, ",")
		if len(xyz) != 3 {
			return "", usage
		}
		if _, err := ints(xyz); err != nil {
			return "", fmt.Errorf("spawned place %q: %w", p, err)
		}
	}
	slices.Sort(places)
	return args[0] + " " + strings.Join(places, " "), nil
}

// queueTask reads one task of a queue line: whether its hook ran on the
// character's own queue, and, for another queue's, how many lines after each
// cast move with it (0 for the default rule). ok is false for anything else.
func queueTask(task string) (own bool, effects int, ok bool) {
	if task == "own" {
		return true, 0, true
	}
	rest, found := strings.CutPrefix(task, "other")
	if !found {
		return false, 0, false
	}
	if rest == "" {
		return false, 0, true
	}
	n, found := strings.CutPrefix(rest, "+")
	effects, err := strconv.Atoi(n)
	if !found || err != nil || effects < 1 || n != strconv.Itoa(effects) {
		return false, 0, false
	}
	return false, effects, true
}

// checkMemo validates an M line: set <var> | <value> or unset <var>.
func checkMemo(line string) error {
	op, rest, _ := strings.Cut(line, " ")
	switch op {
	case "set":
		if k, _, ok := strings.Cut(rest, " | "); ok && k != "" && !strings.Contains(k, " ") {
			return nil
		}
	case "unset":
		if rest != "" && !strings.Contains(rest, " ") {
			return nil
		}
	}
	return fmt.Errorf("memo %q is not set <var> | <value> or unset <var>", line)
}

func checkCheck(kind string, args []string) error {
	switch kind {
	case "hate", "desires", "at":
		return checkFightCheck(kind, args)
	case "mem", "hp", "mp", "privates", "master", "top", "effect", "dead", "inworld", "stance", "reach", "shows":
		return checkStateCheck(kind, args)
	case "items":
		if len(args) == 0 {
			return fmt.Errorf("items takes one or more <id>=<count>")
		}
		if !strings.Contains(args[0], "=") && len(args) > 1 {
			// A mate's count: items <mate> <id>=<n> ...
			args = args[1:]
		}
		for _, a := range args {
			id, count, ok := strings.Cut(a, "=")
			if _, err := strconv.ParseInt(id, 10, 32); !ok || err != nil {
				return fmt.Errorf("item %q is not <id>=<count>", a)
			}
			if _, err := strconv.Atoi(count); err != nil {
				return fmt.Errorf("item %q is not <id>=<count>", a)
			}
		}
	case "rows":
		if len(args) < 2 {
			return fmt.Errorf("rows <quest> <var>=<value> ... | -")
		}
		if len(args) == 2 && args[1] == "-" {
			return nil
		}
		for _, a := range args[1:] {
			if _, _, ok := strings.Cut(a, "="); !ok {
				return fmt.Errorf("row %q is not <var>=<value>", a)
			}
		}
	case "state":
		if len(args) != 2 {
			return fmt.Errorf("state <quest> <status> | -")
		}
	}
	return nil
}

// FirstDiff compares a step's got lines with its wanted ones and describes
// the first difference: the first line that differs, or the first one
// missing or extra; "" when they are equal.
func FirstDiff(what string, got, want []string) string {
	for i := range max(len(got), len(want)) {
		switch {
		case i >= len(got):
			return fmt.Sprintf("%s %d missing:\n want %s", what, i+1, want[i])
		case i >= len(want):
			return fmt.Sprintf("%s %d not expected:\n  got %s", what, i+1, got[i])
		case got[i] != want[i]:
			return fmt.Sprintf("%s %d differs:\n  got %s\n want %s", what, i+1, got[i], want[i])
		}
	}
	return ""
}

// parseClock reads a clock line's hh:mm[:ss] as the time since midnight.
func parseClock(s string) (time.Duration, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("clock %q is not hh:mm[:ss]", s)
	}
	var d time.Duration
	for i, limit := range []int{24, 60, 60}[:len(parts)] {
		v, err := strconv.Atoi(parts[i])
		if err != nil || v < 0 || v >= limit {
			return 0, fmt.Errorf("clock %q is not hh:mm[:ss]", s)
		}
		d += time.Duration(v) * []time.Duration{time.Hour, time.Minute, time.Second}[i]
	}
	return d, nil
}

// characterRef is the word an NPC parameter or event argument writes for
// the character's object id.
const characterRef = "character"

// checkObjectRef checks an NPC parameter or event argument: empty (0), a
// number, characterRef or the role of an NPC line above.
func (sc *Scenario) checkObjectRef(ref string) error {
	if ref == "" || ref == characterRef {
		return nil
	}
	if _, err := strconv.ParseInt(ref, 10, 32); err == nil {
		return nil
	}
	if !slices.ContainsFunc(sc.NPCs, func(n NPC) bool { return n.Role == ref }) {
		return fmt.Errorf("%q is not a number, character or the role of an npc line above", ref)
	}
	return nil
}
