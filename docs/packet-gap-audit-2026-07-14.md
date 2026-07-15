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
- Classified M2-M5 required game client packets: 93. Missing in Go: 60.
- Classified M2-M5 required game server packets: 128. Missing in Go: 75.
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

- `RequestLinkHtml`
- `RequestBypassToServer`
- `RequestBBSwrite`
- `RequestPledgeCrest`
- `RequestSetPledgeCrest`
- `RequestAllyCrest`
- `RequestSetAllyCrest`
- `RequestExPledgeCrestLarge`
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
- `RequestCursedWeaponList`
- `RequestCursedWeaponLocation`
- `RequestConfirmTargetItem`
- `RequestConfirmRefinerItem`
- `RequestConfirmGemStone`
- `RequestConfirmCancelItem`

`RequestBuyItem` and `RequestSellItem` currently have Go decoders and byte-layout tests only. They
are still counted as gaps because merchant NPC context, buylist loading/restock wiring, buy/sell
inventory mutation, adena persistence, and the NPC dialog/bypass owner flow are not implemented.

Implemented and wired M4 movement/rotation/target client packets in Go:

- `CannotMoveAnymore`
- `RequestTargetCancel`
- `StartRotating`
- `FinishRotating`

Missing M4 world/movement client packets:

- `RequestMoveToLocationInVehicle`
- `CannotMoveAnymoreInVehicle`
- `RequestGetOnVehicle`
- `RequestGetOffVehicle`
- `Appearing`
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
- `SendTimeCheck`

Missing M5 stats/combat/items/progression client packets:

- `RequestPetUseItem`
- `RequestGiveItemToPet`
- `RequestGetItemFromPet`
- `RequestPetGetItem`
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

- `PledgeShowMemberListUpdate` ([#631](https://github.com/fatal10110/acis_golang/issues/631))
- `PledgeShowMemberListAll` ([#632](https://github.com/fatal10110/acis_golang/issues/632))
- `PledgeSkillList` ([#717](https://github.com/fatal10110/acis_golang/issues/717))
- `ExMailArrived` ([#718](https://github.com/fatal10110/acis_golang/issues/718))
- `PlaySound` ([#719](https://github.com/fatal10110/acis_golang/issues/719))
- `NpcHtmlMessage` ([#720](https://github.com/fatal10110/acis_golang/issues/720))

`PledgeSkillList`, `ExMailArrived`, `PlaySound`, `NpcHtmlMessage`, `BuyList`, and `SellList`
currently have Go frame builders only. They are still counted as gaps because no production owner
flow can emit them truthfully yet.

Missing M3 data/UI server packets:

- `NpcHtmlMessage`
- `PledgeCrest`
- `AllyCrest`
- `ExPledgeCrestLarge`
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
- `ExCursedWeaponList`
- `ExCursedWeaponLocation`
- `ExShowVariationMakeWindow`
- `ExShowVariationCancelWindow`
- `ExConfirmVariationItem`
- `ExConfirmVariationRefiner`
- `ExConfirmVariationGemstone`
- `ExVariationResult`
- `ExVariationCancelResult`

Implemented and wired M4 movement/rotation server packets in Go:

- `StopMove`
- `StartRotation`
- `StopRotation`

Missing M4 world/movement server packets:

- `ValidateLocation`
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
- `ChairSit`

Missing M5 stats/combat/items/progression server packets:

- `PlaySound`
- `PetInventoryUpdate`
- `PetItemList`
- `PetStatusShow`
- `PetInfo`
- `PetStatusUpdate`
- `PetDelete`
- `Revive`
- `MagicSkillCanceled`
- `AbnormalStatusUpdate`
- `ShortBuffStatusUpdate`
- `ShortCutRegister`
- `ShortCutDelete`
- `ExUseSharedGroupItem`
- `WarehouseDepositList`
- `WarehouseWithdrawList`
- `PackageToList`
- `PackageSendableList`

Implemented and wired M5 item server packets in Go:

- `ChooseInventoryItem`
- `DropItem`
- `EnchantResult`
- `ExAutoSoulShot`
- `InventoryUpdate`
- `PackageSendableList`

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

## Notes

- Duplicate names across sections are intentional. For example `NpcHtmlMessage`, `HennaInfo`, `ExStorageMaxCount`, `Die`, `PlaySound`, `ShortCutInit`, and `SkillCoolTime` are required by more than one closed milestone surface.
- `StartRotation` was also classified into M4 during the movement/rotation pass; it is implemented and wired with `StartRotating`.
- `Action` and `AttackRequest` now wire the target-selection/onAction subset needed for attacking mobs: first request selects and sends target HP status, second `AttackRequest` against the selected target emits `AutoAttackStart` then `Attack`; the existing attack-stance timeout emits `AutoAttackStop`. NPC dialog/interact routing remains deferred to the M7 NPC work, and skill/cast targeting remains deferred to M6.
- `RequestChangeMoveType`, `RequestChangeWaitType`, and `RequestSocialAction` now wire the current run/walk, sit/stand, and social-animation state available in Go. Missing higher-level gates such as mount state, fishing, requester/trade state, and full AI intention are still owned by the systems that introduce those states.
- `RequestDropItem`, `RequestDestroyItem`, and `SendTimeCheck` are wired. Drop/destroy currently cover the inventory/template/count gates available in Go and emit `InventoryUpdate`; player drops also place a ground item through the ground-item task and emit the animated `DropItem` frame during the transient dropper-id window.
- `RequestCrystallizeItem` is wired for restored runtime-known Crystallize skill levels, crystallizable item gates, crystal reward grants, `SystemMessage` feedback, and `InventoryUpdate`.
- `RequestAcquireSkillInfo` and `RequestAcquireSkill` are wired for usual class-template skill learning, learned-skill persistence, `SkillList` refresh, SP `StatusUpdate`, and success/failure `SystemMessage` feedback. Enchant, clan, fishing, transform, and special-trainer learning remain deferred to their owning systems.
- `RequestMagicSkillUse` is wired for known non-passive active skills with `SELF`, `NONE`, `GROUND`, or `ONE` targets, using the current cast controller for MP/HP/item/reuse validation and emitting `MagicSkillUse`, `SetupGauge`, `MagicSkillLaunched`, `SystemMessage`, `ActionFailed`, and MP/HP `StatusUpdate` where applicable. Full AI intention scheduling, delayed cast timers, target-handler integration, effect/skill-handler application, toggles, fusion/signet/chance skills, item-triggered casts, summon/pet casts, and `MagicSkillCanceled` remain deferred to the M6 cast/effect runtime.
- `RequestEnchantItem` is wired through the enchant-scroll `UseItem` path, `ChooseInventoryItem`, scroll ownership/count validation, item enchantability/grade/type gates, scroll consumption, item enchant persistence, blessed reset, normal break/crystal reward, `EnchantResult`, `InventoryUpdate`, `SystemMessage`, and self `UserInfo`. Config-file overrides for enchant rates, store/trade-state gates, and +4 dual/+6 armor-set passive skill side effects remain deferred to their owning systems.
- `RequestAutoSoulShot` is wired as extended client opcode `0x0005` with per-player auto-shot toggle state, `ExAutoSoulShot`, and item-name `SystemMessage` feedback. First-shot recharge, recurring shot consumption, and `ExUseSharedGroupItem` reuse display remain deferred to the item-use/handler burst because the shared item handler/reuse pipeline is not ported yet.
- `StatusUpdate` is implemented and wired for target max/current HP during selection. Broader status/stat recalculation broadcasts still need owner flows as those systems are ported.
- The unique missing counts deduplicate those overlaps: 58 missing game client packets and 72 missing game server packets after the EnterWorld, movement/rotation, inventory, target/action, stance/social, item-operation, auto-shot, skill-acquisition, basic skill-cast, and enchant packet-wiring passes.
- Existing Go code accepts several M4/M5 client opcodes in `clientpackets/wiresafe.go`, but many of them still log "Opcode not wired" or have no decode/run implementation.
- This audit uses original Java class names. Go may keep a slightly different helper shape, such as `Frame...` functions instead of packet structs, but the required client-visible packet behavior is still one original packet at a time.
