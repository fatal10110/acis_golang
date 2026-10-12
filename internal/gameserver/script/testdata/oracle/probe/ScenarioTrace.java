import java.io.File;
import java.io.PrintWriter;
import java.lang.reflect.Field;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.EnumMap;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;

import net.sf.l2j.commons.mmocore.ProbeWire;
import net.sf.l2j.commons.probe.ProbeClock;
import net.sf.l2j.commons.pool.ConnectionPool;
import net.sf.l2j.commons.pool.ThreadPool;
import net.sf.l2j.commons.random.Rnd;

import net.sf.l2j.gameserver.data.manager.SevenSignsManager;
import net.sf.l2j.gameserver.data.sql.ClanTable;
import net.sf.l2j.gameserver.data.sql.PlayerInfoTable;
import net.sf.l2j.gameserver.data.xml.ItemData;
import net.sf.l2j.gameserver.data.xml.NpcData;
import net.sf.l2j.gameserver.data.xml.PlayerData;
import net.sf.l2j.gameserver.data.xml.PlayerLevelData;
import net.sf.l2j.gameserver.data.xml.ScriptData;
import net.sf.l2j.gameserver.enums.CabalType;
import net.sf.l2j.gameserver.enums.EventHandler;
import net.sf.l2j.gameserver.enums.SealType;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.NpcSkillType;
import net.sf.l2j.gameserver.enums.actors.Sex;
import net.sf.l2j.gameserver.idfactory.IdFactory;
import net.sf.l2j.gameserver.model.World;
import net.sf.l2j.gameserver.model.WorldObject;
import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.template.CreatureTemplate;
import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.pledge.Clan;
import net.sf.l2j.gameserver.model.spawn.Spawn;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.network.GameClient.GameClientState;
import net.sf.l2j.gameserver.network.SessionKey;
import net.sf.l2j.gameserver.network.clientpackets.Action;
import net.sf.l2j.gameserver.network.clientpackets.EnterWorld;
import net.sf.l2j.gameserver.network.clientpackets.L2GameClientPacket;
import net.sf.l2j.gameserver.network.clientpackets.RequestBypassToServer;
import net.sf.l2j.gameserver.network.clientpackets.RequestCharacterCreate;
import net.sf.l2j.gameserver.network.clientpackets.RequestGameStart;
import net.sf.l2j.gameserver.network.clientpackets.RequestLinkHtml;
import net.sf.l2j.gameserver.network.clientpackets.RequestRestart;
import net.sf.l2j.gameserver.network.clientpackets.RequestTutorialQuestionMark;
import net.sf.l2j.gameserver.scripting.Quest;
import net.sf.l2j.gameserver.scripting.QuestState;
import net.sf.l2j.gameserver.taskmanager.AiTaskManager;
import net.sf.l2j.gameserver.taskmanager.GameTimeTaskManager;

/**
 * Quest scenario traces: each scenario file of the Go quest suite whose {@code trace} line names
 * a file under {@code oracle/scenarios/} is played here, from the scenario itself, and its trace
 * is written to that file. The header sets the player up as the Go runner does (name, level,
 * class, a led clan of the given level, quest rows, the script draws of the random line) and
 * spawns each NPC line's NPC at its offset from the player, who stands where the Go fixture
 * character enters the world. The hold, equip, standing, pk, cabal and festival lines are seeded
 * as the Go runner seeds them. The NPCs keep only the bindings of the scenario's script and stub
 * lines, as the Go runner registers no other script. A monster the scenario kills or hits is made the Go runner's
 * fixture monster: level 1, 1000 HP, no experience, SP, drops, skills, minions or aggression.
 * <p>
 * The character, item, quest and memo rows the scenario writes are kept (see the probe's
 * connection pool), so a relog stores the character at its logout and restores it from them.
 * Every step is played: action, bypass, link, mark, relog, restart, enter and create through the
 * client packets, kill (each of N kills alone, on a fresh monster once the last one died), hit,
 * advance, pickup (the item's action, then time until it left the ground), approach and tick
 * (one run of the NPC AI task).
 */
final class ScenarioTrace
{
	/** Where the Go fixture character enters the world. */
	private static final int X = 10;
	private static final int Y = 20;
	private static final int Z = 30;

	/** The trace line prefix that marks a scenario this probe traces. */
	private static final String TRACED = "internal/gameserver/script/testdata/oracle/scenarios/";

	/** Virtual time passed after each client packet: one run of every 333 ms task. */
	private static final long STEP = 500;

	/** The kill wait of the Go runner: the dying hooks run 3 s after the death. */
	private static final int KILL_TICKS = 7;

	private static int _clans;

	/** Scenarios played so far: each one's character has an account of its own. */
	private static int _accounts;

	/** The tables whose rows a scenario keeps. */
	private static final String[] KEPT =
	{
		"characters",
		"items",
		"character_quests",
		"character_memo"
	};

	/** The wall clock each scenario starts at: 2026-01-01 00:00 UTC. */
	private static final long WALL = 1_767_225_600_000L;

	/** The game time each scenario starts at, in game minutes: noon of the first game day. */
	private static final int GAME_TIME = 12 * 60;

	/** Pickup waits at most this many ticks for the item to leave the ground. */
	private static final int PICKUP_TICKS = 20;

	private final String _name;
	private final List<String> _lines = new ArrayList<>();
	private final Map<String, NpcLine> _npcs = new LinkedHashMap<>();
	private final Map<String, Npc> _live = new HashMap<>();
	/** Every NPC the probe spawned for an NPC line, wherever it stands now. */
	private final List<Npc> _spawnedNpcs = new ArrayList<>();
	private final List<String[]> _quests = new ArrayList<>();
	private final List<String[]> _steps = new ArrayList<>();
	/** The script and stub lines' classes: the scripts the scenario runs. */
	private final Set<String> _scripts = new HashSet<>();
	/** Header lines the probe does not read: a traced scenario must have none. */
	private final List<String> _unread = new ArrayList<>();
	/** The NPC templates' own script bindings, kept while the scenario's replace them. */
	private final Map<NpcTemplate, Map<EventHandler, List<Quest>>> _bindings = new HashMap<>();
	private String _trace;
	private String _character;
	private int _level;
	private int _class;
	private int _clan = -1;
	private int[] _draws;
	private boolean _always;
	private final List<int[]> _holds = new ArrayList<>();
	private final List<Integer> _equips = new ArrayList<>();
	private int _standing;
	private int _pk;
	private String[] _cabal;
	private int[] _festival;

	private GameClient _client;
	private Player _player;
	/** The player's object id, kept across a relog. */
	private int _objectId;
	/** Whether the NPC lines' NPCs stand: once the character first entered the world. */
	private boolean _spawned;
	/** While true, packets and statements are not recorded: the probe's own setup. */
	private boolean _muted;

	private record NpcLine(String role, int id, String type, int dx, int dy, int dz)
	{
	}

	private ScenarioTrace(String name)
	{
		_name = name;
	}

	/**
	 * The script bindings of every NPC template as the script loader left them, copied before
	 * the contract scenes change some: each scenario's bindings are drawn from these.
	 */
	static Map<NpcTemplate, Map<EventHandler, List<Quest>>> snapshot()
	{
		final Map<NpcTemplate, Map<EventHandler, List<Quest>>> loaded = new HashMap<>();
		for (NpcTemplate t : NpcData.getInstance().getTemplates())
		{
			final Map<EventHandler, List<Quest>> copy = new EnumMap<>(EventHandler.class);
			for (Map.Entry<EventHandler, List<Quest>> e : t.getEventQuests().entrySet())
				copy.put(e.getKey(), List.copyOf(e.getValue()));
			loaded.put(t, copy);
		}
		return loaded;
	}

	static void run(File scenarios, File out, Map<NpcTemplate, Map<EventHandler, List<Quest>>> loaded) throws Exception
	{
		final File[] files = scenarios.listFiles((d, n) -> n.endsWith(".scenario"));
		if (files == null)
			throw new IllegalStateException("no scenario directory " + scenarios);
		Arrays.sort(files);
		for (File f : files)
		{
			final ScenarioTrace t = new ScenarioTrace(f.getName());
			t.parse(Files.readAllLines(f.toPath(), StandardCharsets.UTF_8));
			if (t._trace == null || !t._trace.startsWith(TRACED))
				continue;
			if (!t._unread.isEmpty())
				throw new IllegalStateException(t._name + ": header lines the probe does not read: " + t._unread);

			// Drop what an earlier scenario left queued, so no task of it runs here.
			ThreadPool.cancelAll();
			Rnd.reseed(1);
			if (t._draws != null)
				Rnd.scriptCallers(t._draws, t._always, d -> t._lines.add("# draw " + d));
			t.restrict(loaded);
			ConnectionPool.keep(KEPT);
			ProbeClock.virtual(WALL);
			set(GameTimeTaskManager.class, GameTimeTaskManager.getInstance(), "_time", GAME_TIME);
			set(GameTimeTaskManager.class, GameTimeTaskManager.getInstance(), "_isNight", false);
			final int[] festival = festival();
			final List<String> failures;
			try
			{
				t.play();
			}
			finally
			{
				Rnd.endScriptCallers();
				stop();
				t.restore();
				failures = ConnectionPool.forget();
				ProbeClock.real();
				setFestival(festival);
			}
			if (!failures.isEmpty())
				throw new IllegalStateException(t._name + ": statements the probe store cannot run: " + failures);
			final File target = new File(out, "scenarios/" + t._trace.substring(TRACED.length()));
			target.getParentFile().mkdirs();
			t.write(target);
			t.clear();
		}
	}

	/** Reads the header lines the probe needs and the step verb lines. */
	private void parse(List<String> lines)
	{
		boolean steps = false;
		for (String raw : lines)
		{
			final String line = raw.strip();
			if (line.isEmpty() || line.startsWith("#") || raw.startsWith(" ") || raw.startsWith("\t"))
				continue;
			final String[] f = line.split("\\s+");
			if (steps || isStep(f[0]))
			{
				steps = true;
				// A bypass keeps its own spacing.
				_steps.add(f[0].equals("bypass") ? new String[] { "bypass", line.substring("bypass".length()).strip() } : f);
				continue;
			}
			switch (f[0])
			{
				case "trace" -> _trace = f[1];
				case "character" ->
				{
					_character = f[1];
					_level = Integer.parseInt(f[2]);
				}
				case "class" -> _class = Integer.parseInt(f[1]);
				case "clan" -> _clan = Integer.parseInt(f[1]);
				case "quest" -> _quests.add(f);
				case "random" ->
				{
					_always = f[1].equals("always");
					_draws = Arrays.stream(f, _always ? 2 : 1, f.length).mapToInt(Integer::parseInt).toArray();
				}
				case "npc" -> _npcs.put(f[1], new NpcLine(f[1], Integer.parseInt(f[2]), f[3], Integer.parseInt(f[4]), Integer.parseInt(f[5]), Integer.parseInt(f[6])));
				case "script", "stub" -> _scripts.add(ScriptProbe.PKG + f[1]);
				case "hold" -> _holds.add(new int[]
				{
					Integer.parseInt(f[1]),
					Integer.parseInt(f[2])
				});
				case "equip" -> _equips.add(Integer.parseInt(f[1]));
				case "standing" -> _standing = Integer.parseInt(f[1]);
				case "pk" -> _pk = Integer.parseInt(f[1]);
				case "cabal" -> _cabal = new String[]
				{
					f[1],
					f[2]
				};
				case "festival" -> _festival = new int[]
				{
					Integer.parseInt(f[1]),
					Integer.parseInt(f[2])
				};
				case "item", "page" ->
				{
					// The reference loads every item and page.
				}
				default -> _unread.add(f[0]);
			}
		}
	}

	private static boolean isStep(String verb)
	{
		return switch (verb)
		{
			case "action", "bypass", "link", "hit", "kill", "advance", "approach", "tick", "relog", "pickup", "mark", "restart", "enter", "create" -> true;
			default -> false;
		};
	}

	private void play() throws Exception
	{
		final GameClient client = new GameClient(null);
		client.setAccountName("scenario" + _accounts++);
		client.setSessionId(new SessionKey(1, 2, 3, 4));
		_client = client;

		if (!_steps.isEmpty() && _steps.get(0)[0].equals("create"))
			client.setState(GameClientState.AUTHED);
		else
			seed();

		record();
		for (String[] s : _steps)
		{
			switch (s[0])
			{
				case "action" -> action(npc(s[1]));
				case "bypass" -> bypass(roles(s[1]));
				case "link" -> link(s[1]);
				case "kill" ->
				{
					final int n = s.length > 2 ? Integer.parseInt(s[2].substring(1)) : 1;
					for (int i = 0; i < n; i++)
						kill(s[1]);
				}
				case "hit" -> hit(s[1], Integer.parseInt(s[2]));
				case "advance" -> advance(s[1]);
				case "approach" -> approach(s[1]);
				case "tick" ->
				{
					final int n = s.length > 1 ? Integer.parseInt(s[1].substring(1)) : 1;
					for (int i = 0; i < n; i++)
						aiTick();
				}
				case "pickup" -> pickup(Integer.parseInt(s[1]));
				case "mark" -> mark(Integer.parseInt(s[1]));
				case "relog" ->
				{
					restart();
					enter();
				}
				case "restart" -> restart();
				case "enter" -> enter();
				case "create" -> create();
				default -> throw new IllegalStateException(_name + ": unknown step " + s[0]);
			}
		}
	}

	/**
	 * The seeded character of a scenario that does not create its own: made at the character
	 * line's level and the class line's class, with the clan, quest and holding lines, in the
	 * world at the Go fixture's entry point with the NPC lines' NPCs around it. Its character,
	 * quest and item rows are kept as written, so a relog restores it.
	 */
	private void seed() throws Exception
	{
		final Player player = Player.create(IdFactory.getInstance().getNextId(), PlayerData.getInstance().getTemplate(ClassId.VALUES[_class]), _client.getAccountName(), _character, (byte) 0, (byte) 0, (byte) 0, Sex.MALE);
		player.getStatus().setLevel(_level);
		player.getStatus().setExp(PlayerLevelData.getInstance().getPlayerLevel(_level).requiredExpToLevelUp());
		player.getStatus().setMaxHpMp();
		World.getInstance().addObject(player);

		_client.setState(GameClientState.IN_GAME);
		_client.setPlayer(player);
		player.setClient(_client);
		player.spawnMe(X, Y, Z);
		_player = player;
		_objectId = player.getObjectId();

		if (_clan >= 0)
		{
			final Clan clan = ClanTable.getInstance().createClan(player, "Scenario" + (char) ('a' + _clans++));
			if (clan == null)
				throw new IllegalStateException(_name + ": the clan line's clan was not created");
			clan.changeLevel(_clan);
		}
		for (String[] q : _quests)
		{
			final Quest quest = ScriptData.getInstance().getQuest(q[1]);
			if (quest == null)
				throw new IllegalStateException(_name + ": no quest " + q[1]);
			final QuestState st = new QuestState(player, quest);
			for (int i = 2; i < q.length; i++)
			{
				final int eq = q[i].indexOf('=');
				st.put(q[i].substring(0, eq), q[i].substring(eq + 1));
				ConnectionPool.apply("INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?)", _objectId, q[1], q[i].substring(0, eq), q[i].substring(eq + 1));
			}
		}
		holdings(player);
		spawnNpcs();
	}

	/** The hold, equip, standing, pk, cabal and festival lines, as the Go runner seeds them. */
	private void holdings(Player player) throws Exception
	{
		for (int[] h : _holds)
		{
			if (ItemData.getInstance().getTemplate(h[0]).isStackable())
				player.getInventory().addItem(h[0], h[1]);
			else
				for (int i = 0; i < h[1]; i++)
					player.getInventory().addItem(h[0], 1);
		}
		for (int id : _equips)
		{
			final ItemInstance item = player.getInventory().addItem(id, 1);
			player.getInventory().equipItem(item);
			if (!item.isEquipped())
				throw new IllegalStateException(_name + ": equip " + id + " is not worn");
		}
		if (_standing != 0)
			player.setAllianceWithVarkaKetra(_standing);
		if (_pk != 0)
			player.setPkKills(_pk);
		if (_cabal != null)
			SevenSignsManager.getInstance().setPlayerInfo(player.getObjectId(), CabalType.valueOf(_cabal[0].toUpperCase()), SealType.valueOf(_cabal[1].toUpperCase()));
		if (_festival != null)
			setFestival(_festival);
	}

	/** The festival scores the Seven Signs keep: dawn's, then dusk's. */
	private static int[] festival() throws Exception
	{
		final SevenSignsManager ss = SevenSignsManager.getInstance();
		return new int[]
		{
			(int) field(SevenSignsManager.class, ss, "_dawnFestivalScore"),
			(int) field(SevenSignsManager.class, ss, "_duskFestivalScore")
		};
	}

	private static void setFestival(int[] scores) throws Exception
	{
		final SevenSignsManager ss = SevenSignsManager.getInstance();
		set(SevenSignsManager.class, ss, "_dawnFestivalScore", scores[0]);
		set(SevenSignsManager.class, ss, "_duskFestivalScore", scores[1]);
	}

	private static Object field(Class<?> owner, Object target, String field) throws Exception
	{
		final Field f = owner.getDeclaredField(field);
		f.setAccessible(true);
		return f.get(target);
	}

	/**
	 * Spawns the NPC lines' NPCs around the player, the first time it stands in the world, without
	 * recording what their spawning sent (the Go runner drops it).
	 */
	private void spawnNpcs() throws Exception
	{
		if (_spawned)
			return;
		_spawned = true;
		final boolean muted = _muted;
		_muted = true;
		try
		{
			final Set<String> fought = new HashSet<>();
			for (String[] s : _steps)
				if (s[0].equals("kill") || s[0].equals("hit"))
					fought.add(s[1]);
			for (NpcLine n : _npcs.values())
			{
				if (fought.contains(n.role()))
					fixture(NpcData.getInstance().getTemplate(n.id()), n.type());
				spawn(n);
			}
			ThreadPool.advance(0);
		}
		finally
		{
			_muted = muted;
		}
	}

	/**
	 * Binds every NPC template to the scenario's scripts only, from the bindings the script loader
	 * left, as the Go runner registers them: another script bound to the same NPC (another quest
	 * in its chooser, a monster's own behavior) is not the scenario's.
	 */
	@SuppressWarnings("unchecked")
	private void restrict(Map<NpcTemplate, Map<EventHandler, List<Quest>>> loaded) throws Exception
	{
		final Field events = NpcTemplate.class.getDeclaredField("_questEvents");
		events.setAccessible(true);
		for (NpcTemplate t : NpcData.getInstance().getTemplates())
		{
			final Map<EventHandler, List<Quest>> own = (Map<EventHandler, List<Quest>>) events.get(t);
			final Map<EventHandler, List<Quest>> kept = new EnumMap<>(EventHandler.class);
			for (Map.Entry<EventHandler, List<Quest>> e : loaded.getOrDefault(t, Map.of()).entrySet())
			{
				final List<Quest> quests = new ArrayList<>();
				for (Quest q : e.getValue())
					if (_scripts.contains(q.getClass().getName()))
						quests.add(q);
				if (!quests.isEmpty())
					kept.put(e.getKey(), quests);
			}
			_bindings.put(t, own);
			events.set(t, kept);
		}
	}

	/** Gives the NPC templates their own script bindings back. */
	private void restore() throws Exception
	{
		final Field events = NpcTemplate.class.getDeclaredField("_questEvents");
		events.setAccessible(true);
		for (Map.Entry<NpcTemplate, Map<EventHandler, List<Quest>>> e : _bindings.entrySet())
			events.set(e.getKey(), e.getValue());
		_bindings.clear();
	}

	/**
	 * Makes template the Go runner's fixture monster of the NPC line's type (the instance class a
	 * spawn builds), keeping its script bindings.
	 */
	private static void fixture(NpcTemplate template, String type) throws Exception
	{
		set(NpcTemplate.class, template, "_type", type);
		set(NpcTemplate.class, template, "_level", (byte) 1);
		set(NpcTemplate.class, template, "_exp", 0.);
		set(NpcTemplate.class, template, "_sp", 0.);
		set(NpcTemplate.class, template, "_aggroRange", 0);
		set(NpcTemplate.class, template, "_categories", Collections.emptyList());
		set(NpcTemplate.class, template, "_privateData", Collections.emptyList());
		set(NpcTemplate.class, template, "_passives", Collections.emptyList());
		set(NpcTemplate.class, template, "_skills", new EnumMap<>(NpcSkillType.class));
		set(CreatureTemplate.class, template, "_baseHpMax", 1000.);
	}

	private static void set(Class<?> owner, Object target, String field, Object value) throws Exception
	{
		final Field f = owner.getDeclaredField(field);
		f.setAccessible(true);
		f.set(target, value);
	}

	private Npc spawn(NpcLine n) throws Exception
	{
		final Spawn spawn = new Spawn(NpcData.getInstance().getTemplate(n.id()));
		spawn.setLoc(X + n.dx(), Y + n.dy(), Z + n.dz(), 0);
		final Npc npc = spawn.doSpawn(false);
		_live.put(n.role(), npc);
		_spawnedNpcs.add(npc);
		_lines.add("# npc " + n.role() + " " + npc.getNpcId() + " (object " + npc.getObjectId() + ")");
		return npc;
	}

	private Npc npc(String role)
	{
		final Npc npc = _live.get(role);
		if (npc == null)
			throw new IllegalStateException(_name + ": no NPC " + role);
		return npc;
	}

	/** Replaces each {role} of s with that role's object id. */
	private String roles(String s)
	{
		for (Map.Entry<String, Npc> e : _live.entrySet())
			s = s.replace("{" + e.getKey() + "}", String.valueOf(e.getValue().getObjectId()));
		return s;
	}

	private void kill(String role) throws Exception
	{
		Npc monster = _live.get(role);
		if (monster == null || monster.isDead())
			monster = spawn(_npcs.get(role));
		_lines.add("# kill " + monster.getNpcId() + " (object " + monster.getObjectId() + ")");
		monster.reduceCurrentHp(monster.getStatus().getMaxHp() + 1, _player, null);

		// The dying hook runs 3 s after the death.
		for (int i = 0; i < KILL_TICKS; i++)
			tick();
	}

	private void hit(String role, int damage)
	{
		final Npc monster = npc(role);
		_lines.add("# hit " + monster.getNpcId() + " (object " + monster.getObjectId() + ") " + damage);
		monster.reduceCurrentHp(damage, _player, null);
		tick();
	}

	private void advance(String duration)
	{
		final long ms = millis(duration);
		_lines.add("# advance " + ms);
		for (long passed = 0; passed < ms; passed += STEP)
		{
			final long d = Math.min(STEP, ms - passed);
			_lines.add("T " + d);
			ThreadPool.advance(d);
		}
	}

	/** Reads a Go duration of whole milliseconds: 500ms, 3s, 1m30s. */
	private static long millis(String d)
	{
		long total = 0;
		final java.util.regex.Matcher m = java.util.regex.Pattern.compile("(\\d+)(ms|s|m|h)").matcher(d);
		int end = 0;
		while (m.find())
		{
			if (m.start() != end)
				break;
			end = m.end();
			final long v = Long.parseLong(m.group(1));
			total += switch (m.group(2))
			{
				case "ms" -> v;
				case "s" -> v * 1000;
				case "m" -> v * 60_000;
				default -> v * 3_600_000;
			};
		}
		if (end != d.length())
			throw new IllegalArgumentException("duration " + d);
		return total;
	}

	private void action(Npc npc)
	{
		final ByteBuffer b = body(17);
		b.putInt(npc.getObjectId()).putInt(X).putInt(Y).putInt(Z).put((byte) 0);
		receive("Action " + npc.getNpcId() + " (object " + npc.getObjectId() + ")", new Action(), b);
	}

	private void bypass(String command) throws Exception
	{
		// The reference throttles bypasses to one per 100 ms.
		Thread.sleep(150);
		receive("RequestBypassToServer " + command, new RequestBypassToServer(), string(command));
	}

	private void link(String file)
	{
		receive("RequestLinkHtml " + file, new RequestLinkHtml(), string(file));
	}

	/** A RequestTutorialQuestionMark: the player clicks the tutorial question mark id. */
	private void mark(int id)
	{
		final ByteBuffer b = body(4);
		b.putInt(id);
		receive("RequestTutorialQuestionMark " + id, new RequestTutorialQuestionMark(), b);
	}

	/** A RequestRestart: back to the character select, the character stored at its logout. */
	private void restart()
	{
		receive("RequestRestart", new RequestRestart(), body(0));
		if (_client.getPlayer() != null)
			throw new IllegalStateException(_name + ": the restart was refused");
		_player = null;
	}

	/**
	 * The character select of slot 0 and the EnterWorld: the character restored from its kept
	 * rows enters the world. The character select's flood protector is reset first: the Go
	 * runner selects again at once.
	 */
	private void enter() throws Exception
	{
		Arrays.fill(_client.getFloodProtectors(), 0L);
		final ByteBuffer b = body(18);
		b.putInt(0).putShort((short) 0).putInt(0).putInt(0).putInt(0);
		receive("RequestGameStart 0", new RequestGameStart(), b);
		receive("EnterWorld", new EnterWorld(), body(0));
		_player = _client.getPlayer();
		if (_player == null)
			throw new IllegalStateException(_name + ": the character did not enter the world");
		_objectId = _player.getObjectId();
		spawnNpcs();
	}

	/**
	 * A RequestCharacterCreate of a human fighter named as the character line says, the Go
	 * runner's: race, sex, class and the six stats 0, hair style 1, hair color and face 0. The
	 * created character is then moved, in its kept row, to the Go fixture's entry point.
	 */
	private void create()
	{
		final ByteBuffer b = body(2 * _character.length() + 2 + 12 * 4);
		for (char c : _character.toCharArray())
			b.putChar(c);
		b.putChar('\0');
		for (int i = 0; i < 9; i++)
			b.putInt(0);
		b.putInt(1).putInt(0).putInt(0);
		receive("RequestCharacterCreate " + _character, new RequestCharacterCreate(), b);
		final int obj = PlayerInfoTable.getInstance().getPlayerObjectId(_character);
		if (obj <= 0)
			throw new IllegalStateException(_name + ": the character was not created");
		_objectId = obj;
		ConnectionPool.apply("UPDATE characters SET x=?,y=?,z=? WHERE obj_Id=?", X, Y, Z, obj);
	}

	/**
	 * An Action on the one ground item of the template id, from where the player stands, then
	 * time in ticks until the item left the ground.
	 */
	private void pickup(int id)
	{
		final List<ItemInstance> ground = new ArrayList<>();
		for (WorldObject o : World.getInstance().getObjects())
			if (o instanceof ItemInstance item && item.getItemId() == id && item.isVisible())
				ground.add(item);
		if (ground.size() != 1)
			throw new IllegalStateException(_name + ": pickup " + id + ": " + ground.size() + " ground items of that template");
		final ItemInstance item = ground.get(0);
		final ByteBuffer b = body(17);
		b.putInt(item.getObjectId()).putInt(_player.getX()).putInt(_player.getY()).putInt(_player.getZ()).put((byte) 0);
		receive("Action item " + id + " (object " + item.getObjectId() + ")", new Action(), b);
		for (int i = 0; item.isVisible(); i++)
		{
			if (i == PICKUP_TICKS)
				throw new IllegalStateException(_name + ": pickup " + id + ": the item is still on the ground");
			tick();
		}
	}

	/**
	 * The player steps to 100 in front of the NPC, on the -x side, and the NPC AI task runs once.
	 */
	private void approach(String role)
	{
		final Npc npc = npc(role);
		_lines.add("# approach " + npc.getNpcId() + " (object " + npc.getObjectId() + ")");
		_player.setXYZ(npc.getX() - 100, npc.getY(), npc.getZ());
		_player.revalidateZone(true);
		AiTaskManager.getInstance().run();
		ThreadPool.advance(0);
	}

	/** One run of the NPC AI task, then what it queued now. */
	private void aiTick()
	{
		_lines.add("# tick");
		AiTaskManager.getInstance().run();
		ThreadPool.advance(0);
	}

	private static ByteBuffer string(String s)
	{
		final ByteBuffer b = body(2 * s.length() + 2);
		for (char c : s.toCharArray())
			b.putChar(c);
		b.putChar('\0');
		return b;
	}

	private static ByteBuffer body(int size)
	{
		return ByteBuffer.allocate(size).order(ByteOrder.LITTLE_ENDIAN);
	}

	private void receive(String label, L2GameClientPacket packet, ByteBuffer b)
	{
		_lines.add("C " + label);
		ProbeWire.receive(_client, packet, b.array());
		tick();
	}

	private void tick()
	{
		_lines.add("T " + STEP);
		ThreadPool.advance(STEP);
	}

	private void record()
	{
		ProbeWire.setSink((client, line) ->
		{
			if (client == _client && !_muted)
				_lines.add("S " + line);
		});
		ConnectionPool.setSink(sql ->
		{
			if (!_muted)
				_lines.add("Q " + sql);
		});
	}

	private static void stop()
	{
		ProbeWire.setSink(null);
		ConnectionPool.setSink(null);
	}

	/**
	 * Removes the scenario's player, the NPCs spawned for its NPC lines and every NPC and ground item
	 * near it, so the next scenario starts alone.
	 */
	private void clear()
	{
		final List<Npc> near = new ArrayList<>(_spawnedNpcs);
		final List<ItemInstance> ground = new ArrayList<>();
		for (WorldObject o : World.getInstance().getObjects())
		{
			if (Math.abs(o.getX() - X) >= 3000 || Math.abs(o.getY() - Y) >= 3000)
				continue;
			if (o instanceof Npc npc && !near.contains(npc))
				near.add(npc);
			else if (o instanceof ItemInstance item && item.isVisible())
				ground.add(item);
		}
		for (Npc npc : near)
			npc.deleteMe();
		for (ItemInstance item : ground)
			item.decayMe();
		if (_player != null)
			_player.deleteMe();
		PlayerInfoTable.getInstance().removePlayer(_objectId);
		ThreadPool.cancelAll();
	}

	private void write(File f) throws Exception
	{
		try (PrintWriter w = new PrintWriter(f, StandardCharsets.UTF_8))
		{
			w.println("# Quest scenario trace of the reference server: generated by run.sh, do not edit.");
			w.println("# " + _name + ": every step of the scenario.");
			w.printf("# player object %d; format: README.md in this directory.%n", _objectId);
			for (String l : _lines)
				w.println(l);
		}
	}
}
