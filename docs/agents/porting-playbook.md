# Behavioral porting playbook

Use this workflow for issue-scoped implementation. Scale the ceremony to uncertainty and risk.

## 1. Establish current scope

1. If no issue number is given, shortlist locally with `python3
   scripts/agent-tools/issue_search.py '<milestone or scope terms>' --state open --milestone
   '<milestone>' --limit 20`. Do not list/search live issues or refresh the cache first. Query at most
   three plausible candidates live; use bounded live discovery only if the cache cannot supply one.
2. Fetch the known or shortlisted issue from `fatal10110/acis_golang`; read its current body,
   comments, labels, milestone, parent/child context, and state. Local snapshots are never status
   authority.
3. Read the relevant section of `aCis_gameserver/GO_REWRITE_PLAN.md` and the behavioral documents
   under `aCis_gameserver/docs/go-rewrite/`.
4. Search `.agent-cache/INDEX.md` and linked reports. Treat cached findings as hints until their
   evidence and revision are verified.
5. State the bounded deliverable and any adjacent subsystem that is deliberately out of scope.

Routine work following an established package pattern may use a short plan and proceed. Require
explicit design approval before introducing a new package boundary, public API, concurrency ownership
model, persistence representation, significant dependency, or major cross-system design.

## 2. Run one bounded investigation

Do not scatter a feature's research across many trivial agents or repeated broad reads.

- Use `rg` for known identifiers, constants, opcodes, filenames, and strings.
- Use grepai semantic search when the implementation location is unknown, and grepai trace for call
  relationships. Constrain path, output format, and result count.
- Use `gopls` for Go definitions, references, implementations, diagnostics, and call hierarchy.
- Use ast-grep for syntax-shaped matches or structural rewrites.
- Read only the returned ranges. Expand to a full file when control flow cannot be understood safely
  from targeted ranges.
- Do not use `find`, POSIX `grep`, or `sed` as a discovery loop. After one exact `rg`
  miss, switch to grepai, `gopls`, or ast-grep according to the question.
- Use `java-lookup` for one exact lookup. Use `java-researcher` once for a feature-level multi-file
  investigation. An ordinary issue gets at most one research agent.

The investigation reports observable behavior, not source architecture:

- inputs and preconditions;
- state transitions;
- outputs and packet order;
- every rejection branch;
- persistence effects;
- constants, formulas, rounding, and overflow;
- exact evidence paths and symbols;
- uncertainties and conflicts in the evidence.

Never paste complete methods or large source excerpts into reports.

## 3. Check the client-visible surface

For any client-visible behavior, inspect both inbound handlers and outbound sends before coding.
Identify opcodes, field layout, state gates, send order, broadcasts, and every early return. Read
[`../../internal/gameserver/network/AGENTS.md`](../../internal/gameserver/network/AGENTS.md) for the
packet-impact and silent-rejection contract.

If required behavior depends on a later system, document the exact dependency and integration point.
Do not leave an accepted opcode as a quiet no-op.

**Parity exception: JDK Unicode quirks are not ported.** Client and data text follows the Go standard
library (`unicode`, `strings`, `strconv`, `unicode/utf8`, `unicode/utf16`): an unpaired UTF-16
surrogate decodes to U+FFFD at the wire, case mapping uses the toolchain's Unicode tables (no
Unicode-version case tables, no final-sigma or U+0130 special cases), and integers parse ASCII digits
only. Do not open issues to emulate these. UTF-16 length counting (`commons.UTF16Len`/`UTF16Cut`)
stays a core rule, and stdlib behavior must never accept input the server would otherwise reject for
abuse (length, name validity, duplicate names).

**Parity exception: joining a party takes the joiner off the party-match waiting list** (#5000). The
reference adds a player who accepts a room leader's party invitation to the room but leaves it on the
waiting list. Entry into a room checks only the waiting list, so a listed player could then enter a
second room. Go removes the joiner from the waiting list (`partymatch.Registry.PartyJoined`). Pinned by
`TestPartyJoinedTakesJoinerOffWaitingList` (`internal/gameserver/partymatch`).

**Parity exception: quick join records the real room id** (#5000). The reference stores the requested
id, 0, on a player who quick-joins the first available room. The party-room byte in UserInfo/CharInfo
then stays 0, the player's room chat is dropped, the leader cannot oust it, and logging out does not
take it out of the room. Go records the id of the room actually entered (`partymatch.Registry.Join`).
Pinned by `TestPartyRoomOpenBrowseEnterLeave` (`tests/party`): the quick-join entrant's room line
reaches the room and its withdrawal is shown to the leader.

**Parity exception: a disconnecting party leader always hands over to the first remaining member**
(#5000). The reference refuses to promote a member who is in a duel, so when that member is first in
line the party keeps the leader who just disconnected. Go promotes the first remaining member
regardless (`party.Registry.Leave`); the party duel is cancelled in both. Pinned by
`TestDisconnectingLeaderHandsOverToDuelingMember` (`internal/gameserver/party`).

**Parity exception: an unsold clan hall sale is dropped** (#5018). The reference keeps the seller and
the end date of a sale that ended without a bid, arms no new end and leaves the hall listed. Go tells
the selling clan the hall was not sold, then drops the sale, clears the end date and takes the hall
off the listing (`clanhall.Halls.endAuctionLocked`, also reached by the admin auction end). The
deposit stays spent. Pinned by `TestAuctionEndsWithNoBidOnAHallForSale` (`tests/npcs`).

**Parity exception: re-confirming a registered clan hall sale takes no second deposit** (#5018). The
reference takes the lease deposit again, overwrites the sale terms and answers as for a new sale. Go
leaves the registered sale and its deposit as they are and answers with the sale's page alone
(`clanhall.SaleAlreadyRegistered`). Pinned by `TestAuctionSaleReconfirmTakesNoSecondDeposit`
(`tests/npcs`).

**Parity exception: a clan hall sale's terms wait for the deposit** (#5018). The reference writes the
seller, minimum bid and end date into the live auction when the sale is drafted, before the deposit
is paid. Go keeps them in a draft until the deposit lands and times the end from the confirmed sale
(`clanhall.Halls.DraftSale`, `ConfirmSale`). Pinned by `TestAuctionSaleRegistersOnDeposit`
(`tests/npcs`).

**Parity exception: clan hall function fees stop with the functions** (#5018). On a change of owner
the reference drops a hall's functions but leaves their fee tasks running, so they charge the new
owner, and cancelling a function the hall does not rent creates a level-0, fee-0 function whose term
re-runs forever. Go stops the fees with the functions (`clanhall.Functions.RemoveAll`) and creates
nothing for such a cancel (`Functions.Update`). Pinned by `TestRemoveAllStopsFees`,
`TestUpdateLevelZeroRentsNothing` and `TestFunctionWithoutTermChargedOnce`
(`internal/gameserver/clanhall`).

**Parity exception: clan hall auctions resolve clans by id** (#5113). The reference resolves the
seller, the winner and each refunded bidder by clan name and keeps a destroyed clan's bid, so a
namesake founded later is refunded or awarded, and a sale whose highest bidder was destroyed still
pays the seller. Go resolves every one by id, and destroying a clan withdraws its bid as a
cancellation does; the refund finds the clan gone and pays nothing (`clanhall.Halls.ForgetClan`).
Pinned by `TestAuctionDestroyedLoserRefundsNoNamesake`, `TestAuctionSaleWithADestroyedHighestBidder`
(`tests/clan`) and `TestAuctionWinnerIsTheBiddingClanByID` (`tests/npcs`).

**Parity exception: a lure with no grade is refused** (#5118). The reference casts a line baited with
an equippable lure its grade table does not list (6528, 7067-7070, 8496-8504), uses one up and holds
the caster in place with no bite to come and no way out but a relog. Go refuses the cast before
anything is used up, with the bait notice (`fishing.RefuseUngradedLure`). Pinned by
`TestCastRefusalUngradedLure` (`internal/gameserver/fishing`) and
`TestFishingCastRefusesUngradedLure` (`tests/items`).

**Parity exception: a race ticket sells only while sales are open** (#5119). The reference checks
that sales are open, then later takes the adena and adds the stake, so a purchase straddling the
close is sold after it and left out of the odds. Go checks, takes the adena and stakes in one hold of
the track's lock (`derby.Track.Bypass`); a purchase that finds sales closed takes nothing. Pinned by
`TestBuyTicketStakesBeforeTheSalesClose` (`internal/gameserver/derby`) and
`TestDerbyTicketBoughtDuringTheClosingTick` (`tests/npcs`).

**Parity exception: a quoted dollar sign in an HTML value still sends the page** (#4718). The
reference refuses a placeholder value with an odd run of backslashes before `$` (`a\$`, `a\$1`) and
never sends the page. The player-text callers are a clan notice, petition content and feedback
(`internal/gameserver/network/admin_petition.go`) and a buffer scheme name
(`internal/gameserver/schemebuffer/dialog.go`); any of them holding `\$` fails the page, so a clan
notice fails every member's entry. Go reads the backslash as quoting the `$` like any other
character and sends the page showing `a$` (`commons.HTMLValue`). Pinned by `TestHTMLValue`
(`internal/commons`).

**Parity exception: a missing or malformed `.properties` file stops boot** (#4772). The reference logs
the error and carries on: a missing file gives every key its default, and a malformed one keeps the
keys read before the bad line. Go returns the error (`config.LoadFile`) and the server stops booting.
Pinned by `TestLoadFileRefusesMissingAndMalformedFiles` (`internal/config`).

**Parity exception: a blank line in `banned_ips.properties` is skipped silently** (#4909). The
reference reports a blank line as an invalid ban address. Go skips blank and whitespace-only lines
without a log (`manager.LoadIPBanList`); a bad address is still reported. Pinned by
`TestLoadIPBanList_SkipsBlankLinesSilently` (`internal/loginserver/data/manager`).

**Parity exception: `//reload zone` drops what the old zones raised** (#4801). The reference drops its
zones without their exit rules but leaves the flags they raised on the creatures inside, so a player in
town stays peaceful after leaving it until a relog. Go drops every hold of the old zones with them and
enters the new zones around each creature (`zone.zoneSet.sync`); boat holds and boss grants are kept
across the reload (#5198). Pinned by `TestReloadMovesMembersWithoutExitRules`
(`internal/gameserver/model/zone`).

**Parity exception: the wyvern countdown re-arms after it runs** (#4802). The reference never clears a
spent forced-dismount countdown, so after the first forced dismount a rider entering a no-landing zone
or a siege again only gets the warning and flies on. Go clears the countdown once it has run, so every
entry starts a new one (`network.armWyvernDismount`). Pinned by `TestWyvernDismountedAgainOnNextSiege`
(`tests/pets`).

**Parity exception: a pet stack's count change reaches the pet window** (#4813). The reference queues an
item update only for the player's inventory and paperdoll, so a count change on a pet's stack (feeding,
a merge from give-to-pet or pickup, a partial take-back or destroy) leaves the pet window stale. Go
sends a `PetInventoryUpdate` listing the stack with its new count
(`network.petInventoryDelivery.QueueInventoryUpdate`). Pinned by
`TestPartialPetStackMoveListsModifiedEntries` (`tests/pets`).

**Parity exception: the summoned pet's collar never drops on death** (#4973). The death drop skips the
control item of the pet the player has out or rides, matched by object id. The reference compares an
object id with a template id there, so only the configured kept-pet-items list protects a collar. With
the stock list nothing differs. With a list that leaves out a collar, the reference drops it and Go
keeps it. Pinned by `tests/pets` `TestDeathDropKeepsSummonedPetCollar`.

**Parity exception: crafted private store and trade requests are refused** (#4826). The reference lets
a buyer take more units than a sell row lists (it checks only what the owner holds), take part of a
package sale, sell into a buy row an item of another enchant or more units than the row wants, and put
a worn item on a sell list or in a trade offer. Go refuses each with nothing moved
(`privatestore.Store` deal and list fills, `trade.manipulableItem`). Pinned by `TestDealDivergences`
(`internal/gameserver/privatestore`) and `TestEquippedItemRefusedOnSellListAndTradeOffer`
(`tests/trade`).

**Parity exception: `FreightChar` opens only the account's own characters** (#4826). The reference opens
the freight of whatever character id the command names. Go opens nothing for a character outside the
player's account (`network` warehouse `FreightCharacter` reply). Pinned by
`TestFreightCharDepositsIntoAccountCharacter` (`tests/npcs`).

**Parity exception: region-based freight is tagged with a stable region key** (#4826). The reference
tags region-based freight with an identity hash of the region object, which changes on every run and
cannot be reproduced. Go tags it with `world.RegionKey`, a positive number the same on every boot.
Pinned by `TestPackageSendNeedsKeeperAndTagsItsRegion` (`tests/npcs`).

**Parity exception: merchant lists refuse a GM list and an overflowing sale** (#4829). The reference
opens a buy list no NPC sells (npc id 0 or below) to any client that names it, and a sale whose payment
passes the largest int32 destroys the rows before the overflowing one and pays nothing. Go refuses
such a list to a player who is not a GM (`network` merchant buy) and refuses an overflowing sale whole
with nothing taken (`inventory.Service.Sell`). Pinned by `TestMerchantRefusesGMListToPlayer` and
`TestSellItemRefusals` (`tests/npcs`).

**Parity exception: `//enchant` updates the enchanted player's inventory window** (#5042). The reference
queues the enchanted item's `InventoryUpdate` on the GM's inventory, so the target's client gets none.
Go queues it on the owner's inventory (`network` admin enchant). Pinned by
`TestAdminEnchantSelectedPlayerInventoryUpdate` (`tests/admin`).

## 4. Design the native Go shape

Search the Go repository for existing domain types, helpers, constructors, fixtures, and package
patterns before creating anything. Reuse the smallest existing mechanism that holds.

Extract the required behavior, then design native Go packages and APIs. Do not preserve class graphs,
singleton patterns, getter walls, or control flow merely because the behavior was discovered there.
Keep interfaces at their consumers and keep network packages limited to orchestration.

Classify the task with [`process-skills.md`](process-skills.md). A routine established-pattern port is
Class B; behavior-critical work uses the concise Class C test-first flow; only a real architecture
choice or ambiguity needs the Class D design and approval flow.

## 5. Implement the bounded unit

- Keep shared datapack assets read-only. Correct the parser or consumer.
- Preserve integer width, byte order, formula order, rejection behavior, and persistence effects.
- When a dependency is not ported, do not hardcode its data or absorb the neighboring subsystem.
  Defer the member only when the remaining unit is still useful and correct; otherwise stop and fix
  the scope.
- Return errors from library code. Let composition roots decide whether boot-time failure is fatal.
- Search `cmd/loginserver/main.go` and `cmd/gameserver/main.go` for the relevant Fx provider, invoke,
  data load, listener, lifecycle hook, or runbook entry. Wire boot behavior when the component must be
  live; otherwise document why it is library-only.

## 6. Verify against the contract

- Non-trivial logic leaves the smallest standard-library test that would fail if the behavior
  regressed.
- Exact formulas use expected values produced by an independent reference probe.
- Loaders compare counts and representative field dumps from the same asset.
- Protocol uses byte fixtures or `cmd/packetdiff`; geodata uses `cmd/geoprobe` where applicable.
- Concurrent code gets a focused race test.
- Run focused checks during implementation, then the final gates in
  [`verification.md`](verification.md).

## 7. Resolve gaps immediately and audit before delivery

Do not postpone this workflow until implementation is complete. When a gap, deferral, divergence, or
out-of-scope item is first identified, pause and complete steps 1–5 before continuing. Repeat the
audit against the final diff before delivery.

For every implementation gap, deferral, divergence, or deliberately out-of-scope item that will
ship:

1. From the outer workspace root, run
   `python3 scripts/agent-tools/issue_search.py '<specific terms>' --state all --limit 10`. The cache
   search covers issue numbers, titles, bodies, comments, labels, states, and milestones; comments
   are evidence, not optional metadata.
2. Fetch each plausible match live with
   `gh issue view <number> -R fatal10110/acis_golang --comments --json number,title,body,comments,state,milestone,labels,url`.
   Cached content is discovery-only.
3. When a live issue covers the gap, reuse it. If its body or milestone is incomplete, amend it in
   place while preserving useful history; never create a duplicate. If it is closed but appears
   unresolved, inspect the closure reason and ask before reopening it.
4. When no issue covers the gap, create one containing: source issue or PR context; observed and
   expected behavior; evidence paths and symbols; why the work is deferred; dependency and Go
   integration point; bounded scope and explicit non-scope; acceptance criteria; verification; and
   related issue, PR, and specification links.
5. Reuse the current issue's milestone when the follow-up belongs to the same workstream. Otherwise
   inspect live open milestones and select the one whose documented scope owns the work. Ask instead
   of creating an unassigned or guessed issue when ownership is ambiguous.

Link the existing, amended, or created issue from the current issue or pull request and report its
number in the completion summary. Creating and amending required follow-ups is authorized by the
repository instructions; closing, reopening, or changing unrelated issues still requires explicit
authorization.

Record exactly one compact result in the pull request and final response:

- `Gap audit: no shipped gaps`; or
- `Gap audit: <gap> -> #<issue> (OPEN, comments checked, milestone <name>)` for every shipped gap.

Update `.agent-cache/` only for reusable confirmed findings that future work cannot recover cheaply
from the issue, pull request, code, or specifications. Update an existing report instead of creating
a duplicate.

## 8. Deliver issue implementations

Unless the user explicitly requests local-only work, an issue implementation is incomplete until it
is delivered. After all required gates pass, use a non-`main` branch (default `codex/` prefix), stage
only in-scope files, create a buildable commit, push it, and open a draft pull request against `main`.
Include tests and oracle evidence, the issue-closing keyword, and links for deferred gaps. If branch
safety, credentials, network access, or a required gate blocks delivery, report that exact blocker
instead of claiming completion. Never close the issue directly or merge the pull request without
explicit authorization.
