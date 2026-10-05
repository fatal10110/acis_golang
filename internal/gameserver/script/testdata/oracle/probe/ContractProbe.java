import java.io.File;
import java.lang.reflect.Field;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.time.Instant;
import java.time.LocalDateTime;
import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.TimeZone;
import java.util.TreeMap;

import net.sf.l2j.commons.mmocore.ProbeWire;
import net.sf.l2j.commons.pool.ConnectionPool;
import net.sf.l2j.commons.pool.ThreadPool;
import net.sf.l2j.commons.probe.ProbeClock;
import net.sf.l2j.commons.random.Rnd;

import net.sf.l2j.Config;
import net.sf.l2j.gameserver.data.SkillTable;
import net.sf.l2j.gameserver.data.xml.NpcData;
import net.sf.l2j.gameserver.data.xml.PlayerData;
import net.sf.l2j.gameserver.data.xml.PlayerLevelData;
import net.sf.l2j.gameserver.data.xml.ScriptData;
import net.sf.l2j.gameserver.enums.EventHandler;
import net.sf.l2j.gameserver.enums.QuestStatus;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.Sex;
import net.sf.l2j.gameserver.enums.actors.WeightPenalty;
import net.sf.l2j.gameserver.idfactory.IdFactory;
import net.sf.l2j.gameserver.model.World;
import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;
import net.sf.l2j.gameserver.model.spawn.Spawn;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.network.GameClient.GameClientState;
import net.sf.l2j.gameserver.network.clientpackets.Action;
import net.sf.l2j.gameserver.network.clientpackets.RequestBypassToServer;
import net.sf.l2j.gameserver.network.serverpackets.NpcHtmlMessage;
import net.sf.l2j.gameserver.network.serverpackets.QuestList;
import net.sf.l2j.gameserver.scripting.Quest;
import net.sf.l2j.gameserver.scripting.QuestState;
import net.sf.l2j.gameserver.scripting.ScheduledQuest;
import net.sf.l2j.gameserver.skills.L2Skill;

/**
 * Engine-contract goldens (slice V3): runs the reference classes that implement each
 * contract on probe-made players, NPCs and scripts, and writes one golden file per
 * contract family to {@code <out>/contracts}. Every table names the reference lines it
 * exercises; README.md describes the format.
 */
final class ContractProbe
{
	private static final String GS = "java/net/sf/l2j/gameserver/";
	private static final String QUEST_STATE = GS + "scripting/QuestState.java ";
	private static final String QUEST = GS + "scripting/Quest.java ";
	private static final String NPC = GS + "model/actor/Npc.java ";
	private static final String QUEST_LIST = GS + "model/actor/container/player/QuestList.java ";
	private static final String BYPASS = GS + "network/clientpackets/RequestBypassToServer.java ";

	private static final int X = -84318;
	private static final int Y = 246579;
	private static final int Z = -3730;

	private static final String Q001 = "Q001_LettersOfLove";
	private static final String Q003 = "Q003_WillTheSealBeBroken";
	private static final String Q006 = "Q006_StepIntoTheFuture";
	private static final String NOBLESSE = "NoblesseTeleporter";

	private static final int DARIN = 30048;
	private static final int ROXXY = 30006;
	private static final int BAULRO = 30033;
	private static final int TALLOTH = 30141;
	private static final int BLACK_JUDGE = 30981;

	private static final int DROP_ITEM = 1081;
	private static final int DROP_ITEM_2 = 1082;
	private static final int DROP_ITEM_3 = 1083;
	private static final int DARIN_LETTER = 687;

	private final String _revision;
	private final List<Object> _spawned = new ArrayList<>();
	private int _players;
	private int _spot;

	private ContractProbe(String revision)
	{
		_revision = revision;
	}

	static void run(File out) throws Exception
	{
		final File dir = new File(out, "contracts");
		dir.mkdirs();

		final ContractProbe p = new ContractProbe(System.getProperty("probe.revision", "unknown"));
		// Every contract scene runs on its own tasks: boot tasks (AI ticks, deferred item
		// updates, the festival manager, which blocks in real time once it is due 120 s
		// after boot) are dropped first, so the scenes may pass any amount of virtual time.
		ThreadPool.advance(0);
		ThreadPool.cancelAll();
		p.journal().write(dir);
		p.questList().write(dir);
		p.drops().write(dir);
		p.range().write(dir);
		p.dialog().write(dir);
		p.timers().write(dir);
		p.schedule().write(dir);
		// Last: these two reconfigure NPC templates (clans and script lists) for their scenes.
		p.fanout().write(dir);
		p.aggroTick().write(dir);
	}

	// ---------------------------------------------------------------- journal

	private ContractGolden journal() throws Exception
	{
		final ContractGolden g = new ContractGolden("journal.golden", "Quest journal: cond and flags bits; set, unset, set state, set cond and exit statements and packets in order.", _revision);
		final Quest q1 = quest(Q001);

		g.table("journal.cond_flags", new String[]
		{
			QUEST_STATE + "235-285 setCond",
			QUEST_STATE + "287-295 getFlags",
			QUEST_STATE + "339-349 calculateFlags"
		}, "Each row seeds a started Q001 state silently with <cond>=from_cond and, unless from_flags is -, <flags>=from_flags (decimal as stored), then calls setCond(to).", "cond and flags_var are the stored values after the call (- when absent); get_flags is getFlags() after the call, as the quest list sends it.", "Shifts are Java int shifts: the shift count is taken modulo 32.");
		final int none = Integer.MIN_VALUE + 1;
		final int[][] condRows =
		{
			{ 0, none, 1 }, { 0, none, 0 }, { 0, none, 2 }, { 0, none, 3 }, { 1, none, 1 }, { 1, none, 2 }, { 1, none, 3 }, { 2, none, 5 }, { 3, none, 2 }, { 3, none, 1 }, { 5, none, 3 },
			{ 3, 0x80000005, 3 }, { 3, 0x80000005, 4 }, { 3, 0x80000005, 6 }, { 6, 0x80000025, 3 }, { 6, 0x80000025, 4 }, { 6, 0x80000025, 2 }, { 6, 0x80000025, 1 }, { 6, 0x80000025, 0 },
			{ 4, 0x8000000f, 3 }, { 2, 0x80000003, 1 },
			{ 1, none, 32 }, { 1, none, 33 }, { 30, none, 32 }, { 31, none, 33 }, { 2, 0x80000003, 33 }, { 40, 0x80000003, 34 }
		};
		for (int[] r : condRows)
		{
			final Player p = player();
			final QuestState st = q1.newQuestState(p);
			st.put(QuestState.STATE, QuestStatus.STARTED.toString());
			if (r[0] != 0)
				st.put(QuestState.COND, String.valueOf(r[0]));
			if (r[1] != none)
				st.put(QuestState.FLAGS, String.valueOf(r[1]));
			st.setCond(r[2]);
			g.row(r[0] + "-" + (r[1] == none ? "none" : hex(r[1])) + "-to-" + r[2], "from_cond", r[0], "from_flags", r[1] == none ? "-" : String.valueOf(r[1]), "to", r[2], "cond", or(st.get(QuestState.COND)), "flags_var", or(st.get(QuestState.FLAGS)), "get_flags", hex(st.getFlags()));
			drop(p);
		}

		g.table("journal.get_flags", new String[]
		{
			QUEST_STATE + "287-295 getFlags",
			QUEST_STATE + "339-349 calculateFlags"
		}, "getFlags() on a seeded state without a <flags> var: (1 << cond) - 1 with the 0x80000000 marker, or 0 when cond is 0 or absent.");
		for (int cond : new int[] { 0, 1, 2, 5, 31, 32, 33 })
		{
			final Player p = player();
			final QuestState st = q1.newQuestState(p);
			st.put(QuestState.STATE, QuestStatus.STARTED.toString());
			if (cond != 0)
				st.put(QuestState.COND, String.valueOf(cond));
			g.row("cond-" + cond, "cond", cond, "get_flags", hex(st.getFlags()));
			drop(p);
		}

		g.table("journal.write_order", new String[]
		{
			"java/net/sf/l2j/commons/data/MemoSet.java 34-70 set",
			QUEST_STATE + "31-37 QUEST_SET_VAR",
			QUEST_STATE + "86-122 onSet",
			QUEST_STATE + "212-220 setState",
			QUEST_STATE + "235-285 setCond",
			QUEST_STATE + "297-333 exitQuest"
		}, "Each row builds one player with a single seeded quest state (silently), runs op, and lists in order every server packet (S) and SQL statement (Q) the op caused.", "real=Q001 (quest id 1, quest items 687,688,1079,1080), script=NoblesseTeleporter (id -1, not a real quest). {player} is the player's object id.", "state and vars describe the state afterwards (vars sorted, - when the state left the list); items is the player's count of item 687 afterwards.");
		writeRow(g, "set-new-var", "real", "STARTED", 1, 0, false, "set ex 1", st -> st.set("ex", "1"));
		writeRow(g, "set-same-value", "real", "STARTED", 1, 0, false, "set ex 1 (unchanged value)", st -> st.set("ex", "1"));
		writeRow(g, "unset-var", "real", "STARTED", 1, 0, false, "unset ex", st -> st.unset("ex"));
		writeRow(g, "unset-missing-var", "real", "STARTED", 1, 0, false, "unset missing", st -> st.unset("missing"));
		writeRow(g, "set-state-started", "real", "CREATED", 0, 0, false, "setState STARTED", st -> st.setState(QuestStatus.STARTED));
		writeRow(g, "set-state-same", "real", "STARTED", 1, 0, false, "setState STARTED", st -> st.setState(QuestStatus.STARTED));
		writeRow(g, "set-cond-first", "real", "STARTED", 0, 0, false, "setCond 1", st -> st.setCond(1));
		writeRow(g, "set-cond-next", "real", "STARTED", 1, 0, false, "setCond 2", st -> st.setCond(2));
		writeRow(g, "set-cond-skip", "real", "STARTED", 1, 0, false, "setCond 3", st -> st.setCond(3));
		writeRow(g, "set-cond-down-unset-flags", "real", "STARTED", 4, 0x8000000f, false, "setCond 2", st -> st.setCond(2));
		writeRow(g, "set-cond-down-mask-flags", "real", "STARTED", 6, 0x80000025, false, "setCond 4", st -> st.setCond(4));
		writeRow(g, "set-cond-same", "real", "STARTED", 2, 0, false, "setCond 2", st -> st.setCond(2));
		writeRow(g, "set-cond-script", "script", "STARTED", 1, 0, false, "setCond 3", st -> st.setCond(3));
		writeRow(g, "exit-complete", "real", "STARTED", 3, 0x80000005, true, "exitQuest false", st -> st.exitQuest(false));
		writeRow(g, "exit-repeatable", "real", "STARTED", 3, 0x80000005, true, "exitQuest true", st -> st.exitQuest(true));
		writeRow(g, "exit-created", "real", "CREATED", 0, 0, true, "exitQuest false", st -> st.exitQuest(false));
		writeRow(g, "exit-completed", "real", "COMPLETED", 0, 0, true, "exitQuest true", st -> st.exitQuest(true));
		writeRow(g, "exit-script-repeatable", "script", "STARTED", 1, 0, false, "exitQuest true", st -> st.exitQuest(true));
		writeRow(g, "new-state", "real", "-", 0, 0, false, "newQuestState", null);
		return g;
	}

	private interface StateOp
	{
		void run(QuestState st) throws Exception;
	}

	private void writeRow(ContractGolden g, String id, String quest, String state, int cond, int flags, boolean items, String op, StateOp fn) throws Exception
	{
		final Quest q = quest(quest.equals("real") ? Q001 : NOBLESSE);
		final Player p = player();
		if (items)
			Quest.giveItems(p, DARIN_LETTER, 1);
		settle();

		QuestState st = null;
		if (!state.equals("-"))
		{
			st = q.newQuestState(p);
			st.put(QuestState.STATE, state);
			if (cond != 0)
				st.put(QuestState.COND, String.valueOf(cond));
			if (flags != 0)
				st.put(QuestState.FLAGS, String.valueOf(flags));
			if (state.equals("STARTED"))
				st.put("ex", "1");
		}

		final Recorder rec = new Recorder(p);
		if (fn == null)
			st = q.newQuestState(p);
		else
			fn.run(st);
		final List<String> lines = rec.stop(ContractGolden.roles(p.getObjectId(), "player"));

		final QuestState after = p.getQuestList().getQuestState(q.getName());
		g.row(id, "quest", quest, "seed_state", state, "seed_cond", cond, "seed_flags", flags == 0 ? "-" : String.valueOf(flags), "op", op, "state", after == null ? "-" : or(after.get(QuestState.STATE)), "vars", vars(after), "items", p.getInventory().getItemCount(DARIN_LETTER));
		g.lines(lines);
		drop(p);
	}

	// ---------------------------------------------------------------- quest list packet

	private ContractGolden questList() throws Exception
	{
		final ContractGolden g = new ContractGolden("questlist.golden", "QuestList packet contents.", _revision);
		g.table("questlist.packet", new String[]
		{
			GS + "network/serverpackets/QuestList.java 12-29 QuestList",
			QUEST_LIST + "72-75 getAllQuests",
			QUEST_STATE + "287-295 getFlags"
		}, "Each row seeds the listed states, in that order, silently (name:state:cond[:flags]) and sends one QuestList. Entries are real quests that are started or completed, in journal order, with getFlags().", "started is getAllQuests(false).size(), the count the 25-quest check reads.");
		listRow(g, "empty");
		listRow(g, "one-started", Q001 + ":STARTED:1");
		listRow(g, "created-skipped", Q001 + ":CREATED:0");
		listRow(g, "completed-listed", Q001 + ":COMPLETED:0");
		listRow(g, "completed-keeps-cond", Q001 + ":COMPLETED:2");
		listRow(g, "script-skipped", NOBLESSE + ":STARTED:1");
		listRow(g, "journal-order", Q006 + ":STARTED:2", Q001 + ":COMPLETED:0", Q003 + ":STARTED:1:" + 0x80000005, "Q002_WhatWomenWant:CREATED:0");
		listRow(g, "explicit-flags", Q003 + ":STARTED:3:" + 0x80000005);
		listRow(g, "cond-zero-started", Q003 + ":STARTED:0");
		return g;
	}

	private void listRow(ContractGolden g, String id, String... seeds) throws Exception
	{
		final Player p = player();
		for (String s : seeds)
		{
			final String[] f = s.split(":");
			final QuestState st = quest(f[0]).newQuestState(p);
			st.put(QuestState.STATE, f[1]);
			if (!f[2].equals("0"))
				st.put(QuestState.COND, f[2]);
			if (f.length > 3)
				st.put(QuestState.FLAGS, f[3]);
		}
		final Recorder rec = new Recorder(p);
		p.sendPacket(new QuestList(p));
		final List<String> lines = rec.stop(ContractGolden.roles(p.getObjectId(), "player"));
		g.row(id, "seed", seeds.length == 0 ? "-" : String.join(",", seeds), "started", p.getQuestList().getAllQuests(false).size());
		g.lines(lines);
		drop(p);
	}

	// ---------------------------------------------------------------- drops

	private ContractGolden drops() throws Exception
	{
		final ContractGolden g = new ContractGolden("drops.golden", "Quest drops: the four drop types, roll counts and a fractional rate.", _revision);
		final String[] single =
		{
			QUEST + "74-77 DROP_DIVMOD",
			QUEST + "1164-1222 dropItems"
		};
		final String note1 = "Each row sets Config.RATE_QUEST_DROP to rate, gives the player have of the item, scripts the random source with rolls, then calls dropItems(player, item, count, needed, chance, type). MAX_CHANCE is 1000000.";
		final String note2 = "draws lists every random draw as bound:value in call order; given is the item count added; result is the return value. The S lines are the packets sent at once (item and inventory updates are deferred and not listed).";
		final String note3 = "DIVMOD multiplies the int chance by the double rate and truncates to int before the division and the modulo.";

		g.table("drop.divmod", single, note1, note2, note3);
		dropRow(g, "rate1-hit", 0, 1.0, 1, 0, 300000, 0, 299999);
		dropRow(g, "rate1-miss", 0, 1.0, 1, 0, 300000, 0, 300000);
		dropRow(g, "rate1.5-hit", 0, 1.5, 1, 0, 300000, 0, 449999);
		dropRow(g, "rate1.5-miss", 0, 1.5, 1, 0, 300000, 0, 450000);
		dropRow(g, "rate1.5-over-max-hit", 0, 1.5, 2, 0, 800000, 0, 199999);
		dropRow(g, "rate1.5-over-max-miss", 0, 1.5, 2, 0, 800000, 0, 200000);
		dropRow(g, "rate1.5-truncates", 0, 1.5, 1, 0, 333333, 0, 499998);
		dropRow(g, "rate1.5-truncates-miss", 0, 1.5, 1, 0, 333333, 0, 499999);
		dropRow(g, "rate2-exact-max", 0, 2.0, 3, 0, 500000, 0, 0);
		dropRow(g, "needed-clamps", 0, 1.5, 5, 7, 1000000, 4, 0);
		dropRow(g, "needed-reached-already", 0, 1.0, 1, 3, 1000000, 3);
		dropRow(g, "needed-not-reached", 0, 1.0, 1, 3, 1000000, 1, 0);
		dropRow(g, "needed-exactly-reached", 0, 1.0, 1, 3, 1000000, 2, 0);

		g.table("drop.fixed_rate", new String[]
		{
			QUEST + "74-77 DROP_FIXED_RATE",
			QUEST + "1164-1222 dropItems",
			QUEST + "1133-1136 dropItemsAlways"
		}, note1, note2, "dropItemsAlways is DROP_FIXED_RATE with chance MAX_CHANCE. The amount is (int)(count * rate).");
		dropRow(g, "hit", 1, 1.0, 1, 0, 300000, 0, 299999);
		dropRow(g, "miss", 1, 1.0, 1, 0, 300000, 0, 300000);
		dropRow(g, "rate1.5-amount-truncates", 1, 1.5, 3, 0, 300000, 0, 0);
		dropRow(g, "rate1.5-chance-unscaled", 1, 1.5, 3, 0, 300000, 0, 300000);
		dropRow(g, "rate0.5-amount-zero", 1, 0.5, 1, 0, 1000000, 0, 0);
		dropRow(g, "always", 1, 1.0, 1, 1, 1000000, 0, 999999);

		g.table("drop.fixed_count", new String[]
		{
			QUEST + "74-77 DROP_FIXED_COUNT",
			QUEST + "1164-1222 dropItems"
		}, note1, note2, "The roll is compared with chance * rate as a double; the amount is count.");
		dropRow(g, "rate1.5-hit", 2, 1.5, 4, 0, 300000, 0, 449999);
		dropRow(g, "rate1.5-miss", 2, 1.5, 4, 0, 300000, 0, 450000);
		dropRow(g, "rate1.5-fraction", 2, 1.5, 1, 0, 333333, 0, 499999);
		dropRow(g, "rate1.5-fraction-miss", 2, 1.5, 1, 0, 333333, 0, 500000);

		g.table("drop.fixed_both", new String[]
		{
			QUEST + "74-77 DROP_FIXED_BOTH",
			QUEST + "1164-1222 dropItems"
		}, note1, note2, "Neither the chance nor the amount is scaled by the rate.");
		dropRow(g, "rate1.5-hit", 3, 1.5, 2, 0, 300000, 0, 299999);
		dropRow(g, "rate1.5-miss", 3, 1.5, 2, 0, 300000, 0, 300000);

		g.table("drop.multiple", new String[]
		{
			QUEST + "1241-1318 dropMultipleItems"
		}, "Each row sets the rate, gives the player the have counts, scripts rolls, then calls dropMultipleItems(player, infos, type); infos are item:count:needed:chance in order, have is item:count.", "One draw per entry not already at its needed count. One sound at the end: middle when every entry reached its needed count, else itemget; none when nothing was given.", note2);
		multiRow(g, "divmod-all-hit", 0, 1.0, new int[][] { { DROP_ITEM, 1, 2, 500000 }, { DROP_ITEM_2, 1, 2, 500000 } }, new int[][] {}, 0, 0);
		multiRow(g, "divmod-one-hit", 0, 1.0, new int[][] { { DROP_ITEM, 1, 2, 500000 }, { DROP_ITEM_2, 1, 2, 500000 } }, new int[][] {}, 0, 500000);
		multiRow(g, "divmod-reached", 0, 1.0, new int[][] { { DROP_ITEM, 1, 2, 500000 }, { DROP_ITEM_2, 1, 2, 500000 } }, new int[][] { { DROP_ITEM, 1 }, { DROP_ITEM_2, 1 } }, 0, 0);
		multiRow(g, "divmod-skip-full", 0, 1.0, new int[][] { { DROP_ITEM, 1, 2, 500000 }, { DROP_ITEM_2, 1, 2, 500000 }, { DROP_ITEM_3, 1, 0, 500000 } }, new int[][] { { DROP_ITEM, 2 } }, 0, 0);
		multiRow(g, "unlimited-never-reached", 0, 1.0, new int[][] { { DROP_ITEM, 1, 0, 1000000 } }, new int[][] {}, 0);
		multiRow(g, "none-given-no-sound", 0, 1.0, new int[][] { { DROP_ITEM, 1, 2, 500000 }, { DROP_ITEM_2, 1, 2, 500000 } }, new int[][] {}, 999999, 999999);
		multiRow(g, "all-full-no-draw", 0, 1.0, new int[][] { { DROP_ITEM, 1, 2, 500000 } }, new int[][] { { DROP_ITEM, 2 } });
		multiRow(g, "fixed-count-rate1.5", 2, 1.5, new int[][] { { DROP_ITEM, 3, 0, 300000 }, { DROP_ITEM_2, 3, 5, 300000 } }, new int[][] { { DROP_ITEM_2, 4 } }, 449999, 449999);
		multiRow(g, "fixed-rate-rate1.5", 1, 1.5, new int[][] { { DROP_ITEM, 3, 0, 300000 } }, new int[][] {}, 299999);
		multiRow(g, "fixed-both-rate1.5", 3, 1.5, new int[][] { { DROP_ITEM, 3, 0, 300000 } }, new int[][] {}, 299999);

		Config.RATE_QUEST_DROP = 1.0;
		return g;
	}

	private static final String[] TYPES = { "divmod", "fixed_rate", "fixed_count", "fixed_both" };

	private void dropRow(ContractGolden g, String id, int type, double rate, int count, int needed, int chance, int have, int... rolls) throws Exception
	{
		final Player p = player();
		if (have > 0)
			Quest.giveItems(p, DROP_ITEM, have);
		settle();
		Config.RATE_QUEST_DROP = rate;
		Rnd.script(rolls);
		final Recorder rec = new Recorder(p);
		final boolean result = Quest.dropItems(p, DROP_ITEM, count, needed, chance, (byte) type);
		final List<String> lines = rec.stop(ContractGolden.roles(p.getObjectId(), "player"));
		final String draws = Rnd.endScript();
		g.row(id, "type", TYPES[type], "rate", rate, "count", count, "needed", needed, "chance", chance, "have", have, "rolls", ints(rolls), "draws", draws, "given", p.getInventory().getItemCount(DROP_ITEM) - have, "result", result);
		g.lines(sounds(lines));
		drop(p);
	}

	private void multiRow(ContractGolden g, String id, int type, double rate, int[][] infos, int[][] have, int... rolls) throws Exception
	{
		final Player p = player();
		for (int[] h : have)
			Quest.giveItems(p, h[0], h[1]);
		settle();
		final int[] before = new int[infos.length];
		for (int i = 0; i < infos.length; i++)
			before[i] = p.getInventory().getItemCount(infos[i][0]);
		Config.RATE_QUEST_DROP = rate;
		Rnd.script(rolls);
		final Recorder rec = new Recorder(p);
		final boolean result = Quest.dropMultipleItems(p, infos, (byte) type);
		final List<String> lines = rec.stop(ContractGolden.roles(p.getObjectId(), "player"));
		final String draws = Rnd.endScript();
		final List<String> given = new ArrayList<>();
		for (int i = 0; i < infos.length; i++)
			given.add(infos[i][0] + ":" + (p.getInventory().getItemCount(infos[i][0]) - before[i]));
		final List<String> inf = new ArrayList<>();
		for (int[] i : infos)
			inf.add(i[0] + ":" + i[1] + ":" + i[2] + ":" + i[3]);
		final List<String> hv = new ArrayList<>();
		for (int[] h : have)
			hv.add(h[0] + ":" + h[1]);
		g.row(id, "type", TYPES[type], "rate", rate, "infos", String.join(",", inf), "have", hv.isEmpty() ? "-" : String.join(",", hv), "rolls", ints(rolls), "draws", draws, "given", String.join(",", given), "result", result);
		g.lines(sounds(lines));
		drop(p);
	}

	private static List<String> sounds(List<String> lines)
	{
		final List<String> out = new ArrayList<>();
		for (String l : lines)
			if (l.startsWith("S PlaySound") || l.startsWith("S SystemMessage"))
				out.add(l);
		return out;
	}

	// ---------------------------------------------------------------- range

	private ContractGolden range() throws Exception
	{
		final ContractGolden g = new ContractGolden("range.golden", "Range checks: strict less-than, 3D, centre to centre.", _revision);
		g.table("range.quest_event", new String[]
		{
			NPC + "89 INTERACTION_DISTANCE",
			QUEST_LIST + "124-142 processQuestEvent",
			GS + "model/location/Location.java 162-175 isIn3DRadius"
		}, "The player stands at a fixed point; Darin (30048, Q001 talk-bound, collision radius ignored) is moved to dx,dy,dz from it and set as the last quest NPC; then processQuestEvent(Q001_LettersOfLove, 30048-02.htm) runs. ran says whether the quest event ran (its page was sent). dist is the 3D distance.");
		final int[][] offsets = { { 149, 0, 0 }, { 150, 0, 0 }, { 151, 0, 0 }, { 0, 0, 149 }, { 0, 0, 150 }, { 100, 0, 111 }, { 100, 0, 112 }, { 106, 106, 0 }, { 107, 106, 0 }, { 0, 0, 0 } };
		for (int[] o : offsets)
		{
			final Player p = player();
			final Npc darin = spawn(DARIN, p, o[0], o[1], o[2]);
			p.getQuestList().setLastQuestNpcObjectId(darin.getObjectId());
			final Recorder rec = new Recorder(p);
			p.getQuestList().processQuestEvent(Q001, "30048-02.htm");
			final List<String> lines = rec.stop(ContractGolden.roles(p.getObjectId(), "player", darin.getObjectId(), "darin"));
			g.row(o[0] + "," + o[1] + "," + o[2], "dx", o[0], "dy", o[1], "dz", o[2], "dist", String.format(Locale.ROOT, "%.3f", p.distance3D(darin)), "ran", !lines.isEmpty());
			drop(p);
		}

		g.table("range.interact", new String[]
		{
			NPC + "89 INTERACTION_DISTANCE",
			GS + "model/actor/ai/type/PlayerAI.java 531-539 canDoInteract",
			BYPASS + "100-124 npc_"
		}, "canDoInteract(npc) for the same offsets: the npc_ bypass and interaction gate (strict less-than 150, 3D, no collision radius).");
		for (int[] o : offsets)
		{
			final Player p = player();
			final Npc darin = spawn(DARIN, p, o[0], o[1], o[2]);
			g.row(o[0] + "," + o[1] + "," + o[2], "dx", o[0], "dy", o[1], "dz", o[2], "dist", String.format(Locale.ROOT, "%.3f", p.distance3D(darin)), "can_interact", p.getAI().canDoInteract(darin));
			drop(p);
		}
		return g;
	}

	// ---------------------------------------------------------------- dialog

	private ContractGolden dialog() throws Exception
	{
		final ContractGolden g = new ContractGolden("dialog.golden", "Dialog rules: last quest NPC, general, single and choose windows, the Quest bypass, the npc_ handler.", _revision);
		final String noteRow = "Each row: a fresh player beside fresh NPCs (all within 40 of the player); seed lists quest states set silently (name:state); the bypass is first offered by a page sent silently (the whitelist), then sent as a client RequestBypassToServer; the S lines are every packet it caused, in order. {npc} roles are the NPC object ids.";
		final String noteBind = "Bindings: Darin 30048 quest-start and talk Q001; Roxxy 30006 talk Q001, Q006, NoblesseTeleporter and quest-start Q006; Baulro 30033 talk Q001 and Q006; Talloth 30141 quest-start and talk Q003.";

		g.table("dialog.last_quest_npc", new String[]
		{
			NPC + "278-292 onInteract",
			NPC + "1480-1523 showQuestWindowSingle",
			QUEST_LIST + "39-47 lastQuestNpcObjectId"
		}, "last is the last quest NPC after the steps (role, or 0 when unset). interact is target then interact through Action packets.", noteBind);
		{
			final Scene s0 = scene(DARIN, ROXXY);
			g.row("interact-sets", "steps", "interact darin", "last", s0.lastAfter(() -> s0.interact(s0.npc(DARIN))));
			s0.end();
			final Scene s1 = scene(DARIN, BLACK_JUDGE);
			g.row("interact-first-talk-sets", "steps", "interact black_judge (one first-talk script)", "last", s1.lastAfter(() -> s1.interact(s1.npc(BLACK_JUDGE))));
			s1.end();
			final Scene s2 = scene(DARIN, ROXXY);
			g.row("single-window-sets", "steps", "interact darin; npc_{roxxy}_Quest Q006_StepIntoTheFuture", "last", s2.lastAfter(() ->
			{
				s2.interact(s2.npc(DARIN));
				final String cmd = "npc_" + s2.npc(ROXXY).getObjectId() + "_Quest " + Q006;
				s2.offer(cmd);
				s2.bypass(cmd);
			}));
			s2.end();
			final Scene s3 = scene(DARIN, BAULRO);
			g.row("no-quest-window-keeps", "steps", "interact darin; npc_{baulro}_Quest (no candidate)", "last", s3.lastAfter(() ->
			{
				s3.interact(s3.npc(DARIN));
				final String cmd = "npc_" + s3.npc(BAULRO).getObjectId() + "_Quest";
				s3.offer(cmd);
				s3.bypass(cmd);
			}));
			s3.end();
			final Scene s4 = scene(DARIN, ROXXY);
			g.row("choose-window-keeps", "steps", "interact darin; seed Q001 started; npc_{roxxy}_Quest (two candidates)", "last", s4.lastAfter(() ->
			{
				s4.interact(s4.npc(DARIN));
				s4.seed(Q001 + ":STARTED");
				final String cmd = "npc_" + s4.npc(ROXXY).getObjectId() + "_Quest";
				s4.offer(cmd);
				s4.bypass(cmd);
			}));
			s4.end();
		}

		g.table("dialog.general_window", new String[]
		{
			NPC + "1160-1176 Quest command in onBypassFeedback",
			NPC + "1442-1472 showQuestWindowGeneral",
			NPC + "1480-1523 showQuestWindowSingle",
			NPC + "1531-1556 showQuestWindowChoose",
			BYPASS + "100-124 npc_"
		}, noteRow, noteBind, "Candidates: talk-bound real quests whose state exists and is not created (completed included), then quest-start real quests; zero gives the no-quest page, one the single window, more the choice list.", "after lists the states afterwards.");
		dialogRow(g, "zero-candidates", new int[] { BAULRO }, BAULRO, "", new String[] {});
		dialogRow(g, "one-quest-start", new int[] { DARIN }, DARIN, "", new String[] {});
		dialogRow(g, "one-quest-start-not-talk-first", new int[] { ROXXY }, ROXXY, "", new String[] {});
		dialogRow(g, "created-state-skipped", new int[] { ROXXY }, ROXXY, "", new String[] { Q001 + ":CREATED" });
		dialogRow(g, "started-and-start", new int[] { ROXXY }, ROXXY, "", new String[] { Q001 + ":STARTED" });
		dialogRow(g, "completed-and-started", new int[] { ROXXY }, ROXXY, "", new String[] { Q001 + ":COMPLETED", Q006 + ":STARTED" });
		dialogRow(g, "started-start-quest-listed-once", new int[] { ROXXY }, ROXXY, "", new String[] { Q006 + ":STARTED" });
		dialogRow(g, "completed-only", new int[] { BAULRO }, BAULRO, "", new String[] { Q001 + ":COMPLETED" });
		dialogRow(g, "overweight-single", new int[] { DARIN }, DARIN, "", new String[] { "overweight" });
		dialogRow(g, "overweight-choose", new int[] { ROXXY }, ROXXY, "", new String[] { "overweight", Q001 + ":STARTED" });
		dialogRow(g, "too-many-quests", new int[] { DARIN }, DARIN, "", new String[] { "started25" });
		dialogRow(g, "too-many-but-has-state", new int[] { DARIN }, DARIN, "", new String[] { "started25", Q001 + ":STARTED" });
		dialogRow(g, "twenty-four-quests", new int[] { DARIN }, DARIN, "", new String[] { "started24" });
		dialogRow(g, "completed-do-not-count", new int[] { DARIN }, DARIN, "", new String[] { "completed25" });

		g.table("dialog.single_window", new String[]
		{
			NPC + "1160-1176 Quest command in onBypassFeedback",
			NPC + "1480-1523 showQuestWindowSingle",
			GS + "data/xml/ScriptData.java 153-156 getQuest",
			BYPASS + "100-124 npc_"
		}, noteRow, noteBind, "npc_<id>_Quest <name> resolves the name case-insensitively over every loaded script and does not require it to be bound to the NPC. A state is created only when the NPC is a quest start of that quest.");
		dialogRow(g, "bound-quest", new int[] { DARIN }, DARIN, Q001, new String[] {});
		dialogRow(g, "unbound-quest", new int[] { DARIN }, DARIN, Q003, new String[] {});
		dialogRow(g, "unbound-quest-with-state", new int[] { DARIN }, DARIN, Q003, new String[] { Q003 + ":STARTED" });
		dialogRow(g, "talk-only-no-state", new int[] { ROXXY }, ROXXY, Q001, new String[] {});
		dialogRow(g, "lower-case-name", new int[] { DARIN }, DARIN, Q001.toLowerCase(Locale.ROOT), new String[] {});
		dialogRow(g, "unknown-name", new int[] { DARIN }, DARIN, "Q999_NoSuchQuest", new String[] {});
		dialogRow(g, "script-name-overweight", new int[] { ROXXY }, ROXXY, NOBLESSE, new String[] { "overweight" });
		dialogRow(g, "script-name-too-many", new int[] { ROXXY }, ROXXY, NOBLESSE, new String[] { "started25" });
		dialogRow(g, "real-overweight", new int[] { DARIN }, DARIN, Q001, new String[] { "overweight" });
		dialogRow(g, "real-too-many", new int[] { DARIN }, DARIN, Q001, new String[] { "started25" });
		dialogRow(g, "real-too-many-has-state", new int[] { DARIN }, DARIN, Q001, new String[] { "started25", Q001 + ":STARTED" });

		g.table("dialog.quest_bypass", new String[]
		{
			BYPASS + "136-146 Quest",
			QUEST_LIST + "124-142 processQuestEvent",
			QUEST + "111-127 equals"
		}, noteRow, noteBind, "The top-level Quest <name> <event> bypass: whitelisted; the last quest NPC (set silently) must be in the world, strictly within 150 (3D) and talk-bound to a script equal to the named one. Every rejection is silent. Q001's event 30048-02.htm returns that page with no state change.");
		questBypassRow(g, "runs", DARIN, 0, "Quest " + Q001 + " 30048-02.htm", true, false);
		questBypassRow(g, "not-whitelisted", DARIN, 0, "Quest " + Q001 + " 30048-02.htm", false, false);
		questBypassRow(g, "no-event", DARIN, 0, "Quest " + Q001, true, false);
		questBypassRow(g, "lower-case-name", DARIN, 0, "Quest " + Q001.toLowerCase(Locale.ROOT) + " 30048-02.htm", true, false);
		questBypassRow(g, "unknown-quest", DARIN, 0, "Quest Q999_NoSuchQuest 30048-02.htm", true, false);
		questBypassRow(g, "talk-bound-other-npc", BAULRO, 0, "Quest " + Q001 + " 30048-02.htm", true, false);
		questBypassRow(g, "npc-not-bound", TALLOTH, 0, "Quest " + Q001 + " 30048-02.htm", true, false);
		questBypassRow(g, "npc-gone", DARIN, 0, "Quest " + Q001 + " 30048-02.htm", true, true);
		questBypassRow(g, "at-150", DARIN, 150, "Quest " + Q001 + " 30048-02.htm", true, false);
		questBypassRow(g, "at-149", DARIN, 149, "Quest " + Q001 + " 30048-02.htm", true, false);
		questBypassRow(g, "no-last-npc", 0, 0, "Quest " + Q001 + " 30048-02.htm", true, false);

		g.table("dialog.npc_bypass", new String[]
		{
			BYPASS + "100-124 npc_",
			GS + "model/actor/ai/type/PlayerAI.java 531-539 canDoInteract"
		}, noteRow, "The npc_ handler: whitelist, then the id up to the next underscore; one ActionFailed after any command whose id parses (also when the NPC is too far or the command has no underscore after the id); nothing when the id does not parse or the bypass was not offered.");
		npcBypassRow(g, "accepted", 0, "npc_{darin}_Quest", true);
		npcBypassRow(g, "not-whitelisted", 0, "npc_{darin}_Quest", false);
		npcBypassRow(g, "too-far", 150, "npc_{darin}_Quest", true);
		npcBypassRow(g, "no-command", 0, "npc_{darin}", true);
		npcBypassRow(g, "id-not-a-number", 0, "npc_x_Quest", true);
		npcBypassRow(g, "unknown-object", 0, "npc_1_Quest", true);

		g.table("dialog.quest_equality", new String[]
		{
			QUEST + "111-127 equals"
		}, "Quest.equals over loaded scripts: any two behaviors (DefaultNpc subclasses) are equal; two scripts with the same positive id are equal when their names match; otherwise equal when the class names match.");
		final Quest q1 = quest(Q001);
		final Quest q3 = quest(Q003);
		final Quest nob = quest(NOBLESSE);
		final List<Quest> behaviors = new ArrayList<>();
		for (Quest q : ScriptData.getInstance().getQuests())
			if (q instanceof net.sf.l2j.gameserver.scripting.script.ai.individual.DefaultNpc && behaviors.size() < 2)
				behaviors.add(q);
		g.row("same-quest", "a", q1.getName(), "b", q1.getName(), "equal", q1.equals(q1));
		g.row("two-quests", "a", q1.getName(), "b", q3.getName(), "equal", q1.equals(q3));
		g.row("same-script", "a", nob.getName(), "b", nob.getName(), "equal", nob.equals(nob));
		g.row("quest-and-script", "a", q1.getName(), "b", nob.getName(), "equal", q1.equals(nob));
		g.row("two-behaviors", "a", behaviors.get(0).getClass().getName().substring(ScriptProbe.PKG.length()), "b", behaviors.get(1).getClass().getName().substring(ScriptProbe.PKG.length()), "equal", behaviors.get(0).equals(behaviors.get(1)));
		g.row("behavior-and-script", "a", behaviors.get(0).getClass().getName().substring(ScriptProbe.PKG.length()), "b", nob.getName(), "equal", behaviors.get(0).equals(nob));
		return g;
	}

	private void dialogRow(ContractGolden g, String id, int[] npcs, int target, String name, String[] seeds) throws Exception
	{
		final Scene s = scene(npcs);
		for (String seed : seeds)
			s.seed(seed);
		final String cmd = "npc_" + s.npc(target).getObjectId() + "_Quest" + (name.isEmpty() ? "" : " " + name);
		s.offer(cmd);
		final Recorder rec = new Recorder(s._p);
		s.bypass(cmd);
		final List<String> lines = rec.stop(s.roles());
		g.row(id, "npc", s.role(target), "bypass", "npc_{" + s.role(target) + "}_Quest" + (name.isEmpty() ? "" : " " + name), "seed", seeds.length == 0 ? "-" : String.join(",", seeds), "after", s.states(), "last", s.last());
		g.lines(lines);
		s.end();
	}

	private void questBypassRow(ContractGolden g, String id, int lastNpc, int dx, String cmd, boolean offer, boolean gone) throws Exception
	{
		final Scene s = lastNpc == 0 ? scene(DARIN) : scene(lastNpc);
		if (lastNpc != 0)
		{
			final Npc npc = s.npc(lastNpc);
			if (dx != 0)
				npc.setXYZ(s._p.getX() + dx, s._p.getY(), s._p.getZ());
			s._p.getQuestList().setLastQuestNpcObjectId(npc.getObjectId());
			if (gone)
				npc.deleteMe();
		}
		if (offer)
			s.offer(cmd);
		final Recorder rec = new Recorder(s._p);
		s.bypass(cmd);
		final List<String> lines = rec.stop(s.roles());
		g.row(id, "last_npc", lastNpc == 0 ? "-" : s.role(lastNpc), "dx", dx, "gone", gone, "offered", offer, "bypass", cmd, "after", s.states());
		g.lines(lines);
		s.end();
	}

	private void npcBypassRow(ContractGolden g, String id, int dx, String template, boolean offer) throws Exception
	{
		final Scene s = scene(DARIN);
		final Npc darin = s.npc(DARIN);
		if (dx != 0)
			darin.setXYZ(s._p.getX() + dx, s._p.getY(), s._p.getZ());
		final String cmd = template.replace("{darin}", String.valueOf(darin.getObjectId()));
		if (offer)
			s.offer(cmd);
		final Recorder rec = new Recorder(s._p);
		s.bypass(cmd);
		final List<String> lines = rec.stop(s.roles());
		g.row(id, "dx", dx, "offered", offer, "bypass", template, "after", s.states());
		g.lines(lines);
		s.end();
	}

	/** A player and the NPCs of one dialog row. */
	private final class Scene
	{
		final Player _p;
		final GameClient _c;
		final Map<Integer, Npc> _npcs = new TreeMap<>();

		Scene(int... npcIds) throws Exception
		{
			_p = player();
			_c = _p.getClient();
			int dx = -40;
			for (int id : npcIds)
			{
				_npcs.put(id, spawn(id, _p, dx, 0, 0));
				dx += 20;
			}
			settle();
		}

		Npc npc(int id)
		{
			return _npcs.get(id);
		}

		String role(int id)
		{
			return switch (id)
			{
				case DARIN -> "darin";
				case ROXXY -> "roxxy";
				case BAULRO -> "baulro";
				case TALLOTH -> "talloth";
				case BLACK_JUDGE -> "black_judge";
				default -> String.valueOf(id);
			};
		}

		Map<Integer, String> roles()
		{
			final Map<Integer, String> m = ContractGolden.roles(_p.getObjectId(), "player");
			for (Map.Entry<Integer, Npc> e : _npcs.entrySet())
				m.put(e.getValue().getObjectId(), role(e.getKey()));
			return m;
		}

		void seed(String seed) throws Exception
		{
			switch (seed)
			{
				case "overweight":
				{
					final Field f = Player.class.getDeclaredField("_weightPenalty");
					f.setAccessible(true);
					f.set(_p, WeightPenalty.LEVEL_3);
					return;
				}
				case "started25", "started24", "completed25":
				{
					final int n = seed.endsWith("24") ? 24 : 25;
					final String state = seed.startsWith("completed") ? "COMPLETED" : "STARTED";
					int added = 0;
					for (Quest q : ScriptData.getInstance().getQuests())
					{
						if (added == n)
							break;
						if (!q.isRealQuest() || q.getName().equals(Q001) || q.getName().equals(Q003) || q.getName().equals(Q006))
							continue;
						final QuestState st = q.newQuestState(_p);
						st.put(QuestState.STATE, state);
						st.put(QuestState.COND, "1");
						added++;
					}
					return;
				}
				default:
				{
					final String[] f = seed.split(":");
					final QuestState st = quest(f[0]).newQuestState(_p);
					st.put(QuestState.STATE, f[1]);
					if (f[1].equals("STARTED"))
						st.put(QuestState.COND, "1");
				}
			}
		}

		/** Sends a page offering the commands, with recording off, so the whitelist accepts them. */
		void offer(String... cmds)
		{
			final StringBuilder sb = new StringBuilder("<html><body>");
			for (String c : cmds)
				sb.append("<a action=\"bypass -h ").append(c).append("\">x</a>");
			sb.append("</body></html>");
			final NpcHtmlMessage html = new NpcHtmlMessage(0);
			html.setHtml(sb.toString());
			_p.sendPacket(html);
		}

		void bypass(String command) throws Exception
		{
			// The reference throttles bypasses to one per 100 ms of real time.
			Thread.sleep(110);
			final ByteBuffer b = ByteBuffer.allocate(2 * command.length() + 2).order(ByteOrder.LITTLE_ENDIAN);
			for (char ch : command.toCharArray())
				b.putChar(ch);
			b.putChar('\0');
			// Handled inline: the clock does not move, so AI ticks stay out of the recording.
			ProbeWire.receive(_c, new RequestBypassToServer(), b.array());
		}

		void interact(Npc npc)
		{
			if (_p.getTarget() != npc)
				action(npc);
			action(npc);
		}

		private void action(Npc npc)
		{
			final ByteBuffer b = ByteBuffer.allocate(17).order(ByteOrder.LITTLE_ENDIAN);
			b.putInt(npc.getObjectId()).putInt(_p.getX()).putInt(_p.getY()).putInt(_p.getZ()).put((byte) 0);
			ProbeWire.receive(_c, new Action(), b.array());
			ThreadPool.advance(500);
		}

		interface Steps
		{
			void run() throws Exception;
		}

		String lastAfter(Steps steps) throws Exception
		{
			steps.run();
			return last();
		}

		String last()
		{
			final int id = _p.getQuestList().getLastQuestNpcObjectId();
			if (id == 0)
				return "0";
			for (Map.Entry<Integer, Npc> e : _npcs.entrySet())
				if (e.getValue().getObjectId() == id)
					return role(e.getKey());
			return "other";
		}

		String states()
		{
			final List<String> out = new ArrayList<>();
			for (QuestState st : _p.getQuestList())
			{
				final String n = st.getQuest().getName();
				if (n.equals(Q001) || n.equals(Q003) || n.equals(Q006) || n.equals(NOBLESSE))
					out.add(n + ":" + st.get(QuestState.STATE));
			}
			return out.isEmpty() ? "-" : String.join(",", out);
		}

		void end()
		{
			for (Npc n : _npcs.values())
				if (n.isVisible())
					n.deleteMe();
			drop(_p);
		}
	}

	private Scene scene(int... npcIds) throws Exception
	{
		return new Scene(npcIds);
	}

	// ---------------------------------------------------------------- timers

	/** A script whose timer hook logs every firing. */
	static final class TimerScript extends Quest
	{
		private final String _label;
		private final List<String> _log;
		private final Map<Object, String> _roles;
		private int _rearms;

		TimerScript(String label, List<String> log, Map<Object, String> roles)
		{
			super(-1, "probe");
			_label = label;
			_log = log;
			_roles = roles;
		}

		@Override
		public String onTimer(String name, Npc npc, Player player)
		{
			_log.add("fire " + _label + " " + name + " npc=" + role(npc) + " player=" + role(player) + " at=" + ThreadPool.now());
			if (name.equals("probe") || name.equals("rearm"))
				_log.add("  pending-in-hook=" + (getQuestTimer(name, npc, player) != null));
			if (name.equals("rearm") && _rearms++ == 0)
				_log.add("  restart " + name + " -> " + startQuestTimer(name, npc, player, 1000));
			return null;
		}

		private String role(Object o)
		{
			if (o == null)
				return "none";
			final String r = _roles.get(o);
			return r == null ? "other" : r;
		}
	}

	private ContractGolden timers() throws Exception
	{
		final ContractGolden g = new ContractGolden("timers.golden", "Quest timers: identity, duplicate refusal, one-shot removal before the hook, fixed rate, cancel, no liveness check.", _revision);
		final String[] src =
		{
			QUEST + "637-669 startQuestTimer",
			QUEST + "677-755 getQuestTimer cancelQuestTimers",
			GS + "scripting/QuestTimer.java 20-111 QuestTimer"
		};
		final String note = "Each row runs a fresh script (and a second one, B, where named) on probe NPCs npc1, npc2 and players p1, p2. Lines: every start with its result, then every firing (at is the virtual time since the row began) and every cancel, in order. Timers due at the same time fire in start order (the probe's thread pool; the reference pool does not order ties).";

		g.table("timers.identity", src, note, "A timer is keyed by (name, NPC, player) by identity; a null NPC or player is a key of its own, never a wildcard. Scripts keep separate timer sets.");
		timerRow(g, "duplicates", (t, b, n1, n2, p1, p2, log) ->
		{
			start(log, t, "a", n1, p1, 1000);
			start(log, t, "a", n1, p1, 2000);
			start(log, t, "a", n2, p1, 1000);
			start(log, t, "a", n1, p2, 1000);
			start(log, t, "a", null, p1, 1000);
			start(log, t, "a", n1, null, 1000);
			start(log, t, "a", null, null, 1000);
			start(log, t, "a", null, null, 1000);
			start(log, t, "b", n1, p1, 1000);
			start(log, b, "a", n1, p1, 1000);
			start(log, t, null, null, null, 1000);
			ThreadPool.advance(3000);
		});

		g.table("timers.one_shot", src, note, "A one-shot timer leaves the set before its hook runs: the hook sees no pending timer under its own key and may start the same key again.");
		timerRow(g, "removed-before-hook", (t, b, n1, n2, p1, p2, log) ->
		{
			start(log, t, "probe", n1, p1, 1000);
			ThreadPool.advance(1000);
			start(log, t, "probe", n1, p1, 1000);
			ThreadPool.advance(1000);
		});
		timerRow(g, "restart-in-hook", (t, b, n1, n2, p1, p2, log) ->
		{
			start(log, t, "rearm", n1, null, 1000);
			ThreadPool.advance(3000);
		});
		timerRow(g, "zero-delay", (t, b, n1, n2, p1, p2, log) ->
		{
			start(log, t, "now", null, p1, 0);
			ThreadPool.advance(0);
		});

		g.table("timers.fixed_rate", src, note, "startQuestTimerAtFixedRate(name, npc, player, initial, period): fires at initial, then every period on a fixed grid; it stays in the set (a restart is refused) until cancelled.");
		timerRow(g, "grid", (t, b, n1, n2, p1, p2, log) ->
		{
			log.add("start " + t._label + " tick npc=none player=" + t.role(p1) + " initial=1000 period=500 -> " + t.startQuestTimerAtFixedRate("tick", null, p1, 1000, 500));
			ThreadPool.advance(2200);
			start(log, t, "tick", null, p1, 100);
			ThreadPool.advance(300);
			log.add("cancel tick npc=none player=p1");
			t.cancelQuestTimer("tick", null, p1);
			ThreadPool.advance(1000);
		});

		g.table("timers.cancel", src, note, "Each cancel variant over the same four timers: a(npc1,p1), a(npc2,p1), b(npc1,p2), c(none,none), all due at 1000.");
		final String[] variants = { "all", "name a", "npc npc1", "player p1", "name a npc npc1", "name a player p1", "exact a npc1 p1" };
		for (String v : variants)
		{
			timerRow(g, v.replace(' ', '-'), (t, b, n1, n2, p1, p2, log) ->
			{
				start(log, t, "a", n1, p1, 1000);
				start(log, t, "a", n2, p1, 1000);
				start(log, t, "b", n1, p2, 1000);
				start(log, t, "c", null, null, 1000);
				log.add("cancel " + v);
				switch (v)
				{
					case "all" -> t.cancelQuestTimers();
					case "name a" -> t.cancelQuestTimers("a");
					case "npc npc1" -> t.cancelQuestTimers(n1);
					case "player p1" -> t.cancelQuestTimers(p1);
					case "name a npc npc1" -> t.cancelQuestTimers("a", n1);
					case "name a player p1" -> t.cancelQuestTimers("a", p1);
					case "exact a npc1 p1" -> t.cancelQuestTimer("a", n1, p1);
				}
				ThreadPool.advance(1500);
			});
		}

		g.table("timers.liveness", src, note, "No liveness check at fire time: a timer fires after its NPC is deleted and its player has left the world.");
		timerRow(g, "npc-deleted-player-gone", (t, b, n1, n2, p1, p2, log) ->
		{
			start(log, t, "late", n1, p1, 1000);
			log.add("delete npc1; remove p1 from the world");
			n1.deleteMe();
			p1.decayMe();
			World.getInstance().removeObject(p1);
			ThreadPool.advance(1000);
		});
		return g;
	}

	private interface TimerSteps
	{
		void run(TimerScript t, TimerScript b, Npc n1, Npc n2, Player p1, Player p2, List<String> log) throws Exception;
	}

	private static void start(List<String> log, TimerScript t, String name, Npc npc, Player player, long delay)
	{
		log.add("start " + t._label + " " + name + " npc=" + t.role(npc) + " player=" + t.role(player) + " delay=" + delay + " -> " + t.startQuestTimer(name, npc, player, delay));
	}

	private void timerRow(ContractGolden g, String id, TimerSteps steps) throws Exception
	{
		final List<String> log = new ArrayList<>();
		final Map<Object, String> roles = new java.util.IdentityHashMap<>();
		final TimerScript t = new TimerScript("A", log, roles);
		final TimerScript b = new TimerScript("B", log, roles);
		final Player p1 = player();
		final Player p2 = player();
		final Npc n1 = spawn(DARIN, p1, 40, 0, 0);
		final Npc n2 = spawn(DARIN, p1, -40, 0, 0);
		roles.put(p1, "p1");
		roles.put(p2, "p2");
		roles.put(n1, "npc1");
		roles.put(n2, "npc2");
		settle();
		final long t0 = ThreadPool.now();
		steps.run(t, b, n1, n2, p1, p2, log);
		t.cancelQuestTimers();
		b.cancelQuestTimers();
		g.row(id);
		for (String l : log)
		{
			final int at = l.indexOf(" at=");
			g.line(at < 0 ? l : l.substring(0, at) + " at=" + (Long.parseLong(l.substring(at + 4)) - t0));
		}
		if (n1.isVisible())
			n1.deleteMe();
		if (n2.isVisible())
			n2.deleteMe();
		drop(p1);
		drop(p2);
	}

	// ---------------------------------------------------------------- schedule calendar

	/** A scheduled script that records its start hook. */
	static final class SchedScript extends ScheduledQuest
	{
		int _starts;

		SchedScript()
		{
			super(-1, "task");
		}

		@Override
		protected void onStart()
		{
			_starts++;
		}

		@Override
		protected void onEnd()
		{
		}
	}

	private ContractGolden schedule() throws Exception
	{
		final ContractGolden g = new ContractGolden("schedule.golden", "Scheduled task calendar arithmetic for the four schedule kinds in live use.", _revision);
		g.table("schedule.calendar", new String[]
		{
			GS + "scripting/ScheduledQuest.java 38-106 setSchedule",
			GS + "scripting/ScheduledQuest.java 108-183 parseTimeStamp",
			GS + "scripting/ScheduledQuest.java 218-241 notifyAndSchedule",
			GS + "enums/ScheduleType.java 8-16 ScheduleType",
			"../aCis_datapack/data/xml/scripts.xml 978-985 task entries"
		}, "Each row calls setSchedule(type, start, null) at the virtual instant now (zone and locale given; Calendar.getInstance() and the clock read in setSchedule are the probe's), then notifyAndSchedule() three times.", "next0 is getTimeNext() after setSchedule; next1..next3 after each notifyAndSchedule(); started is the start hook count at setup (always 0: one-event scripts never start at load). Times are local ISO-8601 with offset.", "HOURLY start is minute:second. WEEK_OF_MONTH and DAY_OF_WEEK resolve with the locale's first day of week and minimal days in the first week.");
		final String[][] entries =
		{
			{ "task.SevenSignsUpdate", "HOURLY", "00:00" },
			{ "task.CastleTaxRefresh", "DAILY", "00:00:00" },
			{ "task.ClanLadderRefresh", "DAILY", "00:05:00" },
			{ "task.RecommendationUpdate", "DAILY", "13:00:00" },
			{ "task.ClanLeaderTransfer", "WEEKLY", "TUE 16:55:00" },
			{ "task.RaidPointReset", "MONTHLY_WEEK", "TUE-1 00:00:00" }
		};
		final String[] nows = { "2026-10-05T12:00:00", "2026-10-04T23:59:59", "2026-10-06T16:55:00", "2026-10-06T00:00:00", "2026-12-31T23:30:00", "2027-02-28T13:00:00", "2026-09-01T00:00:01" };
		for (String[] e : entries)
			for (String now : nows)
				schedRow(g, e, now, "UTC", Locale.US);
		for (String[] e : entries)
			schedRow(g, e, "2026-03-28T23:30:00", "Europe/Berlin", Locale.US);
		for (String[] e : entries)
			schedRow(g, e, "2026-10-24T23:30:00", "Europe/Berlin", Locale.US);
		for (String now : new String[] { "2026-10-04T12:00:00", "2026-11-01T12:00:00", "2027-01-03T12:00:00", "2027-02-01T12:00:00" })
		{
			schedRow(g, entries[4], now, "UTC", Locale.GERMANY);
			schedRow(g, entries[5], now, "UTC", Locale.GERMANY);
			schedRow(g, entries[5], now, "UTC", Locale.US);
		}
		ProbeClock.clear();
		return g;
	}

	private void schedRow(ContractGolden g, String[] e, String now, String zone, Locale locale)
	{
		final ZoneId z = ZoneId.of(zone);
		final long ms = LocalDateTime.parse(now).atZone(z).toInstant().toEpochMilli();
		ProbeClock.set(ms, TimeZone.getTimeZone(z), locale);
		final SchedScript s = new SchedScript();
		final boolean ok = s.setSchedule(e[1], e[2], null);
		final List<Object> kv = new ArrayList<>(List.of("task", e[0], "type", e[1], "start", e[2], "now", now, "zone", zone, "locale", locale.toLanguageTag(), "ok", ok, "started", s._starts, "next0", iso(s.getTimeNext(), z)));
		for (int i = 1; i <= 3; i++)
		{
			s.notifyAndSchedule();
			kv.add("next" + i);
			kv.add(iso(s.getTimeNext(), z));
		}
		g.row(e[0].substring(5) + "@" + now + "@" + zone + "@" + locale.toLanguageTag(), kv.toArray());
		ProbeClock.clear();
	}

	private static String iso(long ms, ZoneId z)
	{
		return DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(Instant.ofEpochMilli(ms).atZone(z));
	}


	// ---------------------------------------------------------------- fan-out

	/** A script that logs every party and clan call it receives. */
	static final class FanScript extends Quest
	{
		List<String> _log = new ArrayList<>();
		Map<Object, String> _roles = new java.util.IdentityHashMap<>();

		FanScript()
		{
			super(-1, "probe");
		}

		private String r(Object o)
		{
			if (o == null)
				return "none";
			final String v = _roles.get(o);
			return v == null ? "other" : v;
		}

		@Override
		public void onAttacked(Npc npc, Creature attacker, int damage, L2Skill skill)
		{
			_log.add("ATTACKED npc=" + r(npc) + " attacker=" + r(attacker) + " damage=" + damage + " skill=" + (skill == null ? "none" : skill.getId()));
		}

		@Override
		public void onPartyAttacked(Npc caller, Npc called, Creature target, int damage)
		{
			_log.add("PARTY_ATTACKED caller=" + r(caller) + " called=" + r(called) + " target=" + r(target) + " damage=" + damage);
		}

		@Override
		public void onClanAttacked(Npc caller, Npc called, Creature attacker, int damage, L2Skill skill)
		{
			_log.add("CLAN_ATTACKED caller=" + r(caller) + " called=" + r(called) + " attacker=" + r(attacker) + " damage=" + damage + " skill=" + (skill == null ? "none" : skill.getId()));
		}

		@Override
		public void onPartyDied(Npc caller, Npc called)
		{
			_log.add("PARTY_DIED caller=" + r(caller) + " called=" + r(called));
		}

		@Override
		public void onClanDied(Npc caller, Npc called, Creature killer)
		{
			_log.add("CLAN_DIED caller=" + r(caller) + " called=" + r(called) + " killer=" + r(killer));
		}
	}

	private static final int F_MASTER = 20030;
	private static final int F_MINION1 = 20031;
	private static final int F_MINION2 = 20040;
	private static final int F_CALLER = 20041;
	private static final int F_SAME = 20042;
	private static final int F_DEAD = 20043;
	private static final int F_OTHER = 20044;
	private static final int F_IGNORING = 20045;
	private static final int F_FAR = 20046;
	private static final int F_FOLK = DARIN;
	private static final int SLOW = 1160;

	private FanScript _fan;

	private void fanTemplate(int id, String[] clans, int range, int[] ignored, boolean clearAll) throws Exception
	{
		final NpcTemplate t = NpcData.getInstance().getTemplate(id);
		final EventHandler[] events = { EventHandler.ATTACKED, EventHandler.PARTY_ATTACKED, EventHandler.CLAN_ATTACKED, EventHandler.PARTY_DIED, EventHandler.CLAN_DIED };
		if (clearAll)
			t.getEventQuests().clear();
		for (EventHandler e : events)
		{
			t.getEventQuests().remove(e);
			t.addQuestEvent(e, _fan);
		}
		field(t, "_clans", clans);
		field(t, "_clanRange", range);
		field(t, "_ignoredIds", ignored);
	}

	private static void field(Object o, String name, Object value) throws Exception
	{
		final Field f = o.getClass().getDeclaredField(name);
		f.setAccessible(true);
		f.set(o, value);
	}

	private ContractGolden fanout() throws Exception
	{
		_fan = new FanScript();
		final String[] clan = { "probe_clan" };
		fanTemplate(F_MASTER, null, 0, null, true);
		fanTemplate(F_MINION1, null, 0, null, true);
		fanTemplate(F_MINION2, null, 0, null, true);
		fanTemplate(F_CALLER, clan, 300, null, true);
		fanTemplate(F_SAME, clan, 300, null, true);
		fanTemplate(F_DEAD, clan, 300, null, true);
		fanTemplate(F_OTHER, new String[] { "other_clan" }, 300, null, true);
		fanTemplate(F_IGNORING, clan, 300, new int[] { F_CALLER }, true);
		fanTemplate(F_FAR, clan, 300, null, true);
		fanTemplate(F_FOLK, clan, 300, null, false);

		final L2Skill slow = SkillTable.getInstance().getInfo(SLOW, 1);
		final ContractGolden g = new ContractGolden("fanout.golden", "Attacked, party and clan fan-out per source: hit, aggression effect, skill, death.", _revision);
		final String roles = "Roles: M master (20030) with minions m1 (20031) and m2 (20040), linked with setMaster and the master's minion set; C caller (20041, clan probe_clan, clan range 300); neighbours of C within 100: same (20042, same clan), dead (20043, same clan, dead), other (20044, other clan), ignoring (20045, same clan, ignores 20041), folk (Darin 30048 set to the same clan, where listed); far (20046, same clan) at 450. dead roles are flagged dead without dying. p is the player.";
		final String los = "The probe has no geodata and the scene stands on its ground (Z 0), so every line-of-sight check passes; the line-of-sight gate is pinned by the hand table fanout.line_of_sight.";
		final String order = "Lines are every script call, in order. The minion set and the known list are unordered; every row has at most one eligible minion or neighbour per loop, so the order is fixed.";

		g.table("fanout.hit", new String[] { NPC + "389-469 reduceCurrentHp" }, "Each row: reduceCurrentHp(10, p, null) on the target.", roles, los, order);
		fanRow(g, "solo", "M", new int[] { F_MASTER }, "", () -> _fanNpc.get("M").reduceCurrentHp(10, _fanPlayer, null));
		fanRow(g, "master", "M", new int[] { F_MASTER, F_MINION1, F_MINION2 }, "party m2:dead", () -> _fanNpc.get("M").reduceCurrentHp(10, _fanPlayer, null));
		fanRow(g, "minion", "m1", new int[] { F_MASTER, F_MINION1, F_MINION2 }, "party", () -> _fanNpc.get("m1").reduceCurrentHp(10, _fanPlayer, null));
		fanRow(g, "minion-master-dead", "m1", new int[] { F_MASTER, F_MINION1, F_MINION2 }, "party M:dead", () -> _fanNpc.get("m1").reduceCurrentHp(10, _fanPlayer, null));
		fanRow(g, "dead-target", "m1", new int[] { F_MASTER, F_MINION1, F_MINION2 }, "party m1:dead", () -> _fanNpc.get("m1").reduceCurrentHp(10, _fanPlayer, null));
		fanRow(g, "clan", "C", new int[] { F_CALLER, F_SAME, F_DEAD, F_OTHER, F_IGNORING, F_FAR }, "dead:dead", () -> _fanNpc.get("C").reduceCurrentHp(10, _fanPlayer, null));
		fanRow(g, "clan-folk", "C", new int[] { F_CALLER, F_FOLK, F_DEAD, F_OTHER, F_IGNORING, F_FAR }, "dead:dead", () -> _fanNpc.get("C").reduceCurrentHp(10, _fanPlayer, null));
		fanRow(g, "clan-range-zero", "C", new int[] { F_CALLER, F_SAME }, "C:range0", () -> _fanNpc.get("C").reduceCurrentHp(10, _fanPlayer, null));

		g.table("fanout.aggression", new String[] { GS + "model/actor/ai/type/AttackableAI.java 118-184 onEvtAggression" }, "Each row: the target's AI receives AGGRESSION from p with aggro 50 (what an aggression effect sends); no HP change and no skill.", roles, los, order);
		fanRow(g, "master", "M", new int[] { F_MASTER, F_MINION1 }, "party", () -> aggression(_fanNpc.get("M")));
		fanRow(g, "master-dead-minion", "M", new int[] { F_MASTER, F_MINION1, F_MINION2 }, "party m2:dead", () -> aggression(_fanNpc.get("M")));
		fanRow(g, "minion", "m1", new int[] { F_MASTER, F_MINION1 }, "party", () -> aggression(_fanNpc.get("m1")));
		fanRow(g, "minion-master-dead", "m1", new int[] { F_MASTER, F_MINION1 }, "party M:dead", () -> aggression(_fanNpc.get("m1")));
		fanRow(g, "clan", "C", new int[] { F_CALLER, F_SAME, F_DEAD, F_OTHER, F_IGNORING, F_FAR }, "dead:dead", () -> aggression(_fanNpc.get("C")));

		g.table("fanout.skill", new String[] { GS + "model/actor/cast/CreatureCast.java 454-566 callSkill" }, "Each row: p's cast calls callSkill(Slow 1160 level 1, [target], null); Slow is offensive, debuff=" + slow.isDebuff() + ", aggro points " + slow.getAggroPoints() + ", so the value passed is max(120, aggro points).", roles, los, order);
		fanRow(g, "master", "M", new int[] { F_MASTER, F_MINION1, F_MINION2 }, "party m2:dead", () -> skill(slow, _fanNpc.get("M")));
		fanRow(g, "minion", "m1", new int[] { F_MASTER, F_MINION1, F_MINION2 }, "party", () -> skill(slow, _fanNpc.get("m1")));
		fanRow(g, "minion-master-dead", "m1", new int[] { F_MASTER, F_MINION1, F_MINION2 }, "party M:dead", () -> skill(slow, _fanNpc.get("m1")));
		fanRow(g, "dead-target", "m1", new int[] { F_MASTER, F_MINION1 }, "party m1:dead", () -> skill(slow, _fanNpc.get("m1")));
		fanRow(g, "clan", "C", new int[] { F_CALLER, F_SAME, F_FOLK, F_DEAD, F_OTHER, F_IGNORING, F_FAR }, "dead:dead", () -> skill(slow, _fanNpc.get("C")));

		g.table("fanout.party_died", new String[] { NPC + "471-527 doDie" }, "Each row: doDie(p) on the dying NPC. after lists the master links afterwards.", roles, los, order);
		fanRow(g, "master-dies", "M", new int[] { F_MASTER, F_MINION1 }, "party", () -> _fanNpc.get("M").doDie(_fanPlayer));
		fanRow(g, "master-dies-dead-minion", "M", new int[] { F_MASTER, F_MINION2 }, "party m2:dead", () -> _fanNpc.get("M").doDie(_fanPlayer));
		fanRow(g, "minion-dies", "m1", new int[] { F_MASTER, F_MINION1 }, "party", () -> _fanNpc.get("m1").doDie(_fanPlayer));
		fanRow(g, "minion-dies-master-dead", "m1", new int[] { F_MASTER, F_MINION1 }, "party M:dead", () -> _fanNpc.get("m1").doDie(_fanPlayer));

		g.table("fanout.clan_range", new String[]
		{
			NPC + "442-463 forEachKnownTypeInRadius in the reduceCurrentHp clan scan",
			GS + "model/WorldObject.java 549-558 forEachKnownTypeInRadius",
			"java/net/sf/l2j/commons/math/MathUtil.java 182-217 checkIfInRange"
		}, "The clan scan's range: 3D, inclusive (<=), clan range plus both collision radii. Each row: C and same only, same placed dx from C (same Y and Z); reduceCurrentHp(10, p, null) on C. radii is C's plus same's collision radius.", los);
		{
			final double radii = NpcData.getInstance().getTemplate(F_CALLER).getCollisionRadius() + NpcData.getInstance().getTemplate(F_SAME).getCollisionRadius();
			final int edge = (int) Math.floor(300 + radii);
			for (int dx : new int[] { 299, 300, edge, edge + 1 })
				clanRangeRow(g, dx, radii);
		}

		g.table("fanout.clan_died", new String[] { NPC + "529-555 CLAN_DIED in doDie" }, "Each row: doDie(p) on C.", roles, los, order);
		fanRow(g, "clan", "C", new int[] { F_CALLER, F_SAME, F_DEAD, F_OTHER, F_IGNORING, F_FAR }, "dead:dead", () -> _fanNpc.get("C").doDie(_fanPlayer));
		fanRow(g, "clan-folk", "C", new int[] { F_CALLER, F_FOLK }, "", () -> _fanNpc.get("C").doDie(_fanPlayer));
		return g;
	}

	private final Map<String, Npc> _fanNpc = new java.util.LinkedHashMap<>();
	private Player _fanPlayer;

	private void aggression(Npc npc)
	{
		npc.getAI().notifyEvent(net.sf.l2j.gameserver.enums.AiEventType.AGGRESSION, _fanPlayer, 50);
	}

	private void skill(L2Skill skill, Npc target)
	{
		_fanPlayer.getCast().callSkill(skill, new Creature[] { target }, null);
	}

	private interface FanOp
	{
		void run() throws Exception;
	}

	private String fanRole(int id)
	{
		return switch (id)
		{
			case F_MASTER -> "M";
			case F_MINION1 -> "m1";
			case F_MINION2 -> "m2";
			case F_CALLER -> "C";
			case F_SAME -> "same";
			case F_DEAD -> "dead";
			case F_OTHER -> "other";
			case F_IGNORING -> "ignoring";
			case F_FAR -> "far";
			case F_FOLK -> "folk";
			default -> String.valueOf(id);
		};
	}

	/**
	 * setup: "party" links m1 and m2 (when spawned) to M; "role:dead" flags a role dead;
	 * "C:range0" sets the caller template's clan range to 0 for the row.
	 */
	private void fanRow(ContractGolden g, String id, String target, int[] ids, String setup, FanOp op) throws Exception
	{
		_fanNpc.clear();
		_fan._log.clear();
		_fan._roles.clear();
		_fanPlayer = player();
		// Without geodata every cell is ground at Z 0; standing on it keeps line of sight clear.
		_fanPlayer.setXYZ(_fanPlayer.getX(), _fanPlayer.getY(), 0);
		_fan._roles.put(_fanPlayer, "p");
		int dx = -100;
		for (int npcId : ids)
		{
			final String role = fanRole(npcId);
			final Npc npc = role.equals("far") ? spawn(npcId, _fanPlayer, 450, 0, 0) : spawn(npcId, _fanPlayer, dx, 50, 0);
			if (!role.equals("far"))
				dx += 25;
			net.sf.l2j.gameserver.taskmanager.AiTaskManager.getInstance().remove(npc);
			_fanNpc.put(role, npc);
			_fan._roles.put(npc, role);
		}
		settle();
		final List<String> parts = new ArrayList<>(List.of(setup.split(" ")));
		if (parts.contains("party") || parts.stream().anyMatch(s -> s.startsWith("m")))
		{
			final Npc m = _fanNpc.get("M");
			for (String r : new String[] { "m1", "m2" })
			{
				final Npc n = _fanNpc.get(r);
				if (m != null && n != null)
				{
					n.setMaster(m);
					m.getMinions().add(n);
				}
			}
		}
		int callerRange = -1;
		for (String part : parts)
		{
			if (part.endsWith(":dead"))
				_fanNpc.get(part.substring(0, part.length() - 5)).setIsDead(true);
			if (part.equals("C:range0"))
			{
				final NpcTemplate t = NpcData.getInstance().getTemplate(F_CALLER);
				callerRange = t.getClanRange();
				field(t, "_clanRange", 0);
			}
		}
		op.run();
		final List<String> lines = new ArrayList<>(_fan._log);
		if (callerRange >= 0)
			field(NpcData.getInstance().getTemplate(F_CALLER), "_clanRange", callerRange);
		final List<String> after = new ArrayList<>();
		for (Map.Entry<String, Npc> e : _fanNpc.entrySet())
			if (e.getKey().startsWith("m"))
				after.add(e.getKey() + ".master=" + _fan.r(e.getValue().getMaster()));
		g.row(id, "target", target, "npcs", String.join(",", _fanNpc.keySet()), "setup", setup.isEmpty() ? "-" : setup.replace(' ', ','), "after", after.isEmpty() ? "-" : String.join(",", after));
		g.lines(lines);
		for (Npc n : _fanNpc.values())
		{
			n.setIsDead(false);
			if (n.isVisible())
				n.deleteMe();
		}
		drop(_fanPlayer);
	}

	private void clanRangeRow(ContractGolden g, int dx, double radii) throws Exception
	{
		_fan._log.clear();
		_fan._roles.clear();
		final Player p = player();
		p.setXYZ(p.getX(), p.getY(), 0);
		final Npc c = spawn(F_CALLER, p, 0, 50, 0);
		final Npc same = spawn(F_SAME, p, dx, 50, 0);
		net.sf.l2j.gameserver.taskmanager.AiTaskManager.getInstance().remove(c);
		net.sf.l2j.gameserver.taskmanager.AiTaskManager.getInstance().remove(same);
		_fan._roles.put(p, "p");
		_fan._roles.put(c, "C");
		_fan._roles.put(same, "same");
		c.reduceCurrentHp(10, p, null);
		g.row("dx-" + dx, "dx", dx, "radii", String.format(Locale.ROOT, "%.1f", radii), "called", _fan._log.stream().anyMatch(l -> l.contains("called=same")));
		g.lines(new ArrayList<>(_fan._log));
		c.deleteMe();
		same.deleteMe();
		drop(p);
	}

	// ---------------------------------------------------------------- first-aggro tick

	/** A see-creature script; when attack is set it adds an attack desire of 200 on the seen creature. */
	static final class SeeScript extends Quest
	{
		final List<String> _log = new ArrayList<>();
		final Map<Object, String> _roles = new java.util.IdentityHashMap<>();
		boolean _attack;

		SeeScript()
		{
			super(-1, "probe");
		}

		@Override
		public void onSeeCreature(Npc npc, Creature creature)
		{
			final String who = _roles.getOrDefault(creature, "other");
			_log.add("  see " + who + " lifetime=" + npc.getAI().getLifeTime() + " intention=" + npc.getAI().getCurrentIntention().getType());
			if (_attack)
			{
				npc.getAI().addAttackDesire(creature, 200);
				_log.add("  add attack desire on " + who + " -> intention=" + npc.getAI().getCurrentIntention().getType());
			}
		}
	}

	private static final int AGGRO_NPC = 20049;

	private ContractGolden aggroTick() throws Exception
	{
		final SeeScript see = new SeeScript();
		final NpcTemplate t = NpcData.getInstance().getTemplate(AGGRO_NPC);
		t.getEventQuests().clear();
		t.addQuestEvent(EventHandler.SEE_CREATURE, see);

		final ContractGolden g = new ContractGolden("aggro_tick.golden", "The see-creature tick scan and the tick on which a first attack desire turns into the attack intention.", _revision);
		g.table("aggro.first_tick", new String[]
		{
			GS + "model/actor/ai/type/NpcAI.java 449-575 runAI",
			GS + "model/actor/ai/type/NpcAI.java 698-726 addAttackDesire"
		}, "An NPC (20049, its scripts replaced by one see-creature script) is taken off the AI task manager; each tick is one runAI() call (the task manager's 1 s call). The script, where attack is on, adds an attack desire of 200 on the seen creature, as the aggressive behaviors' tryToAttack does. Lines: per tick, the see-creature calls inside it, then the intention and lifetime after it.", "addAttackDesire runs a nested runAI(false) when the aggro list has no most-hated creature; runAI decides whether it may pick a desire (instantRun) before the see-creature scan, from the lifetime and the queued desires. Default see range " + Config.DEFAULT_SEE_RANGE + "; p is in range at 100, out of range at 1000.");
		aggroRow(g, see, "fresh-npc-sees-attacks", true, "in", 0, false, new String[] { "tick", "tick", "tick" });
		aggroRow(g, see, "aged-npc-sees-attacks", true, "out", 3, false, new String[] { "in", "tick", "tick" });
		aggroRow(g, see, "fresh-npc-aggro-list-held", true, "in", 0, true, new String[] { "tick", "tick", "tick" });
		aggroRow(g, see, "aged-npc-aggro-list-held", true, "out", 3, true, new String[] { "in", "tick", "tick" });
		aggroRow(g, see, "queued-attack-before-first-tick", false, "in", 0, false, new String[] { "desire", "tick", "tick" });

		g.table("aggro.see_once", new String[]
		{
			GS + "model/actor/ai/type/NpcAI.java 449-496 runAI see-creature scan"
		}, "The same NPC with attack off: a non-raid NPC calls see-creature once per creature while it stays in range; leaving the range forgets it, and coming back calls again.");
		aggroRow(g, see, "in-out-in", false, "in", 0, false, new String[] { "tick", "tick", "out", "tick", "in", "tick" });
		return g;
	}

	private void aggroRow(ContractGolden g, SeeScript see, String id, boolean attack, String start, int age, boolean held, String[] steps) throws Exception
	{
		see._log.clear();
		see._roles.clear();
		see._attack = attack;
		final Player p = player();
		final Player other = player();
		see._roles.put(p, "p");
		see._roles.put(other, "p2");
		final int px = p.getX();
		final int py = p.getY();
		final int pz = p.getZ();
		final Npc npc = spawn(AGGRO_NPC, p, 0, 0, 0);
		npc.setXYZ(px + 100, py, pz);
		net.sf.l2j.gameserver.taskmanager.AiTaskManager.getInstance().remove(npc);
		other.setXYZ(px + 100, py + 2000, pz);
		p.setXYZ(start.equals("in") ? px : px - 900, py, pz);
		for (int i = 0; i < age; i++)
			npc.getAI().runAI();
		if (held)
			npc.getAI().getAggroList().addDamageHate(other, 0, 1);
		final List<String> lines = new ArrayList<>();
		lines.add("setup lifetime=" + npc.getAI().getLifeTime() + " intention=" + npc.getAI().getCurrentIntention().getType() + " p=" + start + (held ? " aggro-list holds p2" : ""));
		int tick = 0;
		for (String step : steps)
		{
			switch (step)
			{
				case "tick" ->
				{
					see._log.clear();
					npc.getAI().runAI();
					lines.add("tick " + (++tick));
					lines.addAll(see._log);
					lines.add("  after intention=" + npc.getAI().getCurrentIntention().getType() + " lifetime=" + npc.getAI().getLifeTime());
				}
				case "in" ->
				{
					p.setXYZ(px, py, pz);
					lines.add("p moves in range");
				}
				case "out" ->
				{
					p.setXYZ(px - 900, py, pz);
					lines.add("p moves out of range");
				}
				case "desire" ->
				{
					npc.getAI().addAttackDesire(p, 200);
					lines.add("attack desire on p added outside a tick -> intention=" + npc.getAI().getCurrentIntention().getType());
				}
			}
		}
		g.row(id, "attack", attack, "start", start, "age", age, "aggro_list_held", held);
		g.lines(lines);
		if (npc.isVisible())
			npc.deleteMe();
		drop(p);
		drop(other);
	}

	// ---------------------------------------------------------------- helpers

	/** Records every packet sent to one player and every SQL statement until {@link #stop}. */
	private static final class Recorder
	{
		private final List<String> _raw = new ArrayList<>();

		Recorder(Player p)
		{
			final GameClient c = p.getClient();
			ProbeWire.setSink((client, line) ->
			{
				if (client == c)
					_raw.add("S" + line);
			});
			ConnectionPool.setSink(sql -> _raw.add("Q" + sql));
		}

		List<String> stop(Map<Integer, String> roles)
		{
			ProbeWire.setSink(null);
			ConnectionPool.setSink(null);
			final List<String> out = new ArrayList<>();
			for (String r : _raw)
				out.add(r.charAt(0) == 'S' ? ContractGolden.packet(r.substring(1), roles) : ContractGolden.sql(r.substring(1), roles));
			return out;
		}
	}

	private Player player() throws Exception
	{
		final int spot = _spot++;
		final Player player = Player.create(IdFactory.getInstance().getNextId(), PlayerData.getInstance().getTemplate(ClassId.HUMAN_FIGHTER), "probe", "Contract" + (++_players), (byte) 0, (byte) 0, (byte) 0, Sex.MALE);
		player.getStatus().setLevel(10);
		player.getStatus().setExp(PlayerLevelData.getInstance().getPlayerLevel(10).requiredExpToLevelUp());
		player.getStatus().setMaxHpMp();
		World.getInstance().addObject(player);

		final GameClient client = new GameClient(null);
		client.setState(GameClientState.IN_GAME);
		client.setPlayer(player);
		player.setClient(client);
		player.spawnMe(X + (spot % 40) * 500, Y + (spot / 40) * 500, Z);
		settle();
		return player;
	}

	private Npc spawn(int npcId, Player near, int dx, int dy, int dz) throws Exception
	{
		final Spawn spawn = new Spawn(NpcData.getInstance().getTemplate(npcId));
		spawn.setLoc(near.getX() + dx, near.getY() + dy, near.getZ() + dz, 0);
		final Npc npc = spawn.doSpawn(false);
		npc.setXYZ(near.getX() + dx, near.getY() + dy, near.getZ() + dz);
		return npc;
	}

	private static void drop(Player p)
	{
		ProbeWire.setSink(null);
		ConnectionPool.setSink(null);
		p.decayMe();
		World.getInstance().removeObject(p);
		settle();
	}

	/** Runs every task that is due, with recording off, so deferred updates of a setup step stay out of the next recording. */
	private static void settle()
	{
		ThreadPool.advance(1000);
	}

	private static Quest quest(String name)
	{
		final Quest q = ScriptData.getInstance().getQuest(name);
		if (q == null)
			throw new IllegalStateException("no script " + name);
		return q;
	}

	private static String or(String v)
	{
		return v == null ? "-" : v;
	}

	private static String hex(int v)
	{
		return String.format("0x%08x", v);
	}

	private static String ints(int[] v)
	{
		if (v.length == 0)
			return "-";
		final List<String> s = new ArrayList<>();
		for (int i : v)
			s.add(String.valueOf(i));
		return String.join(",", s);
	}

	private static String vars(QuestState st)
	{
		if (st == null)
			return "-";
		final TreeMap<String, String> m = new TreeMap<>(st);
		final List<String> out = new ArrayList<>();
		for (Map.Entry<String, String> e : m.entrySet())
			out.add(e.getKey() + ":" + e.getValue());
		return out.isEmpty() ? "-" : String.join(",", out);
	}

}
