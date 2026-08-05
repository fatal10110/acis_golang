# Action response contract

How to decide what a handler owes the client after it accepts a user action. Load this when adding or
changing a handler for a client action packet.

## The client's pending action

When the client accepts an action click it registers a pending action and locks further input until the
server resolves it. Two distinct failures leave it outstanding:

- a rejection branch that returns without sending anything;
- a **successful** action whose response never releases the click.

The second is the easier one to miss. Packets that describe the resulting world — `GetItem`,
`DeleteObject`, `InventoryUpdate`, `PetStatusShow` — do not release the pending action. They report what
changed, not that the requested action finished. When the release is missing the character stops
responding to *every* later click, not just the dropped one, and the session usually ends with the
client being killed rather than with a server-side error.

## Which flows release on success

Not every flow releases on success, and adding a release where the reference does not send one is
equally wrong. Decide per flow from the reference's think/intention entry point, and check whether the
client-action-failed send sits **before** the guards or **inside** one:

| Shape | Flows | Rule |
|---|---|---|
| Unconditional — first statement of the think method | pickup, interact (including clicking an owned summon and click-driven chair sit), follow | Release on success **and** rejection |
| Inside a guard | attack, cast, sit key / action-bar sit, stand, move-to, fake death | Release on rejection **only**; success answers with its own packets (`ChangeWaitType` + `ChairSit`, an attack frame, and so on) |

Chair sit has two entry points with different shapes: a second Action click on the chair routes
through `StaticObject.onAction` -> `tryToInteract` -> `thinkInteract`, whose first statement is an
unconditional `clientActionFailed()` (PlayerAI.java:415) — the click-driven sit belongs in the
unconditional row. The sit key and action-bar sit button route through `tryToSit`
(PlayerAI.java:430), which only sends `clientActionFailed()` on a `denyAiAction()` rejection, never
on success — that path stays in the guard row.

Two traps:

- Do not generalize from one flow to its neighbours by intuition. Sit and pickup sit in different
  groups despite both being second-click Action targets.
- The release is a no-op for non-player actors, so a pet-driven variant of a player flow may
  legitimately send nothing. Check whose AI owns the action before copying the player behavior.

## Verifying it

- Rejection branches: `silent_action_test.go` is the guardrail. Every case there sends a request built
  to be rejected and asserts at least one frame comes back. Extend it when adding an action-shaped
  opcode or rejection branch.
- Success paths: `silent_action_test.go` cannot help — it only probes rejections. Assert the release in
  the flow's own success-path test, in packet order. See
  `internal/gameserver/network/pickup_test.go` (`TestGameClientLinkPickupGroundItemFullClientFlow`,
  `TestGameClientLinkPickupAdenaMergeFullClientFlow`) and the owned-summon case in `pet_test.go`
  (`TestHandleTargetActionShowsPetStatusForOwnerPet`).

A handler that answers rejections correctly can still freeze a live client on success, and every gate in
the repository can be green while it does. Tests that only read opcodes will not notice; the symptom
appears only against a real client.
