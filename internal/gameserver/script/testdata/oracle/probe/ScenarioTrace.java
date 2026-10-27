import java.io.File;
import java.io.PrintWriter;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.nio.file.FileSystems;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.PathMatcher;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.BitSet;
import java.util.Comparator;
import java.util.Collections;
import java.util.EnumMap;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

import net.sf.l2j.commons.mmocore.ProbeWire;
import net.sf.l2j.commons.probe.ProbeClock;
import net.sf.l2j.commons.pool.ConnectionPool;
import net.sf.l2j.commons.pool.ThreadPool;
import net.sf.l2j.commons.random.Rnd;

import net.sf.l2j.Config;
import net.sf.l2j.gameserver.data.manager.CastleManager;
import net.sf.l2j.gameserver.data.manager.CoupleManager;
import net.sf.l2j.gameserver.data.manager.DerbyTrackManager;
import net.sf.l2j.gameserver.data.manager.FishingChampionshipManager;
import net.sf.l2j.gameserver.data.manager.LotteryManager;
import net.sf.l2j.gameserver.data.manager.RaidPointManager;
import net.sf.l2j.gameserver.data.manager.RelationManager;
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
import net.sf.l2j.gameserver.enums.SiegeSide;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.NpcSkillType;
import net.sf.l2j.gameserver.enums.actors.Sex;
import net.sf.l2j.gameserver.idfactory.IdFactory;
import net.sf.l2j.gameserver.model.World;
import net.sf.l2j.gameserver.model.WorldObject;
import net.sf.l2j.gameserver.model.WorldRegion;
import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.container.player.SubClass;
import net.sf.l2j.gameserver.model.actor.instance.Folk;
import net.sf.l2j.gameserver.model.actor.instance.Monster;
import net.sf.l2j.gameserver.model.actor.template.CreatureTemplate;
import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.pledge.Clan;
import net.sf.l2j.gameserver.model.residence.castle.Castle;
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
import net.sf.l2j.gameserver.taskmanager.AttackStanceTaskManager;
import net.sf.l2j.gameserver.taskmanager.DecayTaskManager;
import net.sf.l2j.gameserver.taskmanager.GameTimeTaskManager;
import net.sf.l2j.gameserver.taskmanager.ItemInstanceTaskManager;
import net.sf.l2j.gameserver.taskmanager.ItemsOnGroundTaskManager;
import net.sf.l2j.gameserver.taskmanager.PvpFlagTaskManager;
import net.sf.l2j.gameserver.taskmanager.ShadowItemTaskManager;
import net.sf.l2j.gameserver.taskmanager.WalkerTaskManager;
import net.sf.l2j.gameserver.taskmanager.WaterTaskManager;

/**
 * Quest scenario traces: each scenario file of the Go quest suite whose {@code trace} line names
 * a file under {@code oracle/scenarios/} is played here, from the scenario itself, and its trace
 * is written to that file. The header sets the player up as the Go runner does (name, level,
 * class, the subclass it plays, a led clan of the given level and its side of a castle's siege, quest rows, the script
 * draws of the random line) and
 * spawns each NPC line's NPC at its offset from the player, who stands where the Go fixture
 * character enters the world. The hold, equip, standing, pk, cabal, festival, sealowner, memo and spoil lines are seeded
 * as the Go runner seeds them. The NPCs keep only the bindings of the scenario's script and stub
 * lines, as the Go runner registers no other script. Every NPC line's NPC has the casting stats of the Go runner's
 * fixture NPC (WIT and DEX 0, its base attack speed), and a monster the scenario kills or hits is made the Go runner's
 * fixture monster: level 1, 1000 HP, no experience, SP, drops, skills, minions or aggression. The templates get their
 * own values back once the scenario is over.
 * <p>
 * The character, item, quest, memo and subclass rows the scenario writes are kept (see the probe's
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
	/** The object id of the other player a spoil line's other names: nobody in the world. */
	private static final int OTHER_SPOILER = 0x7fff0001;

	/** The kill wait of the Go runner: the dying hooks run 3 s after the death. */
	private static final int KILL_TICKS = 7;

	/** The account of every scenario's character: no row of an earlier scenario is kept. */
	private static final String ACCOUNT = "scenario";

	/** The name of the clan a clan line creates: an earlier scenario's clan is gone. */
	private static final String CLAN = "Scenario";

	/** The tables whose rows a scenario keeps. */
	private static final String[] KEPT =
	{
		"characters",
		"items",
		"character_quests",
		"character_memo",
		"character_subclasses"
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
	/** The template fields the scenario changed, each with the value it had, in the order they changed. */
	private final List<Altered> _altered = new ArrayList<>();
	private String _trace;
	private String _character;
	private int _level;
	private int _class;
	/** The subclass line's class and level; 0 for none. */
	private int _subclass;
	private int _subclassLevel;
	private int _clan = -1;
	/** The siege line's castle id and side; null for none. */
	private String[] _siege;
	private int[] _draws;
	private boolean _always;
	private final List<int[]> _holds = new ArrayList<>();
	private final List<Integer> _equips = new ArrayList<>();
	private int _standing;
	private int _pk;
	private String[] _cabal;
	private int[] _festival;
	/** The sealowner lines: seal name, cabal name. */
	private final List<String[]> _sealOwners = new ArrayList<>();
	/** The memo lines: variable, value. */
	private final List<String[]> _memos = new ArrayList<>();
	/** The spoil lines' roles, and whether the spoiler is another player. */
	private final Map<String, Boolean> _spoils = new HashMap<>();
	/** The live lines of each step, by the step's index: NPC id. */
	private final Map<Integer, List<Integer>> _lives = new HashMap<>();

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

	/** A field of target the scenario changed, and the value it had before. */
	private record Altered(Field field, Object target, Object value)
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

	/**
	 * Plays every scenario of {@code scenarios}, or, when {@code only} is not blank, the ones whose
	 * file name matches one of its comma-separated globs. A trace depends on its scenario alone,
	 * so playing a subset writes the same traces as a full run.
	 */
	static void run(File scenarios, String only, File out, Map<NpcTemplate, Map<EventHandler, List<Quest>>> loaded) throws Exception
	{
		final List<PathMatcher> globs = new ArrayList<>();
		for (String g : only.split(","))
			if (!g.isBlank())
				globs.add(FileSystems.getDefault().getPathMatcher("glob:" + g.strip()));
		final File[] files = scenarios.listFiles((d, n) -> n.endsWith(".scenario")
			&& (globs.isEmpty() || globs.stream().anyMatch(m -> m.matches(Path.of(n)))));
		if (files == null)
			throw new IllegalStateException("no scenario directory " + scenarios);
		if (files.length == 0 && !globs.isEmpty())
			throw new IllegalStateException("no scenario matches PROBE_ONLY=" + only);
		Arrays.sort(files);
		// What loads from the database on first use is loaded before the first scenario, else its
		// query (and the runners' ids) would be in whichever scenario came first: the events the
		// reference loads at boot after the scripts, and the friend lists, which the reference loads
		// when a first character enters or leaves the world.
		// The track has run its first countdown, as it has a second after the server starts: its
		// race packet and runners exist, which a race manager appearing or leaving sends and reads.
		final Method countdown = DerbyTrackManager.class.getDeclaredMethod("countdown");
		countdown.setAccessible(true);
		final DerbyTrackManager derby = DerbyTrackManager.getInstance();
		countdown.invoke(derby);
		// The countdown shuffles the runners with an unseeded Random; the order the probe traces must not
		// depend on it. The runners go back in object id order (the chosen eight are the first eight, a
		// view of the same list the race packet holds).
		final Field runnersField = DerbyTrackManager.class.getDeclaredField("_runners");
		runnersField.setAccessible(true);
		@SuppressWarnings("unchecked")
		final List<Npc> runners = (List<Npc>) runnersField.get(derby);
		runners.sort(Comparator.comparingInt(Npc::getObjectId));
		final Field chosenField = DerbyTrackManager.class.getDeclaredField("_chosenRunners");
		chosenField.setAccessible(true);
		chosenField.set(derby, runners.subList(0, 8));
		LotteryManager.getInstance();
		CoupleManager.getInstance();
		if (Config.ALLOW_FISH_CHAMPIONSHIP)
			FishingChampionshipManager.getInstance();
		RelationManager.getInstance();
		final Ids ids = Ids.take();
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
			// Every scenario draws its object ids from where the first one did, so its trace does
			// not depend on which other scenarios exist.
			ids.restore();
			Rnd.reseed(1);
			if (t._draws != null)
				Rnd.scriptCallers(t._draws, t._always, d -> t._lines.add("# draw " + d));
			t.restrict(loaded);
			ConnectionPool.keep(KEPT);
			ProbeClock.virtual(WALL);
			set(GameTimeTaskManager.class, GameTimeTaskManager.getInstance(), "_time", GAME_TIME);
			set(GameTimeTaskManager.class, GameTimeTaskManager.getInstance(), "_isNight", false);
			final int[] festival = festival();
			final Map<SealType, CabalType> sealOwners = new EnumMap<>(SevenSignsManager.getInstance().getSealOwners());
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
				SevenSignsManager.getInstance().getSealOwners().clear();
				SevenSignsManager.getInstance().getSealOwners().putAll(sealOwners);
			}
			if (!failures.isEmpty())
				throw new IllegalStateException(t._name + ": statements the probe store cannot run: " + failures);
			final File target = new File(out, "scenarios/" + t._trace.substring(TRACED.length()));
			target.getParentFile().mkdirs();
			t.write(target);
			t.clear();
			ids.forget(t._name);
		}
	}

	/**
	 * The object id factory as the first scenario found it. Each scenario starts from it, so the
	 * next one reuses the ids this one drew: once a scenario is over, {@link #forget} drops every
	 * object and registry entry that still holds one of them.
	 */
	private record Ids(BitSet used, int free, int next)
	{
		static Ids take() throws Exception
		{
			final IdFactory f = IdFactory.getInstance();
			return new Ids((BitSet) ((BitSet) field(IdFactory.class, f, "_freeIds")).clone(), ((AtomicInteger) field(IdFactory.class, f, "_freeIdCount")).get(), ((AtomicInteger) field(IdFactory.class, f, "_nextFreeId")).get());
		}

		void restore() throws Exception
		{
			final IdFactory f = IdFactory.getInstance();
			set(IdFactory.class, f, "_freeIds", used.clone());
			set(IdFactory.class, f, "_freeIdCount", new AtomicInteger(free));
			set(IdFactory.class, f, "_nextFreeId", new AtomicInteger(next));
			// Every region's object map starts the scenario fresh, holding what stood there before
			// the first scenario, in object id order. A map keeps the capacity an earlier scenario grew
			// it to, and its iteration order (the order a character entering the world is shown what
			// stands around it) follows that capacity.
			for (WorldRegion[] column : World.getInstance().getWorldRegions())
			{
				for (WorldRegion region : column)
				{
					final Map<Integer, WorldObject> objects = new ConcurrentHashMap<>();
					region.getObjects().stream().sorted(Comparator.comparingInt(WorldObject::getObjectId)).forEach(o -> objects.put(o.getObjectId(), o));
					set(WorldRegion.class, region, "_objects", objects);
				}
			}
		}

		/** The task managers' registries of creatures and items: maps keyed by them, or sets. */
		private static List<Object> taskRegistries() throws Exception
		{
			return List.of(
				field(AttackStanceTaskManager.class, AttackStanceTaskManager.getInstance(), "_creatures"),
				field(DecayTaskManager.class, DecayTaskManager.getInstance(), "_creatures"),
				field(AiTaskManager.class, AiTaskManager.getInstance(), "_npcs"),
				field(WalkerTaskManager.class, WalkerTaskManager.getInstance(), "_walkers"),
				field(PvpFlagTaskManager.class, PvpFlagTaskManager.getInstance(), "_players"),
				field(WaterTaskManager.class, WaterTaskManager.getInstance(), "_players"),
				field(GameTimeTaskManager.class, GameTimeTaskManager.getInstance(), "_players"),
				field(ItemsOnGroundTaskManager.class, ItemsOnGroundTaskManager.getInstance(), "_items"),
				field(ItemInstanceTaskManager.class, ItemInstanceTaskManager.getInstance(), "_items"),
				field(ShadowItemTaskManager.class, ShadowItemTaskManager.getInstance(), "_shadowItems"));
		}

		/** Whether id is one a scenario drew. */
		boolean drawn(int id)
		{
			return id >= IdFactory.FIRST_OID && !used.get(id - IdFactory.FIRST_OID);
		}

		/**
		 * Removes from the world every object of an id the scenario drew, wherever it stands (an NPC
		 * a script spawned away from the player), and drops the registry entries keyed by one (the
		 * clan, its siege registrations and castle, the character's Seven Signs sign-up and raid
		 * points, the task managers' creatures and items). Fails on an object still in the
		 * world: the next scenario's object of that id would not enter it.
		 */
		@SuppressWarnings("unchecked")
		void forget(String scenario) throws Exception
		{
			for (WorldObject o : new ArrayList<>(World.getInstance().getObjects()))
			{
				if (!drawn(o.getObjectId()))
					continue;
				if (o instanceof Npc npc)
					npc.deleteMe();
				else if (o instanceof Player player)
					player.deleteMe();
				else
					o.decayMe();
			}
			ThreadPool.cancelAll();
			((Map<Integer, ?>) field(ClanTable.class, ClanTable.getInstance(), "_clans")).keySet().removeIf(this::drawn);
			for (Castle castle : CastleManager.getInstance().getCastles())
			{
				castle.getSiege().getRegisteredClans().keySet().removeIf(c -> drawn(c.getClanId()));
				if (drawn(castle.getOwnerId()))
					castle.setOwnerId(0);
			}
			((Map<Integer, ?>) field(SevenSignsManager.class, SevenSignsManager.getInstance(), "_playersData")).keySet().removeIf(this::drawn);
			((Map<Integer, ?>) field(RaidPointManager.class, RaidPointManager.getInstance(), "_entries")).keySet().removeIf(this::drawn);
			// A world object equals any other of its id, so a task manager's entry the scenario left
			// would stand for the next scenario's object of that id: the attack stance a cast leaves on
			// its caster and its target shows the next NPC of that id in combat.
			for (Object registry : taskRegistries())
				(registry instanceof Map<?, ?> m ? m.keySet() : (Set<?>) registry).removeIf(o -> o instanceof WorldObject w && drawn(w.getObjectId()));
			final List<String> kept = new ArrayList<>();
			for (WorldObject o : World.getInstance().getObjects())
				if (drawn(o.getObjectId()))
					kept.add(o + " (object " + o.getObjectId() + ")");
			if (!kept.isEmpty())
				throw new IllegalStateException(scenario + ": world objects outlive the scenario: " + kept);
		}
	}

	/** Reads the header lines the probe needs and the step verb lines. */
	private void parse(List<String> lines)
	{
		boolean steps = false;
		for (String raw : lines)
		{
			final String line = raw.strip();
			if (line.startsWith("live ") && (raw.startsWith(" ") || raw.startsWith("\t")) && !_steps.isEmpty())
			{
				// A step's expectation: how many NPCs of that template stand once it is over.
				_lives.computeIfAbsent(_steps.size() - 1, k -> new ArrayList<>()).add(Integer.parseInt(line.split("\\s+")[1]));
				continue;
			}
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
				case "subclass" ->
				{
					_subclass = Integer.parseInt(f[1]);
					_subclassLevel = Integer.parseInt(f[2]);
				}
				case "clan" -> _clan = Integer.parseInt(f[1]);
				case "siege" -> _siege = new String[]
				{
					f[1],
					f[2]
				};
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
				case "sealowner" -> _sealOwners.add(new String[]
				{
					f[1],
					f[2]
				});
				case "memo" -> _memos.add(new String[]
				{
					f[1],
					f[2]
				});
				case "spoil" -> _spoils.put(f[1], f.length > 2);
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
		client.setAccountName(ACCOUNT);
		client.setSessionId(new SessionKey(1, 2, 3, 4));
		_client = client;

		if (!_steps.isEmpty() && _steps.get(0)[0].equals("create"))
			client.setState(GameClientState.AUTHED);
		else
			seed();

		record();
		for (int index = 0; index < _steps.size(); index++)
		{
			final String[] s = _steps.get(index);
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
			for (int id : _lives.getOrDefault(index, List.of()))
				_lines.add("# live " + id + " " + live(id));
		}
	}

	/** How many NPCs of the template stand alive in the world. */
	private static long live(int npcId)
	{
		return World.getInstance().getObjects().stream().filter(o -> o instanceof Npc n && n.getNpcId() == npcId && !n.isDead()).count();
	}

	/**
	 * The seeded character of a scenario that does not create its own: made at the character
	 * line's level and the class line's class, playing the subclass line's subclass, with the clan,
	 * quest and holding lines, in the world at the Go fixture's entry point with the NPC lines' NPCs
	 * around it. Its character, subclass, quest and item rows are kept as written, so a relog
	 * restores it.
	 */
	private void seed() throws Exception
	{
		final Player player = Player.create(IdFactory.getInstance().getNextId(), PlayerData.getInstance().getTemplate(ClassId.VALUES[_class]), _client.getAccountName(), _character, (byte) 0, (byte) 0, (byte) 0, Sex.MALE);
		player.getStatus().setLevel(_level);
		player.getStatus().setExp(PlayerLevelData.getInstance().getPlayerLevel(_level).requiredExpToLevelUp());
		if (_subclass != 0)
			subclass(player);
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
			final Clan clan = ClanTable.getInstance().createClan(player, CLAN);
			if (clan == null)
				throw new IllegalStateException(_name + ": the clan line's clan was not created");
			clan.changeLevel(_clan);
			if (_siege != null)
				siege(clan);
		}
		else if (_siege != null)
			throw new IllegalStateException(_name + ": a siege line needs a clan line");
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
		for (String[] m : _memos)
			player.getMemos().set(m[0], m[1]);
		holdings(player);
		spawnNpcs();
	}

	/**
	 * The subclass line's subclass, in slot 1 at its level, and played: the state the reference's
	 * restore leaves of a character that logged out on it (its row kept, so a relog restores it),
	 * with the base class's level and experience left as the character line set them.
	 */
	private void subclass(Player player) throws Exception
	{
		final long exp = PlayerLevelData.getInstance().getPlayerLevel(_subclassLevel).requiredExpToLevelUp();
		final SubClass sub = new SubClass(_subclass, 1, exp, 0, (byte) _subclassLevel);
		player.getSubClasses().put(sub.getClassIndex(), sub);
		ConnectionPool.apply("INSERT INTO character_subclasses (char_obj_id,class_id,exp,sp,level,class_index) VALUES (?,?,?,?,?,?)", player.getObjectId(), sub.getClassId(), sub.getExp(), sub.getSp(), sub.getLevel(), sub.getClassIndex());
		set(Player.class, player, "_classIndex", sub.getClassIndex());
		set(Player.class, player, "_activeClass", sub.getClassId());
		player.setTemplate(sub.getClassDefinition());
		if (!player.isSubClassActive() || player.getStatus().getLevel() != _subclassLevel)
			throw new IllegalStateException(_name + ": the subclass line's subclass is not played");
	}

	/**
	 * The siege line: the clan registered on that side of the castle's siege, as the reference's
	 * siege restore leaves a stored registration; an owner clan is the castle's owner, on its
	 * siege's owner side, as the castle and clan restores leave a stored ownership.
	 */
	private void siege(Clan clan)
	{
		final Castle castle = CastleManager.getInstance().getCastleById(Integer.parseInt(_siege[0]));
		if (castle == null)
			throw new IllegalStateException(_name + ": no castle " + _siege[0]);
		final SiegeSide side = SiegeSide.valueOf(_siege[1].toUpperCase());
		if (side == SiegeSide.OWNER)
		{
			castle.setOwnerId(clan.getClanId());
			clan.setCastle(castle.getId());
			castle.getSiege().getRegisteredClans().put(clan, SiegeSide.OWNER);
		}
		else
			castle.getSiege().registerClan(clan, side);
		if (castle.getSiege().getSide(clan) != side)
			throw new IllegalStateException(_name + ": the siege line's clan is not on the " + side + " side");
	}
	
	/** The hold, equip, standing, pk, cabal, festival, sealowner, memo and spoil lines, as the Go runner seeds them. */
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
		for (String[] o : _sealOwners)
			SevenSignsManager.getInstance().getSealOwners().put(SealType.valueOf(o[0].toUpperCase()), CabalType.valueOf(o[1].toUpperCase()));
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
				final NpcTemplate template = NpcData.getInstance().getTemplate(n.id());
				casting(template, n.type());
				if (fought.contains(n.role()))
					fixture(template, n.type());
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

	/** Gives the NPC templates their own script bindings and the values the scenario changed back. */
	private void restore() throws Exception
	{
		final Field events = NpcTemplate.class.getDeclaredField("_questEvents");
		events.setAccessible(true);
		for (Map.Entry<NpcTemplate, Map<EventHandler, List<Quest>>> e : _bindings.entrySet())
			events.set(e.getKey(), e.getValue());
		_bindings.clear();
		for (int i = _altered.size() - 1; i >= 0; i--)
		{
			final Altered a = _altered.get(i);
			a.field().set(a.target(), a.value());
		}
		_altered.clear();
	}

	/** Sets a field of target until the scenario is over (see {@link #restore}). */
	private void alter(Class<?> owner, Object target, String field, Object value) throws Exception
	{
		final Field f = owner.getDeclaredField(field);
		f.setAccessible(true);
		_altered.add(new Altered(f, target, f.get(target)));
		f.set(target, value);
	}

	/**
	 * Gives template the casting stats of the Go runner's fixture NPC of the NPC line's type, which
	 * every NPC line's NPC has there: WIT and DEX 0, and a base attack speed of 253 for a civilian
	 * kind, 300 for any other. A cast's hit time and reuse delay follow them.
	 */
	private void casting(NpcTemplate template, String type) throws Exception
	{
		alter(CreatureTemplate.class, template, "_baseWIT", 0);
		alter(CreatureTemplate.class, template, "_baseDEX", 0);
		alter(CreatureTemplate.class, template, "_basePAtkSpd", civilian(type) ? 253. : 300.);
	}

	/**
	 * Whether the Go runner's fixture NPC of type is its civilian one: the kinds whose instance class
	 * is a Folk, and the siege towers and flag, less the Christmas tree.
	 */
	private static boolean civilian(String type) throws Exception
	{
		return switch (type)
		{
			case "FlameTower", "LifeTower", "SiegeFlag" -> true;
			case "ChristmasTree" -> false;
			default -> Folk.class.isAssignableFrom(Class.forName("net.sf.l2j.gameserver.model.actor.instance." + type));
		};
	}

	/**
	 * Makes template the Go runner's fixture monster of the NPC line's type (the instance class a
	 * spawn builds), keeping its script bindings.
	 */
	private void fixture(NpcTemplate template, String type) throws Exception
	{
		alter(NpcTemplate.class, template, "_type", type);
		alter(NpcTemplate.class, template, "_level", (byte) 1);
		alter(NpcTemplate.class, template, "_exp", 0.);
		alter(NpcTemplate.class, template, "_sp", 0.);
		alter(NpcTemplate.class, template, "_aggroRange", 0);
		alter(NpcTemplate.class, template, "_categories", Collections.emptyList());
		alter(NpcTemplate.class, template, "_privateData", Collections.emptyList());
		alter(NpcTemplate.class, template, "_passives", Collections.emptyList());
		alter(NpcTemplate.class, template, "_skills", new EnumMap<>(NpcSkillType.class));
		alter(CreatureTemplate.class, template, "_baseHpMax", 1000.);
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
		// A spawn clears the spoil state, so each fresh monster of the role is marked after it.
		if (_spoils.containsKey(n.role()))
			((Monster) npc).getSpoilState().setSpoilerId(_spoils.get(n.role()) ? OTHER_SPOILER : _player.getObjectId());
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
