# Packet gap audit - 2026-07-14

Scope correction: packets owned by M0, M1, M2, M3, M4, and M5 are treated as required now.

The original game packet appendices do not label packets by milestone, so this audit maps packets to closed milestone scope:

- M0: no concrete protocol packets.
- M1: login client/server packets and GS-LS link packets.
- M2: game client connect, character create/delete/restore/select, and EnterWorld baseline.
- M3: data-backed UI/content packets for HTML, crests, multisell, buylists, hennas, recipes, skill trees, manor, cursed weapons, augmentation, fish data, and related screens.
- M4: world, visibility, spawn, doors/static objects, movement, vehicles, time/day-night, and teleport/restart movement packets.
- M5: stats, combat, item/inventory/container, shots, pet inventory/status, skill acquisition/progression, death/respawn, and related update packets.

Sources checked:

- `../aCis_gameserver/docs/go-rewrite/90-appendix-game-client-packets.md`
- `../aCis_gameserver/docs/go-rewrite/91-appendix-game-server-packets.md`
- `../aCis_gameserver/docs/go-rewrite/92-appendix-login-packets.md`
- `../aCis_gameserver/docs/go-rewrite/16-connection-flow.md`
- `../aCis_gameserver/GO_REWRITE_PLAN.md`
- `../aCis_gameserver/java/net/sf/l2j/gameserver/network/clientpackets/EnterWorld.java`
- `internal/gameserver/network/clientpackets/`
- `internal/gameserver/network/serverpackets/`
- `internal/link/`
- `internal/loginserver/network/clientpackets/`
- `internal/loginserver/network/serverpackets/`

## Summary

- Original game client appendix: 206 concrete dispatcher targets.
- Original game server appendix: 282 packet classes, including base/composite classes.
- Classified M2-M5 required game client packets: 94. Missing in Go: 48.
- Classified M2-M5 required game server packets: 128. Missing in Go: 65.
- M1 login client/server packets are implemented.
- M1 GS-LS link packets are implemented under `internal/link/`.
- M2 base game connect/create/select packet set is implemented.
- The big gaps now start at clan-specific EnterWorld packets plus M3-M5 data/world/item/combat packet surfaces.

GitHub backlog parents opened for packets still not implemented:

- Client packets: [#553 Packet backlog: remaining M3-M5 client packets](https://github.com/fatal10110/acis_golang/issues/553)
- Server packets: [#630 Packet backlog: remaining M2-M5 server packets](https://github.com/fatal10110/acis_golang/issues/630)

## M1 Login/Link

No concrete M1 packet gap found.

Login client packets present in Go:

- `AuthGameGuard`
- `RequestAuthLogin`
- `RequestServerList`
- `RequestServerLogin`

Login server packets present in Go:

- `AccountKicked`
- `GGAuth`
- `Init`
- `LoginFail`
- `LoginOk`
- `PlayFail`
- `PlayOk`
- `ServerList`

GS-LS link packets present in Go under `internal/link/`:

- `AuthResponse`
- `BlowFishKey`
- `ChangeAccessLevel`
- `GameServerAuth`
- `InitLS`
- `KickPlayer`
- `LoginServerFail`
- `PlayerAuthRequest`
- `PlayerAuthResponse`
- `PlayerInGame`
- `PlayerLogout`
- `ServerStatus`

## Game Client Packet Gaps

M2 required client packets are complete:

- `SendProtocolVersion`
- `AuthLogin`
- `Logout`
- `RequestCharacterCreate`
- `RequestCharacterDelete`
- `RequestGameStart`
- `RequestNewCharacter`
- `CharacterRestore`
- `EnterWorld`

Missing M3 data/UI client packets:

- `RequestBBSwrite`
- `RequestSetPledgeCrest`
- `RequestSetAllyCrest`
- `RequestExSetPledgeCrestLarge`
- `MultiSellChoose`
- `RequestBuyItem`
- `RequestSellItem`
- `RequestPreviewItem`
- `RequestBuyProcure`
- `RequestBuySeed`
- `RequestProcureCropList`
- `RequestSetSeed`
- `RequestSetCrop`
- `RequestHennaItemList`
- `RequestHennaItemInfo`
- `RequestHennaEquip`
- `RequestHennaUnequipList`
- `RequestHennaUnequipInfo`
- `RequestHennaUnequip`
- `RequestRecipeBookOpen`
- `RequestRecipeBookDestroy`
- `RequestRecipeItemMakeInfo`
- `RequestRecipeItemMakeSelf`
- `RequestRecipeShopMessageSet`
- `RequestRecipeShopListSet`
- `RequestRecipeShopManageQuit`
- `RequestRecipeShopMakeInfo`
- `RequestRecipeShopMakeItem`
- `RequestRecipeShopManagePrev`
- `RequestExEnchantSkillInfo`
- `RequestExEnchantSkill`
- `RequestExFishRanking`
- `RequestConfirmTargetItem`
- `RequestConfirmRefinerItem`
- `RequestConfirmGemStone`
- `RequestConfirmCancelItem`

`RequestBuyItem` and `RequestSellItem` currently have Go decoders and byte-layout tests only. They
are still counted as gaps because merchant NPC context, buylist loading/restock wiring, buy/sell
inventory mutation, adena persistence, and the NPC dialog/bypass owner flow are not implemented.

`RequestExEnchantSkillInfo` and `RequestExEnchantSkill` currently have Go decoders and byte-layout
tests only. They are still counted as gaps because the skill-enchant tree, trainer validation, cost
payment, success/failure roll, and skill persistence flow are not implemented.

`RequestConfirmTargetItem`, `RequestConfirmRefinerItem`, `RequestConfirmGemStone`, and
`RequestConfirmCancelItem` currently have Go decoders and byte-layout tests only. They are still
counted as gaps because the live augmentation validation/apply/remove flow is not implemented.

Implemented and wired M3 data/UI client packets in Go:

- `RequestBypassToServer` (`player_help` HTML bypass only; admin, NPC, quest, community-board, hero, olympiad, and manor bypass owners remain deferred until those systems exist)
- `RequestLinkHtml`
- `RequestAllyCrest`
- `RequestExPledgeCrestLarge`
- `RequestPledgeCrest`
- `RequestCursedWeaponList`
- `RequestCursedWeaponLocation` (accepted; no response is emitted while no cursed weapon is active)

Implemented and wired M4 movement/rotation/target client packets in Go:

- `Appearing`
- `CannotMoveAnymore`
- `RequestTargetCancel`
- `StartRotating`
- `FinishRotating`

Missing M4 world/movement client packets:

- `RequestMoveToLocationInVehicle`
- `CannotMoveAnymoreInVehicle`
- `RequestGetOnVehicle`
- `RequestGetOffVehicle`
- `ObserverReturn`

Implemented and wired M5 target/combat/item/stance/social client packets in Go:

- `Action`
- `AttackRequest`
- `RequestAcquireSkillInfo`
- `RequestAcquireSkill`
- `RequestCrystallizeItem`
- `RequestDestroyItem`
- `RequestDropItem`
- `RequestMagicSkillUse`
- `RequestAutoSoulShot`
- `RequestChangeMoveType`
- `RequestChangeWaitType`
- `RequestSocialAction`
- `RequestPackageSendableItemList`
- `RequestEnchantItem`
- `RequestPetUseItem`
- `RequestGiveItemToPet`
- `RequestGetItemFromPet`
- `RequestPetGetItem`
- `SendTimeCheck`

Missing M5 stats/combat/items/progression client packets:

- `RequestChangePetName`
- `SendWarehouseDepositList`
- `SendWarehouseWithdrawList`
- `RequestPackageSend`
- `RequestRestartPoint`

Warehouse/freight transaction packets remain deferred:

- `SendWarehouseDepositList` and `SendWarehouseWithdrawList` need dialog/bypass routing to establish the current warehouse NPC and active warehouse before the client request arrives.
- `RequestPackageSend` needs same-account freight ownership, freight restore/persistence, fee charging, and active warehouse/session validation.
- `RequestPackageSendableItemList` is wired for the preview step and returns `PackageSendableList` for carried, tradable, non-quest inventory items.

## Game Server Packet Gaps

M2 base server packets are complete:

- `VersionCheck`
- `AuthLoginFail`
- `CharSelectInfo`
- `CharCreateOk`
- `CharCreateFail`
- `CharDeleteOk`
- `CharDeleteFail`
- `NewCharacterSuccess`
- `SSQInfo`
- `CharSelected`
- `SkillList`
- `UserInfo`
- `ItemList`
- `ActionFailed`

Implemented and wired EnterWorld burst packets in Go:

- `ExStorageMaxCount`
- `HennaInfo`
- `EtcStatusUpdate`
- `QuestList`
- `FriendList`
- `ShortCutInit`
- `Die`
- `SkillCoolTime`

Remaining EnterWorld burst packet gaps:

- `PledgeSkillList` ([#717](https://github.com/fatal10110/acis_golang/issues/717))
- `ExMailArrived` ([#718](https://github.com/fatal10110/acis_golang/issues/718))
- `PlaySound` ([#719](https://github.com/fatal10110/acis_golang/issues/719))
- `NpcHtmlMessage` ([#720](https://github.com/fatal10110/acis_golang/issues/720))

`PledgeShowMemberListUpdate` ([#631](https://github.com/fatal10110/acis_golang/issues/631)),
`PledgeShowMemberListAll` ([#632](https://github.com/fatal10110/acis_golang/issues/632)),
`PledgeSkillList`, `ExMailArrived`, `PlaySound`, `NpcHtmlMessage`, `BuyList`, and `SellList`
currently have Go frame builders only. `ExEnchantSkillList` and `ExEnchantSkillInfo` also have Go
frame builders only. The augmentation variation packets
`ExShowVariationMakeWindow`, `ExShowVariationCancelWindow`, `ExConfirmVariationItem`,
`ExConfirmVariationRefiner`, `ExConfirmVariationGemstone`, `ExConfirmCancelItem`,
`ExVariationResult`, and `ExVariationCancelResult` also have Go frame builders only. They are not
wired until production owner flows can emit them truthfully.

Missing M3 data/UI server packets:

- `NpcHtmlMessage`
- `MultiSellList`
- `BuyList`
- `SellList`
- `SellListProcure`
- `BuyListSeed`
- `HennaEquipList`
- `HennaItemInfo`
- `HennaUnequipList`
- `HennaItemUnequipInfo`
- `RecipeBookItemList`
- `RecipeItemMakeInfo`
- `RecipeShopItemInfo`
- `RecipeShopManageList`
- `RecipeShopMsg`
- `RecipeShopSellList`
- `ExShowSeedInfo`
- `ExShowCropInfo`
- `ExShowManorDefaultInfo`
- `ExShowSeedSetting`
- `ExShowCropSetting`
- `ExShowSellCropList`
- `ExShowProcureCropDetail`
- `ExEnchantSkillList`
- `ExEnchantSkillInfo`
- `ExShowVariationMakeWindow`
- `ExShowVariationCancelWindow`
- `ExConfirmVariationItem`
- `ExConfirmVariationRefiner`
- `ExConfirmVariationGemstone`
- `ExVariationResult`
- `ExVariationCancelResult`

Implemented and wired M3 data/UI server packets in Go:

- `AllyCrest`
- `ExCursedWeaponList`
- `ExCursedWeaponLocation`
- `ExPledgeCrestLarge`
- `PledgeCrest`

Implemented and wired M4 movement/rotation/static-object server packets in Go:

- `StopMove`
- `ValidateLocation`
- `StartRotation`
- `StopRotation`
- `ChairSit`

Missing M4 world/movement server packets:

- `GetOnVehicle`
- `GetOffVehicle`
- `VehicleDeparture`
- `VehicleInfo`
- `OnVehicleCheckLocation`
- `MoveToLocationInVehicle`
- `StopMoveInVehicle`
- `ValidateLocationInVehicle`
- `VehicleStarted`
- `SunRise`
- `SunSet`

Missing M5 stats/combat/items/progression server packets:

- `PetItemList`
- `PetInfo`
- `PetStatusUpdate`

Implemented M5 item/status server packet encoders in Go, with owning runtime wiring still tracked by the relevant systems:

- `AbnormalStatusUpdate`
- `ExUseSharedGroupItem`
- `MagicSkillCanceled`
- `PackageToList`
- `PackageSendableList`
- `PlaySound`
- `Revive`
- `ShortBuffStatusUpdate`
- `WarehouseDepositList`
- `WarehouseWithdrawList`

Implemented and wired M5 item server packets in Go:

- `ChooseInventoryItem`
- `DropItem`
- `EnchantResult`
- `ExAutoSoulShot`
- `InventoryUpdate`
- `PackageSendableList`
- `PetInventoryUpdate`
- `PetStatusShow`
- `PetDelete`

Implemented and wired M5 target/combat server packets in Go:

- `AutoAttackStart`
- `AutoAttackStop`
- `Attack`
- `ChangeMoveType`
- `ChangeWaitType`
- `MyTargetSelected`
- `SocialAction`
- `TargetSelected`
- `TargetUnselected`
- `StatusUpdate`

Implemented and wired M5 skill progression/cast server packets in Go:

- `AcquireSkillList`
- `AcquireSkillInfo`
- `AcquireSkillDone`
- `MagicSkillUse`
- `MagicSkillLaunched`
- `SetupGauge`

Implemented and wired M5 shortcut server packets in Go:

- `ShortCutRegister`
- `ShortCutDelete`

## Notes

- Duplicate names across sections are intentional. For example `NpcHtmlMessage`, `HennaInfo`, `ExStorageMaxCount`, `Die`, `PlaySound`, `ShortCutInit`, and `SkillCoolTime` are required by more than one closed milestone surface.
- `StartRotation` was also classified into M4 during the movement/rotation pass; it is implemented and wired with `StartRotating`.
- `ChairSit` is wired for selected type-1 static objects within interaction range. It broadcasts `ChangeWaitType` before `ChairSit`, marks the chair busy while occupied, and releases it when the player stands or stops. Map signs, arena signs, and richer static-object interactions remain deferred.
- `Action` and `AttackRequest` now wire the target-selection/onAction subset needed for attacking mobs: first request selects and sends target HP status, second `AttackRequest` against the selected target emits `AutoAttackStart` then `Attack`; the existing attack-stance timeout emits `AutoAttackStop`. NPC dialog/interact routing remains deferred to the M7 NPC work, and skill/cast targeting remains deferred to M6.
- `RequestChangeMoveType`, `RequestChangeWaitType`, and `RequestSocialAction` now wire the current run/walk, sit/stand, and social-animation state available in Go. Missing higher-level gates such as mount state, fishing, requester/trade state, and full AI intention are still owned by the systems that introduce those states.
- `RequestDropItem`, `RequestDestroyItem`, and `SendTimeCheck` are wired. Drop/destroy currently cover the inventory/template/count gates available in Go and emit `InventoryUpdate`; player drops also place a ground item through the ground-item task and emit the animated `DropItem` frame during the transient dropper-id window.
- `RequestCrystallizeItem` is wired for restored runtime-known Crystallize skill levels, crystallizable item gates, crystal reward grants, `SystemMessage` feedback, and `InventoryUpdate`.
- `RequestAcquireSkillInfo` and `RequestAcquireSkill` are wired for usual class-template skill learning, learned-skill persistence, `SkillList` refresh, SP `StatusUpdate`, and success/failure `SystemMessage` feedback. Enchant, clan, fishing, transform, and special-trainer learning remain deferred to their owning systems.
- `RequestMagicSkillUse` is wired for known non-passive active skills with `SELF`, `NONE`, `GROUND`, or `ONE` targets, using the current cast controller for MP/HP/item/reuse validation and emitting `MagicSkillUse`, `SetupGauge`, `MagicSkillLaunched`, `SystemMessage`, `ActionFailed`, and MP/HP `StatusUpdate` where applicable. Full AI intention scheduling, delayed cast timers, target-handler integration, effect/skill-handler application, toggles, fusion/signet/chance skills, item-triggered casts, summon/pet casts, and the `MagicSkillCanceled` send path remain deferred to the M6 cast/effect runtime.
- `RequestEnchantItem` is wired through the enchant-scroll `UseItem` path, `ChooseInventoryItem`, scroll ownership/count validation, item enchantability/grade/type gates, scroll consumption, item enchant persistence, blessed reset, normal break/crystal reward, `EnchantResult`, `InventoryUpdate`, `SystemMessage`, and self `UserInfo`. Config-file overrides for enchant rates, store/trade-state gates, and +4 dual/+6 armor-set passive skill side effects remain deferred to their owning systems.
- `RequestPetUseItem`, `RequestGiveItemToPet`, `RequestGetItemFromPet`, and `RequestPetGetItem` are wired for active-pet lookup, pet inventory transfer/equip mutation, immediate visible ground-item pickup, item persistence, `GetItem`, `DeleteObject`, `PetInventoryUpdate`, player `InventoryUpdate`, and pet-use `SystemMessage` feedback. Pet AI movement-to-pickup, drop-protection/looter gates, pet food/potion item handlers, player operating/transaction-state gates, and richer pet stat refreshes remain deferred to their owning systems.
- `RequestShortCutReg` and `RequestShortCutDel` are wired for persisted client shortcut entries, including starter shortcut creation, EnterWorld restoration, `ShortCutRegister`, and `ShortCutDelete`. Item reuse timers, macro bodies, recipe validation, and soulshot auto-use side effects remain deferred to their owning systems.
- `RequestChangePetName` remains deferred because the Go runtime has no active pet naming state, pet-name uniqueness query, NPC-name lookup by name, or control-item custom type update flow.
- `RequestPledgeCrest` is wired against the loaded small pledge crest `.dds` cache and emits `PledgeCrest`. `RequestAllyCrest` is wired in game against loaded ally crest `.dds` cache data and emits `AllyCrest` only when data exists. Crest upload/update packets and large pledge crests remain deferred to the clan/crest write-owner flows.
- `RequestCursedWeaponList` loads cursed weapon definitions at gameserver boot and emits `ExCursedWeaponList`. `RequestCursedWeaponLocation` is accepted but currently sends nothing because the Go runtime has no active cursed-weapon spawn/activation state yet.
- `PetItemList`, `PetInfo`, and `PetStatusUpdate` remain deferred together: the current Go pet actor lacks the full pet info/status snapshot surface and owner spawn/info broadcast path needed to emit truthful full-list/status packets. `PetStatusShow` and `PetDelete` are wired where the current runtime has exact backing behavior.
- `Revive`, `MagicSkillCanceled`, `AbnormalStatusUpdate`, `ShortBuffStatusUpdate`, and `ExUseSharedGroupItem` have byte-layout encoders with focused tests. Their send paths remain deferred until resurrect, cast-cancel, effect-list, short-buff, and shared-item reuse systems produce truthful runtime state.
- `RequestAutoSoulShot` is wired as extended client opcode `0x0005` with per-player auto-shot toggle state, `ExAutoSoulShot`, and item-name `SystemMessage` feedback. First-shot recharge, recurring shot consumption, and `ExUseSharedGroupItem` reuse display remain deferred to the item-use/handler burst because the shared item handler/reuse pipeline is not ported yet.
- `StatusUpdate` is implemented and wired for target max/current HP during selection. Broader status/stat recalculation broadcasts still need owner flows as those systems are ported.
- The last full unique missing-count pass deduplicated those overlaps as 48 missing game client packets and 63 missing game server packets after the EnterWorld, movement/rotation, ValidateLocation correction, ChairSit, inventory, target/action, stance/social, item-operation, auto-shot, skill-acquisition, basic skill-cast, enchant, backed pet inventory/status, linked-html, pledge-crest, ally-crest, and cursed-weapon-list packet-wiring passes. Recompute this count during the next full packet audit; this M5 burst only removes encoder gaps and leaves runtime send-path gaps tracked above.
- Existing Go code accepts several M4/M5 client opcodes in `clientpackets/wiresafe.go`, but many of them still log "Opcode not wired" or have no decode/run implementation.
- This audit uses original Java class names. Go may keep a slightly different helper shape, such as `Frame...` functions instead of packet structs, but the required client-visible packet behavior is still one original packet at a time.
