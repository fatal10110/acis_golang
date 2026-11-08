"""#258 parity-sweep slicer (v5): docs/parity-sweep/slicer.py.

Assigns every production .go file under internal/ and cmd/ to exactly one parity slice, routes the
thin GameClientLink handle* methods that live in connection-core files to the feature slice whose
code they call, and lists read-only context files per slice.

Rules: slice IDs are frozen in RULES. Never renumber; a new slice gets the next free ID (P82, ...).
Order of decision per file: test-helper dirs -> OVERRIDES -> first matching RULES pattern -> late ('~')
catch-all patterns -> TOOLING.
The script exits non-zero on unassigned files, dead rules, bad or stale overrides, and handler routing ties.

Usage (from the outer acis_public/ root): python3 <go-root>/docs/parity-sweep/slicer.py <go-root> <out.json>
The output is deterministic: the same tree always gives byte-identical JSON.
"""
import collections, fnmatch, json, os, re, subprocess, sys

N = 'internal/gameserver/network/'
G = 'internal/gameserver/'
A = G + 'model/actor/'
P = A + 'player/'
S = G + 'data/sql/'
C = 'cmd/gameserver/'

def net(*n): return [N + x for x in n]
def sql(*n): return [S + x + '.go' for x in n]
def ply(*n): return [P + 'character_' + x + '.go' for x in n]
def cmd(*n): return [C + x + '.go' for x in n]

# Test-support directories that hold non-_test.go helpers: tooling, never a slice.
TEST_HELPER_DIRS = {'worldtest', 'targettest', 'attackabletest', 'effecttest', 'skilltest', 'sqltest', 'testdata'}
TOOLING = ['internal/botclient/', 'internal/testsupport/', 'internal/gameservertest/', 'internal/dbtest/',
           'internal/datadiff/', 'internal/provenance/', 'cmd/botswarm/', 'cmd/datadiff/', 'cmd/packetdiff/',
           'cmd/pageindex/', 'cmd/scriptfp/', 'cmd/testtiming/']

# Explicit file -> slice overrides (reviewed misassignments by name).
OVERRIDES = {
    N + 'session.go': 'P05',            # game-connection cipher session, not the LS link
    N + 'petition.go': 'P43',           # would match pet*.go
    N + 'announcements.go': 'P43',
    N + 'herb.go': 'P36', N + 'consumable.go': 'P36',
    N + 'chance.go': 'P17',             # chance-cast delivery
    N + 'taskeffects.go': 'P80',        # autosave, zone membership ticks, item expiry
    N + 'scheduled_tasks.go': 'P80',    # castle tax, Seven Signs period saves
    G + 'data/manager/npcs_hostile.go': 'P21',
    G + 'script/ai/warrior/base.go': 'P62',
    G + 'skill/persistence.go': 'P09',          # buff and skill-reuse save/restore
    G + 'skill/acquisition.go': 'P18', G + 'skill/enchant.go': 'P18', G + 'skill/leveling.go': 'P18',
    G + 'inventory/warehouse.go': 'P38', G + 'inventory/sell.go': 'P39', G + 'inventory/death_drop.go': 'P22',
    G + 'data/manager/npcs_mercenary.go': 'P45', G + 'data/manager/npcs_tower.go': 'P45',
    G + 'data/manager/npcs_artifact.go': 'P44',
    G + 'script/ai/individual/royal_rush_npc.go': 'P65', G + 'script/ai/individual/royal_rush_mission_npc.go': 'P65',
    N + 'classmaster.go': 'P51', N + 'academy_graduation.go': 'P41',
    P + 'character_recommendation.go': 'P28', P + 'character_cast.go': 'P17', P + 'character_cast_gate.go': 'P17',
    G + 'data/manager/npcs_cc_rights.go': 'P22', P + 'reward.go': 'P22', P + 'party_reward.go': 'P22',
    G + 'skill/catalog.go': 'P09', G + 'skill/transient.go': 'P09',
    N + 'gm_command.go': 'P57', N + 'gmlist.go': 'P57',
    P + 'character_revive_request.go': 'P28', N + 'resurrection_scroll.go': 'P36',
    A + 'creature/duel.go': 'P50',
    P + 'character_target.go': 'P19', P + 'character_target_actor.go': 'P19', P + 'character_intention.go': 'P20',
    N + 'pagan_keys.go': 'P36', N + 'hostile_talk.go': 'P23', G + 'model/entity/cursedweapon.go': 'P51',
    A + 'creature/cubic_formula.go': 'P29', G + 'task/attackstance.go': 'P16', G + 'task/pvpflag.go': 'P16',
    N + 'posture_settled.go': 'P19', N + 'punishment.go': 'P57', N + 'access_level.go': 'P57',
    N + 'adena.go': 'P36', N + 'belongings.go': 'P36',
}

# (id, area, title, java_start, evidence, [patterns], [cmd/gameserver context files])
RULES = [
# ---------------- A0 foundation ----------------
('P01','A0','Commons: wire, crypt, netutil, ids, scheduler, rnd, hashorder','net.sf.l2j.commons.* (mmocore, crypt, random, pool, data)','commons oracle tests',
 ['internal/commons/'],[]),
('P02','A0','Config loading & defaults','net.sf.l2j.Config + config/*.properties','internal/config cases, config keys reference test',
 ['internal/config/'],cmd('config','config_keys')),
('P03','A0','Login server: auth, crypt, server list, accounts','net.sf.l2j.loginserver.*','internal/loginserver tests',
 ['internal/loginserver/','cmd/loginserver/','cmd/accountmgr/','cmd/gsregister/'],[]),
('P04','A0','GS<->LS link, session keys, login status','gameserver.LoginServerThread, loginserver.GameServerThread','tests/lifecycle',
 ['internal/link/']+net('loginlink.go','loginstatus.go','sessionvalidator.go','session_addr.go'),[]),
('P05','A0','Game connection core: cipher, framing, client loop, opcode gate, state gates, flood protection, GameClientLink','network.GameClient, GamePacketHandler, FloodProtectors, mmocore','decodefuzz, tests/lifecycle',
 [N+'cipher/']+net('conn.go','client.go','client_loop.go','client_preauth.go','client_dispatch.go','client_ingame.go','client_extended.go','client_connstate.go','clientregistry.go','clientstats.go','dispatch.go','state.go','frame_reject.go','floodprotector.go','server.go','broadcast.go','doc.go','config_gates.go'),cmd('network','infra')),
('P06','A0','Character select/create/delete, enter world, logout/restart, exit guard','CharacterCreate/Select/Delete, EnterWorld, Logout, RequestRestart','tests/lifecycle, tests/character',
 net('character_flow.go','character_flow_feature.go','character_select.go','restart_selection.go','lifecycle.go','exitguard.go','disconnect_delay.go','left_owner_settle.go','gm_login.go','access_level.go','punishment.go','playerclock.go','live_player.go','live_player_events.go')+[G+'model/restart/']+ply('punishment'),net('sessionvalidator.go')),
# ---------------- A1 data & persistence ----------------
('P07','A1','XML loaders: items, skills, skill trees, effect/condition grammar','data.xml ItemData/SkillTable/SkillTreeData + DocumentItem/DocumentSkill','data/xml oracle+parity tests; cmd/datadiff internal/datadiff',
 [G+'data/xml/'+f for f in ('item.go','skill.go','skilltree.go','effectgrammar.go','condattrs.go','attrvalues.go','templatedecode.go','templatenodes.go','files.go','doc.go','tables.go')],[]),
('P08','A1','XML loaders: NPCs, spawnlist, class templates, levels, zones, doors, residences, economy, routes','data.xml NpcData, SpawnData, PlayerData, ZoneData, DoorData, CastleData, MultisellData ...','data/xml tests; cmd/datadiff internal/datadiff',
 [G+'data/xml/*'],cmd('data')),
('P09','A1','SQL: character persistence (character, items, skills, quests, shortcuts, macros, subclass, memo, buff/reuse save)','Player.store/restore, ItemInstance updates, character DAOs','data/sql sqltest, tests/character, schema drift test',
 sql('character','character_access','character_edit','character_lookup','character_punishment','characterskills','item','itemflush','skillsave','macro','shortcut','subclass','quest','memo','bookmark','doc'),[]),
('P10','A1','NPC manager: spawn/respawn/makers/territories, world objects, HTML and crest cache','SpawnManager, Spawn, NpcMaker/territory, HtmCache, CrestCache','tests/npcs',
 [G+'data/manager/'+f for f in ('npcs_spawn*.go','npcs_respawn.go','npcs_despawn.go','npcs_makers.go','npcs_maker_ops.go','npcs_maker_panel.go','npcs_territory.go','npcs_nospawns.go','npcs_script_*.go','npcs_slot_names.go','spawns.go','roster.go','worldobjects.go','door_watch.go','doc.go')]+[G+'data/cache/',G+'model/spawn/']+sql('spawn'),[]),
# ---------------- A2 world ----------------
('P11','A2','Geodata: block formats, reader, engine (LOS, height, canMove)','geoengine.GeoEngine + geodata blocks','geo engine oracle',
 [G+'geo/*',G+'geo/block/',G+'geo/reader/',G+'geo/engine/'],[]),
('P12','A2','Pathfinding, dynamic geo (doors/fences), geo tools','geoengine.pathfinding, door/fence geo objects','geo pathfind and probe tests (need ACIS_GEODATA_DIR; local-only per #4426)',
 [G+'geo/','cmd/geopack/','cmd/geoprobe/']+net('admin_geo.go'),[]),
('P13','A2','World grid, regions, visibility/known lists, geometry','model.World, WorldRegion, known lists, location','tests/npcs, world tests',
 [G+'world/',G+'model/geometry/',G+'model/location/',G+'model/entity/',G+'model/observer/']+net('visibility.go','npc_radius_broadcast.go','observer.go','invisible.go','group_screen_message.go')+ply('observer','invisible'),cmd('world','observer')),
('P14','A2','Zones & zone pulse','model.zone.* (ZoneType subclasses), ZoneManager','zone tests, tests/combat',
 [G+'model/zone/',G+'zonepulse/']+net('zone_*.go','arena_zone.go','mothertree_zone.go'),[]),
('P15','A2','Movement, follow, teleport, routes, validate position','Creature.move*, MoveToLocation, ValidatePosition, TeleportToLocation','tests/character, move tests',
 [A+'move/',G+'model/route/',G+'model/travel/']+net('movement.go','player_follow.go','move_debug.go','instant_teleport.go','recall.go')+ply('fall','flight','instant_teleport','recall'),[]),
# ---------------- A3 actors ----------------
('P16','A3','Creature core, physical attack, attack stance, PvP/karma flags','model.actor.Creature, attack.CreatureAttack, PlayerAttack','tests/combat',
 [A+'*',A+'creature/',A+'attack/',A+'event/']+net('attack*.go','combat.go','pk_karma.go','hostile_talk.go','spawn_protection.go'),[]),
('P17','A3','Skill casting controller, chance casts','model.actor.cast.* (CreatureCast, PlayerCast)','tests/skills, cast tests',
 [A+'cast/'],[]),
('P18','A3','Cast request packets, item-skill casts, skill acquisition/enchant','RequestMagicSkillUse, UseItem->skill, RequestAcquireSkill, RequestExEnchantSkill','tests/skills',
 net('magic_skill.go','item_skill_cast.go','target_cast_item.go','skill_changes.go','skill_acquisition.go','skill_enchant.go','trainer.go','clan_skills.go','siege_skills.go'),[]),
('P19','A3','Targeting & target handlers, sit/stand','RequestAction/RequestTargetCancel, handler.targethandlers.*','tests/skills, handler/target tests',
 net('targeting.go')+[G+'handler/target/'],[]),
('P20','A3','AI core & intentions','ai.CreatureAI/PlayableAI/AttackableAI, desire queue','tests/ai',
 [A+'ai/']+net('live_intention*.go','use_item_intention.go'),[]),
('P21','A3','Monsters: attackable/hostile model, aggro, hate, movement','Attackable, Monster, aggro/hate list, RaidBoss/GrandBoss status','tests/npcs, tests/combat',
 [A+'attackable/',A+'npc/hostile*.go'],[]),
('P22','A3','Kill rewards: exp/sp, drops, overhit, absorb, seeds, party loot, death drop','Attackable.calculateRewards/doItemDrop, DropData, party loot, seed harvest','tests/npcs, tests/party',
 [A+'npc/overhit.go',A+'npc/absorb.go',A+'npc/raid_multipliers.go',A+'npc/seed_state.go',G+'data/manager/npcs_rewards.go',G+'data/manager/npcs_droprates.go',G+'data/manager/npcs_party_rewards.go',G+'data/manager/killreward*.go']+net('death_drop.go','party_loot*.go','loot_channels.go','party_rewards.go','pickup.go','npc_pickup.go')+ply('loot','death_drop'),cmd('config_death_drop')),
('P23','A3','NPC folk: bypass, dialogs, HTML, villagemaster, chat','Folk, Npc.onBypassFeedback, NpcHtmlMessage, VillageMaster','tests/npcs',
 [A+'npc/folk_bypass.go',A+'npc/folk_chat*.go',A+'npc/folk_villagemaster.go',A+'npc/folk_residence_*.go',A+'npc/folk_signs_bypass.go',A+'npc/folk_festival_bypass.go',A+'npc/dialog_command.go',A+'npc/folk_merchant.go',A+'npc/folk_sell.go',A+'npc/folk_trainer.go',A+'npc/folk_warehouse.go',A+'npc/folk_dungeon.go',A+'npc/folk_observer.go',A+'npc/folk_derby.go',A+'npc/folk_olympiad.go',A+'npc/folk_hallmanager.go',A+'npc/folk_chamberlain.go',A+'npc/folk_instant_teleport.go',A+'npc/folk_tower.go',A+'npc/folk_siegeflag.go',A+'npc/folk.go',A+'npc/folk_script_ops.go']+net('bypass.go','folk.go','npc_*.go','html*.go','dlganswer.go','dungeon_gatekeeper.go','door_interact.go','tutorial.go'),[]),
('P24','A3','NPC folk combat & stats: fight, cast, HP/regen, walk, desires, death','Folk/Guard combat, NpcStatus, NPC AI hooks','tests/npcs, tests/ai',
 [A+'npc/folk_*.go'],[]),
('P25','A3','NPC core: template, instance, decoration, effect points, script hooks, NPC manager rest','NpcTemplate, Npc, NpcInfo, NPC script events','tests/npcs',
 ['~'+A+'npc/',A+'npcinfo/','~'+G+'data/manager/'],[]),
('P26','A3','Player model part 1: stats, attack, runtime, state, vitals, death/exp/karma','Player stats/status/attack, death penalty, karma','tests/character, tests/combat',
 [P+f for f in ('character_stats.go','character_attack*.go','character_runtime.go','character_state.go','character_vitals.go','character_death*.go','character_karma.go','character_pvpflag.go','character_regen*.go','character_max*.go','character_*penalty.go','character_charges.go','character_autoshot.go','character_dotnotice.go','character_raid_curse.go','character_effects.go','character_effect_actor.go','character_conditions.go')],[]),
('P27','A3','Player model part 2: progression, classes, subclass model, skills, remaining player state','Player level/exp/sp, ClassId, subclasses, skill use','tests/character',
 ['~'+P],[]),
('P28','A3','Player commands & UI: subclass requests, shortcuts, macros, chat, user commands, resurrection, restart point, recommendations','user commands, Say2 + chat handlers, RequestShortCutReg, RequestMakeMacro, RequestRestartPoint, RequestEvaluate','tests/character, tests/social',
 [G+'handler/chat/',G+'handler/user/',G+'model/bookmark/',G+'model/macro/',G+'model/shortcut/']+net('subclass.go','player_rules.go','shortcut.go','macro.go','recommendation.go','occupation.go','belongings.go','adena.go','posture_settled.go','restartpoint.go','usercommand*.go','chat.go','pagan_keys.go')+sql('recommendation'),cmd('config_chat')),
('P29','A3','Summons & pets model: formula, live state, lifecycle, commands, cubics','Summon, Pet, Servitor, BabyPet, Cubic, PetData','tests/pets',
 [A+'summon/',A+'pet/',A+'cubic/']+sql('pet')+ply('summon*','servitor','cubic'),[]),
('P30','A3','Summon/pet network: spawn, events, items, mounts, wyvern','RequestActionUse (pet), summon items, Mount/Ride, wyvern','tests/pets',
 net('summon*.go','pet_*.go','pet.go','mount*.go','wyvern*.go','cubic.go')+[G+'petitem/']+ply('mount*'),cmd('wyvern')),
# ---------------- A4 skills ----------------
('P31','A4','Skill model, conditions, stat funcs, stat bonus','skills.L2Skill, conditions, Func*, BaseStats','skill tests, tests/skills',
 [G+'model/skill/',G+'skill/conditions/',G+'skill/funcs/',G+'skill/stat/',G+'skill/statbonus/',G+'skill/def/'],[]),
('P32','A4','Formulas: damage, crit, hit, resist, regen','skills.Formulas','skill/formulas oracle vectors (confirm they came from the Java oracle)',
 [G+'skill/formulas/',G+'skill/*'],[]),
('P33','A4','Effects','skills.effects.Effect*','skill/effect tests, tests/skills',
 [G+'skill/effect/'],[]),
('P34','A4','Skill handlers','handler.skillhandlers.*','handler/skill tests, tests/skills',
 [G+'handler/skill/'],[]),
# ---------------- A5 items ----------------
('P35','A5','Item model & containers: inventory, warehouse, freight, ground items','model.item.*, model.itemcontainer.*','tests/items',
 [G+'model/item/',G+'model/itemcontainer/',G+'model/grounditem/',G+'model/armorset/']+sql('grounditem'),[]),
('P36','A5','Inventory ops: equip/drop/destroy/crystallize, use item, shots, herbs, potions, delivery','UseItem, RequestDropItem, RequestDestroyItem, RequestCrystallizeItem, item handlers','tests/items',
 [G+'inventory/',G+'handler/item/',G+'handler/*']+net('inventory*.go','item_*.go','manor_item.go')+ply('equipped','inventory_limit','herb','shortbuff'),[]),
('P37','A5','Enchant, augment, craft/recipes, henna/symbol maker','RequestEnchantItem, augmentation, RecipeController, henna equip','tests/items',
 [G+'enchant/',G+'augment/',G+'model/augmentation/',G+'craft/',G+'model/recipe/',G+'model/henna/',G+'symbolmaker/']+net('enchant.go','augment.go','recipe*.go','henna.go')+sql('augmentation','henna','recipebook')+ply('henna','recipebook'),[]),
('P38','A5','Trade, private stores, exchange, warehouse','TradeList, TradeRequest, PrivateStore*, warehouse deposit/withdraw','tests/trade, tests/items',
 [G+'trade/',G+'privatestore/',G+'exchange/']+net('trade.go','privatestore*.go','warehouse.go','clan_warehouse.go')+ply('privatestore','script_warehouse'),[]),
('P39','A5','Merchants, buylists, multisell, sell','BuyList, Multisell, RequestSellItem','tests/items',
 [G+'merchant/',G+'model/buylist/',G+'model/multisell/']+net('merchant.go','multisell.go','sell.go')+sql('buylist'),[]),
# ---------------- A6 social ----------------
('P40','A6','Party, command channel, party matching','model.group.Party/CommandChannel, PartyMatchRoom','tests/party',
 [G+'party/',G+'partymatch/']+net('party*.go')+ply('party','partybars','partyroom'),[]),
('P41','A6','Clan core: create/dissolve, members, ranks, crest, academy, subunits','model.pledge.Clan, ClanTable','tests/clan',
 [G+'clan/']+net('clan.go','clan_view.go','clan_subunit.go','clan_dissolution.go','clan_leader_transfer.go','clan_title.go','clan_crest.go','clan_villagemaster.go','clan_leave_items.go','crest.go')+sql('clan','clan_board','clan_dissolution')+ply('clan','title'),cmd('clan')),
('P42','A6','Alliance, clan wars, friends/relations, block list','alliance + clan wars in ClanTable, friend list, block list','tests/clan, tests/social',
 [G+'social/relation/']+net('alliance.go','clan_war*.go','friends.go','relations.go','social_graph.go','clan_gate.go')+sql('relation','clan_subunit_war')+ply('clan_war_death','relation','blockall'),cmd('social')),
('P43','A6','Community board, mail, petitions, wedding, announcements','communitybbs.*, PetitionManager, CoupleManager, AnnouncementData','tests/bbs, tests/petition, tests/wedding',
 [G+'bbs/',G+'social/petition/',G+'wedding/',G+'announcement/']+net('community_board*.go','wedding.go')+sql('bbs_forum','bbs_mail','petition','wedding')+ply('wedding'),cmd('announcements','board','config_petition','wedding')),
# ---------------- A7 residences ----------------
('P44','A7','Castles: chamberlain, taxes, owner, doors/gates, artifact, manor','model.residence.castle.*, CastleManorManager','tests/clan, castlemanor oracle',
 [G+'castle/',G+'castlemanor/',G+'model/residence/',G+'model/manor/',G+'model/door/']+net('castle*.go','residence*.go','admin_castle.go','admin_manor.go','admin_door.go')+sql('castle','manor')+ply('residence','manor'),cmd('castle','castlemanor','manor')+net('scheduled_tasks.go')),
('P45','A7','Sieges: registration, flags, guards, mercenaries, siege rules, siege guard AI','model.residence.castle.Siege, SiegeGuardManager, script.ai.siegeguards.* (16)','tests/clan, tests/ai',
 [G+'siege/',G+'script/ai/siegeguard/']+net('siege*.go','mercenary_ticket.go','admin_siege.go')+sql('siege')+ply('siege*'),cmd('siege')),
('P46','A7','Clan halls: ownership, auction, functions','model.residence.clanhall.*, ClanHallManager, auction','tests/clan',
 [G+'clanhall/']+net('clanhall*.go','auctioneer.go','admin_clanhall.go')+sql('clan_hall_auction','clan_hall_function','clan_hall_owner')+ply('clanhall'),cmd('clanhall')),
('P47','A7','Siegable clan halls (agit wars)','script.siegablehall.* (7) + ClanHallSiege','tests/clan',
 [G+'script/siegablehall/']+net('hall_siege.go')+sql('clan_hall_siege'),[]),
('P48','A7','Seven Signs, festival, signs priests','SevenSigns, SevenSignsFestival, SignsPriest','sevensigns tests, festival reference test',
 [G+'sevensigns/',G+'festival/',G+'signspriest/']+net('sevensigns*.go','festival*.go','signs_priest.go')+sql('sevensigns','festival')+ply('sevensigns','festival'),cmd('festival','signspriest')+net('scheduled_tasks.go')),
# ---------------- A8 events ----------------
('P49','A8','Olympiad & heroes','olympiad.*, HeroManager','tests/olympiad',
 [G+'olympiad/',G+'hero/']+net('olympiad*.go','hero*.go','admin_olympiad.go')+sql('olympiad','hero')+ply('olympiad*','noble'),cmd('olympiad','hero')),
('P50','A8','Duel, derby, lottery, fishing, fishing championship','DuelManager, DerbyTrackManager, LotteryManager, fishing, FishingChampionshipManager','tests/duel, tests/lottery, tests/fishchamp',
 [G+'duel/',G+'derby/',G+'lottery/',G+'fishing/',G+'fishchamp/',G+'model/fish/']+net('duel.go','derby.go','lottery.go','fishing*.go')+sql('derby','lottery','fishchamp')+ply('duel','fishing'),cmd('derby','lottery','fishchamp')),
('P51','A8','Boats, cursed weapons, raid points, scheme buffer, gatekeeper, fences, class master','BoatManager, CursedWeaponManager, RaidPointManager, BufferManager, FenceManager','tests/boat',
 [G+'boat/',G+'cursedweapon/',G+'raidpoint/',G+'schemebuffer/',G+'gatekeeper/',G+'fence/',G+'classmaster/',G+'model/staticobject/']+net('boat*.go','cursed_weapon.go','raidpoint.go','schemebuffer.go')+sql('cursedweapon','raidpoint','bufferscheme')+ply('boat','cursed_weapon'),cmd('cursedweapons','raidpoints','schemebuffer','classmaster')),
# ---------------- A9 packets & admin ----------------
('P52','A9','Server packets A (files a-h): wire layout','network.serverpackets.* (byte layout, field order)','serverpackets oracle',
 [N+'serverpackets/[a-h]*'],[]),
('P53','A9','Server packets B (files i-r): wire layout','network.serverpackets.*','serverpackets oracle',
 [N+'serverpackets/[i-r]*'],[]),
('P54','A9','Server packets C (files s-z): wire layout','network.serverpackets.*','serverpackets oracle',
 [N+'serverpackets/'],[]),
('P55','A9','Client packet decoders','network.clientpackets.* readImpl','decodefuzz, clientpackets tests',
 [N+'clientpackets/'],[]),
('P56','A9','Admin commands A: show/info/find/set/spawn','handler.admincommandhandlers.*','tests/admin',
 net('admin_show.go','admin_info*.go','admin_find.go','admin_set*.go','admin_spawn*.go','admin_maker.go','admin_summon.go','admin_knownlist.go','admin_target.go','admin_panel.go','admin_help.go','admin.go'),cmd('config_admin')),
('P57','A9','Admin commands B: the rest, reload, GM list and Alt+G, punishments, access levels','handler.admincommandhandlers.*','tests/admin',
 net('admin_*.go')+[G+'model/admin/',G+'handler/admin/']+ply('edit'),cmd('reload')),
# ---------------- A10 scripts ----------------
('P58','A10','Script engine core: registry, dispatch, handles, hooks, timers, schedule, memo, quest log, NpcString','scripting.Quest base runtime, script registration','internal/testsupport/scriptcontract internal/testsupport/scriptfp internal/testsupport/handleops',
 [G+'script/'+f for f in ('registry.go','dispatch.go','handle*.go','hooks.go','timers.go','schedule*.go','script.go','env.go','seams.go','payload.go','lifecycle.go','memo.go','global_memo.go','journal.go','event_rules.go','idle.go','doc.go','checks.go','text.go','page_text.go','dialog.go','behavior.go','attacked.go','see_creature.go','dump.go','admin_info.go')]+[G+'script/task/',G+'model/questlog/',G+'model/memo/',G+'model/npcstring/']+net('script*.go','quest*.go')+ply('script*','quests','class_script'),cmd('scripts','event_rules')),
('P59','A10','Script ops API: npc/player/world/spawn/clan/hall ops, cells','script-callable world operations (Quest helper methods)','internal/testsupport/scriptcontract internal/testsupport/handleops',
 [G+'script/*'],[]),
('P60','A10','Script catalog & makers','script.maker.* (43 classes), script registration','internal/testsupport/scriptfp tests/npcs',
 [G+'script/catalog/',G+'script/maker/'],cmd('scripts_handleops')),
('P61','A10','Grand boss AI','script.ai.boss.* (35 classes)','tests/ai',
 [G+'script/ai/boss/']+sql('bosszone'),[]),
('P62','A10','Warrior AI part 1: WarriorBase, party*, cast*','script.ai.individual.Monster.WarriorBase.* (base, party and cast families)','tests/ai',
 [G+'script/ai/warrior/party*',G+'script/ai/warrior/cast*'],[]),
('P63','A10','Warrior AI part 2: physical, royal, corpse, run, special','script.ai.individual.Monster.WarriorBase.*','tests/ai',
 [G+'script/ai/warrior/'],[]),
('P64','A10','Wizard AI','script.ai.individual.Monster.WizardBase.* (~70 classes)','tests/ai',
 [G+'script/ai/wizard/'],[]),
('P65','A10','Raid, hall, royal rush, agit warrior AI','script.ai.individual RaidBoss*, RoyalRushDefaultNpc, AgitWarrior, script.ai.siegablehall.*','tests/ai',
 [G+'script/ai/raid/',G+'script/ai/hall/',G+'script/ai/royalrush/',G+'script/ai/agitwarrior/'],[]),
('P66','A10','Guard, individual, monster behaviour, group, default AI','script.ai.individual.Guard, remaining script.ai.individual.Monster','tests/ai',
 [G+'script/ai/'],[]),
('P67','A10','Features & teleport scripts','script.feature.* (16), script.teleport.* (13)','tests/quest',
 [G+'script/feature/',G+'script/teleport/']+ply('tutorial'),[]),
('P68','A10','Quest triage: rank all quests, set each band\'s review set','quest.* (343 classes in scriptfp/reference.golden)','internal/gameserver/script/quest internal/testsupport/scriptfp internal/testsupport/scriptpages internal/testsupport/scriptcontract tests/quest (reference checkout required)',
 [],[]),
('P69','A10','Class-change chain A: secondclass/thirdclass memo + Q070-Q100','quest.Q070-Q100 and the shared class-change memo','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/secondclass',G+'script/quest/thirdclass']+[G+'script/quest/q%03d/'%i for i in range(70,101)],[]),
('P70','A10','Class-change chain B: Q211-Q222','quest.Q211-Q222 (second-class trials)','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/q%03d/'%i for i in range(211,223)],[]),
('P71','A10','Class-change chain C: Q223-Q233','quest.Q223-Q233 (second-class trials)','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/q%03d/'%i for i in range(223,234)],[]),
('P72','A10','Quests 000-069','quest.Q001-Q069','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/q0'],[]),
('P73','A10','Quests 101-199','quest.Q101-Q199','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/q1'],[]),
('P74','A10','Quests 200-210, 234-299','quest.Q2xx outside the class-change chain','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/q2'],[]),
('P75','A10','Quests 300-349','quest.Q300-Q349','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/q30',G+'script/quest/q31',G+'script/quest/q32',G+'script/quest/q33',G+'script/quest/q34'],[]),
('P76','A10','Quests 350-399','quest.Q350-Q399','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/q3'],[]),
('P77','A10','Quests 400-499','quest.Q400-Q499','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/q4'],[]),
('P78','A10','Quests 500-619','quest.Q500-Q619','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/q5',G+'script/quest/q60',G+'script/quest/q61'],[]),
('P79','A10','Quests 620-699 + quest package root','quest.Q620-Q699','internal/gameserver/script/quest tests/quest',
 [G+'script/quest/'],[]),
# ---------------- A11 lifecycle ----------------
('P80','A11','Scheduled tasks, autosave, persistence worker, shutdown, sim','TaskManager, *TaskManager, Shutdown, store cadence','tests/lifecycle',
 [G+'task/',G+'persist/',G+'shutdown/',G+'sim/'],cmd('tasks','shutdown')),
('P81','A11','Composition root & boot order (incl. TZ-dependent timers)','GameServer main boot sequence','cmd/gameserver tests',
 ['cmd/gameserver/',G+'*',G+'data/*',G+'model/*'],[]),
]

# Handler -> slice overrides where call-graph voting is wrong or finds no network callee.
HANDLER_OVERRIDES = {
    'handleRequestCursedWeaponList': 'P51', 'handleRequestDuelSurrender': 'P50',
    'handleRequestExitPartyMatchingWaitingRoom': 'P40', 'handleRequestManorList': 'P44',
    'handleRequestBuyProcure': 'P44', 'handleRequestSkillCoolTime': 'P17',
    'handleAction': 'P19', 'handleAttackRequest': 'P19', 'handleRequestChangeWaitType': 'P19',
    'handleRequestSocialAction': 'P15', 'handleRequestShowMiniMap': 'P36',
    'handleRequestItemList': 'P06', 'handleRequestSkillList': 'P06', 'handleRequestNewCharacter': 'P06',
    'handleAppearing': 'P28', 'handleRequestBypassToServer': 'P23',
    'handleStartRotating': 'P15', 'handleFinishRotating': 'P15', 'handleRequestAllyCrest': 'P42',
}

# Execution waves: foundations first, client-visible/abusable early, shared flows together.
WAVES = {
    1: ('Protocol foundation and session', [1, 4, 5, 6, 9, 52, 53, 54, 55]),
    2: ('Config, loaders, spawning, geo, world, zones', [2, 7, 8, 10, 11, 12, 13, 14]),
    3: ('Movement, items and their persistence', [15, 35, 36, 37, 38, 39, 80]),
    4: ('Combat core and player model', [16, 17, 18, 19, 20, 26, 27, 31, 32]),
    5: ('Effects, skill handlers, player commands, summons, login, boot', [3, 28, 29, 30, 33, 34, 81]),
    6: ('NPCs and the script engine', [21, 22, 23, 24, 25, 58, 59, 60]),
    7: ('Social, castles, sieges', [40, 41, 42, 43, 44, 45]),
    8: ('Halls, Seven Signs, events, admin', [46, 47, 48, 49, 50, 51, 56, 57, 65]),
    9: ('AI scripts, features, quest tooling triage', [61, 62, 63, 64, 66, 67, 68]),
    10: ('Quests part 1', [69, 70, 71, 72, 73, 74]),
    11: ('Quests part 2', [75, 76, 77, 78, 79]),
}

# Domain reviews. Weights per lens: architecture, design, smell, performance, bugs (H main, M check, L skim).
# Vertical domains run twice: run A (all lenses but bugs, any time) and run B (bugs, after the last wave
# holding one of its slices). split=True splits run A into A1 (architecture + design) and A2 (smell + performance).
# Cross-cutting D18/D19 run A once over the whole tree (design level), then a bugs run per wave over that
# wave's slices. D20 covers tooling, tests and ops, not the runtime tree.
DOMAINS = [
    # (id, title, slice numbers | 'cross-cutting', weights 'ADSPB', split, scope note)
    ('D1', 'Protocol & connection', [1, 4, 5, 52, 53, 54, 55], 'HHMHH', True, ''),
    ('D2', 'Login server & config', [2, 3], 'MMMLH', False, ''),
    ('D3', 'Session, persistence, DB schema & boot', [6, 9, 80, 81], 'HMMMH', False,
     'Also aCis_datapack/sql (65 files) vs internal/gameserver/data/sql/sqltest schemas and schema_drift_test; container time zone (Seven Signs, Olympiad and siege timers use local wall-clock time).'),
    ('D4', 'Data loading & content tables', [7, 8, 10], 'MMHMM', False, ''),
    ('D5', 'World, geo, zones, movement', [11, 12, 13, 14, 15], 'MMLHH', False, ''),
    ('D6', 'Combat core & AI', [16, 17, 18, 19, 20], 'HHMHH', True, ''),
    ('D7', 'Skills, effects, formulas', [31, 32, 33, 34], 'MMMMH', False, ''),
    ('D8', 'NPCs & monsters', [21, 22, 23, 24, 25], 'HHHHM', True, ''),
    ('D9', 'Player & summons', [26, 27, 28, 29, 30], 'MHHMH', False, ''),
    ('D10', 'Items & economy', [35, 36, 37, 38, 39], 'MMMLH', False, ''),
    ('D11', 'Social & groups', [40, 41, 42, 43], 'MMMLM', False, ''),
    ('D12', 'Residences, sieges, Seven Signs', [44, 45, 46, 47, 48], 'MMMMH', False,
     'Timed state machines across restart and container time zone.'),
    ('D13', 'Events & minigames', [49, 50, 51], 'MMMLH', False, ''),
    ('D14', 'Script engine', [58, 59, 60], 'HHMMM', False, ''),
    ('D15', 'AI scripts', [61, 62, 63, 64, 65, 66], 'MMHHM', False, ''),
    ('D16', 'Quest & feature content', list(range(67, 80)), 'LMHLM', False, ''),
    ('D17', 'Admin & GM', [56, 57], 'LMHLH', False, ''),
    ('D18', 'Concurrency & goroutine ownership', 'cross-cutting', 'HHMHH', False,
     'Races, lock order, goroutine ownership. Run A: the whole runtime tree at design level (docs/concurrency-refactor-plan.md). Bugs runs: one per wave over that wave\'s slices.'),
    ('D19', 'Security & abuse surface', 'cross-cutting', 'MMLLH', False,
     'Bypass and HTML command input, flood protection, admin access levels, login auth, opcode state gates. Run A: whole tree at design level. Bugs runs: one per wave.'),
    ('D20', 'Tooling, tests, ops, observability', 'cross-cutting', 'MMHMH', False,
     'Scope: tooling_not_runtime files, tests/, ops/, docs/ops, docs/observability.md, .github/workflows. Not the runtime tree.'),
]
LENS_NAMES = ['architecture', 'design', 'smell', 'performance', 'bugs']

CORE_CONTEXT = [N + 'state.go', N + 'client_dispatch.go', N + 'dispatch.go']
DEF_RE = re.compile(r'^func (?:\(\s*\w+\s+\*?(\w+)\s*\)\s*)?(\w+)\s*[\[(]', re.M)


def main():
    root = sys.argv[1] if len(sys.argv) > 1 else '.'
    out = os.path.abspath(sys.argv[2] if len(sys.argv) > 2 else 'manifest.json')
    os.chdir(root)

    def walk(test):
        res = []
        for top in ('internal', 'cmd'):
            for r, _, fs in os.walk(top):
                if 'testdata' in r.split('/'):
                    continue
                res += [os.path.join(r, f) for f in fs if f.endswith('.go') and f.endswith('_test.go') == test]
        return sorted(res)

    def match(p, path):
        if p.endswith('/'):
            return path.startswith(p)
        if any(c in p for c in '*?['):
            return fnmatch.fnmatch(path, p) and path.count('/') == p.count('/')
        return path.startswith(p)

    text = lambda f: open(f, errors='ignore').read()
    hits = collections.Counter()

    def owner(f):
        if TEST_HELPER_DIRS & set(f.split('/')[:-1]):
            return 'TOOL'
        if f in OVERRIDES:
            return OVERRIDES[f]
        for late in (False, True):
            for r in RULES:
                for p in r[5]:
                    if p.startswith('~') != late:
                        continue
                    if match(p.lstrip('~'), f):
                        hits[(r[0], p)] += 1
                        return r[0]
        return 'TOOL' if any(f.startswith(t) for t in TOOLING) else None

    prod = walk(False)
    assign, unassigned = collections.defaultdict(list), []
    for f in prod:
        o = owner(f)
        (assign[o].append(f) if o else unassigned.append(f))
    rule_ids = {r[0] for r in RULES}
    bad_override = [k for k, v in OVERRIDES.items() if v not in rule_ids or k not in prod] + [
        k for k, v in HANDLER_OVERRIDES.items() if v not in rule_ids]
    dead = [(r[0], p) for r in RULES for p in r[5] if hits[(r[0], p)] == 0 and not any(
        f in OVERRIDES and match(p.lstrip('~'), f) for f in prod)]
    dead_ctx = [(r[0], c) for r in RULES for c in r[6] if c not in prod]

    file_slice = {f: o for o, fs in assign.items() for f in fs}

    # ---- Handler routing: GameClientLink handle* methods in P05 files go to the slice whose code they call.
    netfiles = [f for f in prod if f.startswith(N) and f.count('/') == 3]
    by_key, by_name = {}, collections.defaultdict(set)
    for f in netfiles:
        for m in DEF_RE.finditer(text(f)):
            by_key[(m.group(1) or '', m.group(2))] = f
            by_name[m.group(2)].add(f)
    bodies = []
    for f in assign.get('P05', []):
        if f.count('/') == 3:
            for m in re.finditer(r'^func \((\w+) \*GameClientLink\) (handle\w+)\(.*?^}', text(f), re.M | re.S):
                bodies.append((f, m.group(1), m.group(2), m.group(0)))

    def callees(var, body):
        out = []
        for m in re.finditer(r'(?:\b(\w+)\.|(?<![\w.)\]]))(\w+)\(', body):
            recv, name = m.group(1), m.group(2)
            # Only calls whose target is certain: methods on the link itself and package-level functions.
            # Calls on any other receiver (live.Target(), x.ObjectID()) are ignored, never guessed by name.
            if recv == var:
                df = by_key.get(('GameClientLink', name))
            elif recv is None:
                df = by_key.get(('', name))
            else:
                df = None
            if df:
                out.append((name, df))
        return out

    # A helper called from many handlers (onLive, flood gates) is plumbing, not the feature.
    fanin = collections.Counter(n for _, v, _, b in bodies for n in {c for c, _ in callees(v, b)})
    shared = {n for n, c in fanin.items() if c >= 6}
    routed, ties, handler_ctx = collections.defaultdict(list), [], collections.defaultdict(set)
    secondary = collections.defaultdict(set)
    seen_handlers = set()
    for f, var, hname, body in bodies:
        seen_handlers.add(hname)
        votes, files = collections.Counter(), set()
        for name, df in callees(var, body):
            if name in shared:
                continue
            files.add(df)
            if file_slice.get(df) not in (None, 'P05', 'TOOL'):
                votes[file_slice[df]] += 1
        top = votes.most_common(2)
        target = top[0][0] if top else 'P05'
        if hname in HANDLER_OVERRIDES:
            target = HANDLER_OVERRIDES[hname]
        elif len(top) == 2 and top[0][1] == top[1][1]:
            ties.append((hname, dict(votes)))
        routed[target].append('%s:%s' % (os.path.basename(f), hname))
        handler_ctx[target] |= {x for x in files if file_slice.get(x) != target}
        for x in files:
            o = file_slice.get(x)
            if o not in (None, 'TOOL', target):
                secondary[o].add('%s:%s' % (os.path.basename(f), hname))
        handler_ctx[target].add(f)
    stale_handler_overrides = sorted(set(HANDLER_OVERRIDES) - seen_handlers)

    # ---- Same-package context: files owned by other slices that define the types/functions an owned file uses.
    TYPE_RE = re.compile(r'^type\s+(\w+)', re.M)
    FUNC_RE = re.compile(r'^func\s+(\w+)\s*[\[(]', re.M)
    types_by_file = {f: set(TYPE_RE.findall(text(f))) for f in prod}
    funcs_by_file = {f: set(FUNC_RE.findall(text(f))) for f in prod}

    def code_only(src):
        # Drop comments and string literals so prose words never count as identifier uses.
        src = re.sub(r'/\*.*?\*/', ' ', src, flags=re.S)
        src = re.sub(r'//[^\n]*', ' ', src)
        src = re.sub(r'`[^`]*`', ' ', src)
        return re.sub(r'"(?:\\.|[^"\\\n])*"', ' ', src)

    def package_context(sid, fs):
        scores = collections.Counter()
        for d in sorted({os.path.dirname(f) for f in fs}):
            others = [x for x in prod if os.path.dirname(x) == d and file_slice.get(x) not in (sid, 'TOOL')
                      and (sid in ('P56', 'P57') or not os.path.basename(x).startswith('admin'))]
            if not others:
                continue
            used = set()
            for f in fs:
                if os.path.dirname(f) == d:
                    used |= set(re.findall(r'\b[A-Za-z_]\w*\b', code_only(text(f))))
            for x in others:
                # A type the slice uses weighs 3 (its definition is core context); a function weighs 1.
                n = 3 * len(types_by_file[x] & used) + len(funcs_by_file[x] & used)
                if n >= 3:
                    scores[x] = n
        return [x for x, _ in sorted(scores.items(), key=lambda kv: (-kv[1], kv[0]))[:6]]

    # ---- Evidence packages: owned packages with tests (the network root runs once per wave) + named suites.
    test_dirs = {os.path.dirname(f) for f in walk(True)}
    evidence_errors = []

    def evidence_packages(r, fs):
        pk = {os.path.dirname(f) for f in fs if os.path.dirname(f) in test_dirs and os.path.dirname(f) != N.rstrip('/')}
        named = re.findall(r'\b((?:internal|cmd|tests)/[\w/]+)', r[4])
        missing = [p for p in named if not os.path.isdir(p)]
        if missing:
            evidence_errors.append((r[0], missing))
        pk |= {p.rstrip('/') for p in named if os.path.isdir(p)}
        return ['./' + p for p in sorted(pk)]

    try:
        sha = subprocess.run(['git', '-C', '.', 'rev-parse', 'HEAD'], capture_output=True, text=True).stdout.strip() or None
        if subprocess.run(['git', '-C', '.', 'status', '--porcelain', '--', 'internal', 'cmd'], capture_output=True, text=True).stdout.strip():
            sha = (sha or '') + '-dirty'
    except OSError:
        sha = None
    man = {'issue': 258, 'slicer': 'v5', 'sha': sha, 'id_policy': 'frozen; new slices take the next free ID',
           'unassigned': unassigned, 'dead_rules': dead, 'dead_context': dead_ctx, 'bad_overrides': bad_override,
           'routing_ties': ties, 'stale_handler_overrides': stale_handler_overrides,
           'wave_level_evidence': ['./' + N.rstrip('/')], 'slices': []}
    all_routed_out = sorted(h for k, v in routed.items() if k != 'P05' for h in v)
    for r in sorted(RULES, key=lambda r: int(r[0][1:])):
        sid = r[0]
        fs = assign.get(sid, [])
        p_lines = g_lines = 0
        for f in fs:
            t = text(f)
            n = t.count('\n')
            if 'Code generated' in t[:300]:
                g_lines += n
            else:
                p_lines += n
        has_net = any(f.startswith(N) for f in fs) or routed.get(sid)
        ctx = [c for c in CORE_CONTEXT if c not in fs] if has_net and sid != 'P05' else []
        for c in list(r[6]) + sorted(handler_ctx.get(sid, ())) + package_context(sid, fs):
            if c not in ctx and c not in fs:
                ctx.append(c)
        entry = dict(id=sid, area=r[1], title=r[2], java_start=r[3], evidence=r[4],
                     evidence_packages=evidence_packages(r, fs), prod_lines=p_lines, generated_lines=g_lines,
                     files=fs, routed_handlers=sorted(routed.get(sid, [])) if sid != 'P05' else [],
                     secondary_handlers=sorted(secondary.get(sid, ())),
                     context_files=ctx)
        if sid == 'P05':
            entry['kept_handlers'] = sorted(routed.get('P05', []))
            entry['routed_out_handlers'] = all_routed_out  # live in P05 files but are reviewed by other slices
        man['slices'].append(entry)

    ids = {r[0] for r in RULES}
    counts = collections.Counter('P%02d' % n for _, ns in WAVES.values() for n in ns)
    wave_errors = sorted('missing from waves: ' + i for i in ids - set(counts))
    wave_errors += sorted('in several waves: ' + i for i, c in counts.items() if c > 1)
    wave_errors += sorted('unknown slice in waves: ' + i for i in set(counts) - ids)
    wave_of = {('P%02d' % n): w for w, (_, ns) in WAVES.items() for n in ns}
    for x in man['slices']:
        x['wave'] = wave_of.get(x['id'])
    man['waves'] = [dict(wave=w, title=t, slices=['P%02d' % n for n in ns]) for w, (t, ns) in WAVES.items()]

    man['domains'], man['domain_runs'], domain_errors = [], [], []
    for did, title, ns, weights, split, note in DOMAINS:
        w = dict(zip(LENS_NAMES, weights))
        if ns == 'cross-cutting':
            slices_ = 'cross-cutting'
        else:
            slices_ = ['P%02d' % n for n in ns]
            bad = [i for i in slices_ if i not in wave_of]
            if bad:
                domain_errors.append((did, bad))
                continue
        man['domains'].append(dict(id=did, title=title, slices=slices_, weights=w, scope_note=note))
        non_bugs = LENS_NAMES[:4]
        if did in ('D18', 'D19'):
            man['domain_runs'].append(dict(run=did + '-A', domain=did, lenses=[l for l in non_bugs if w[l] != 'L'],
                                           scope='whole runtime tree, design level', trigger='any'))
            for wave, (_, wns) in WAVES.items():
                man['domain_runs'].append(dict(run='%s-W%d' % (did, wave), domain=did, lenses=['bugs'],
                                               scope=['P%02d' % n for n in wns], trigger='after-wave-%d' % wave))
            continue
        if did == 'D20':
            man['domain_runs'].append(dict(run='D20-A', domain=did, lenses=non_bugs, scope='tooling, tests, ops', trigger='any'))
            man['domain_runs'].append(dict(run='D20-B', domain=did, lenses=['bugs'], scope='tooling, tests, ops', trigger='any'))
            continue
        groups = [('A1', LENS_NAMES[:2]), ('A2', LENS_NAMES[2:4])] if split else [('A', non_bugs)]
        for tag, lenses in groups:
            man['domain_runs'].append(dict(run='%s-%s' % (did, tag), domain=did, lenses=lenses, scope=slices_, trigger='any'))
        man['domain_runs'].append(dict(run=did + '-B', domain=did, lenses=['bugs'], scope=slices_,
                                       trigger='after-wave-%d' % max(wave_of[i] for i in slices_)))
    covered = {i for d in man['domains'] if isinstance(d['slices'], list) for i in d['slices']}
    domain_errors += sorted('in no domain: ' + i for i in ids - covered)
    man['wave_errors'] = wave_errors
    man['domain_errors'] = domain_errors
    man['evidence_errors'] = evidence_errors
    man['tooling_not_runtime'] = dict(files=assign.get('TOOL', []))
    with open(out, 'w') as fh:
        json.dump(man, fh, indent=1, sort_keys=False)
        fh.write('\n')
    for x in man['slices']:
        print('%s %-4s %6d h=%-3d ctx=%-2d ev=%-2d %s' % (x['id'], x['area'], x['prod_lines'],
              len(x['routed_handlers']) + len(x.get('kept_handlers', [])), len(x['context_files']),
              len(x['evidence_packages']), x['title'][:58]))
    problems = dict(unassigned=unassigned, dead_rules=dead, dead_context=dead_ctx, bad_overrides=bad_override,
                    routing_ties=ties, stale_handler_overrides=stale_handler_overrides, wave_errors=wave_errors,
                    domain_errors=domain_errors, evidence_errors=evidence_errors)
    for k, v in problems.items():
        print('%s: %s' % (k, v if v else 'none'))
    sys.exit(1 if any(problems.values()) else 0)


if __name__ == '__main__':
    main()
