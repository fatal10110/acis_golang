import java.io.File;
import java.io.PrintWriter;
import java.lang.reflect.Constructor;
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
import net.sf.l2j.commons.probe.ProbeHeard;
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
import net.sf.l2j.gameserver.data.manager.SpawnManager;
import net.sf.l2j.gameserver.data.SkillTable;
import net.sf.l2j.gameserver.skills.L2Skill;
import net.sf.l2j.gameserver.data.manager.ZoneManager;
import net.sf.l2j.gameserver.model.zone.form.ZoneCuboid;
import net.sf.l2j.gameserver.model.zone.ZoneForm;
import net.sf.l2j.gameserver.model.zone.type.subtype.ZoneType;
import net.sf.l2j.gameserver.data.manager.ClanHallManager;
import net.sf.l2j.gameserver.data.sql.ClanTable;
import net.sf.l2j.gameserver.data.sql.PlayerInfoTable;
import net.sf.l2j.gameserver.data.xml.DoorData;
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
import net.sf.l2j.gameserver.model.actor.instance.Door;
import net.sf.l2j.gameserver.model.actor.instance.Folk;
import net.sf.l2j.gameserver.model.actor.instance.Monster;
import net.sf.l2j.gameserver.model.actor.template.CreatureTemplate;
import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.memo.NpcMemo;
import net.sf.l2j.gameserver.enums.SiegeStatus;
import net.sf.l2j.gameserver.model.pledge.Clan;
import net.sf.l2j.gameserver.model.residence.clanhall.ClanHall;
import net.sf.l2j.gameserver.model.residence.clanhall.SiegableHall;
import net.sf.l2j.gameserver.model.residence.castle.Castle;
import net.sf.l2j.gameserver.model.spawn.ASpawn;
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
import net.sf.l2j.gameserver.network.serverpackets.CharSelectInfo;
import net.sf.l2j.gameserver.network.serverpackets.MonRaceInfo;
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
 * character enters the world. The hold, equip, standing, pk, karma, warehouse, hero, cabal, festival, sealowner, memo and spoil lines are seeded
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

	/** The account of a mate: no row of an earlier scenario is kept either. */
	private static final String MATE_ACCOUNT = "mate";

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
	/** The clan line's hall: the clan hall the clan owns, 0 for none. */
	private int _clanHall;
	/** The hall lines: id, status, and the stored next siege date in milliseconds (0 for none). */
	private final List<long[]> _halls = new ArrayList<>();
	private final List<String> _hallStatuses = new ArrayList<>();
	/** The mate line of the clan's leader, when the character is not: name, level. */
	private String[] _leader;
	/** The mate line's relation to the character, sponsor or apprentice; null for neither. */
	private String _mateRelation;
	/** Whether the mate line says online: the mate is in the world, in a client of its own, from the start. */
	private boolean _mateOnline;
	/** The mate line's offset from where the character first enters the world. */
	private final int[] _mateAt = new int[3];
	/** The mate line's hold lines: item id, count. */
	private final List<int[]> _mateHolds = new ArrayList<>();
	private int[] _draws;
	/** The time of day the clock line starts the scenario at, in milliseconds since midnight. */
	private long _clock;
	private boolean _always;
	private final List<int[]> _holds = new ArrayList<>();
	private final List<Integer> _equips = new ArrayList<>();
	private int _standing;
	private int _pk;
	private int _karma;
	private boolean _hero;
	/** The warehouse lines: item id, count. */
	private final List<int[]> _warehouse = new ArrayList<>();
	private boolean _noble;
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
	private final Map<Integer, List<Integer>> _spawns = new HashMap<>();
	/** The door lines: role, datapack door id. */
	private final Map<String, Integer> _doors = new LinkedHashMap<>();
	/** The zone lines: zone id, then the box's offsets dx1, dy1, dx2, dy2, dz1, dz2 from where the character enters the world. */
	private final List<int[]> _zones = new ArrayList<>();
	/** The shape each zoned id had before the scenario moved it. */
	private final Map<Integer, ZoneForm> _shapes = new HashMap<>();
	/** The open, dungeon, standing and stance expectations of each step, by the step's index, as written. */
	private final Map<Integer, List<String[]>> _checks = new HashMap<>();
	/** Whether a step sets or ends the character's attack stance, so the stance task runs on the scenario's time. */
	private boolean _stances;

	private GameClient _client;
	/** The client the step being played sends its packets from: the player's, or the mate's of an as step. */
	private GameClient _actor;
	/** The trace tag of the client that sends, "@" and the mate's name for the mate's, empty for the player's. */
	private String _tag = "";
	private Player _player;
	/** The mate: the clan's leader, a character of its own that has a client once it is in the world. */
	private Player _mate;
	private GameClient _mateClient;
	/** While true, the mate's own packets are not recorded: its enter burst, which the Go runner drops. */
	private boolean _entering;
	/** The mate's object id, kept across its entering the world. */
	private int _mateObjectId;
	/** The player's object id, kept across a relog. */
	private int _objectId;
	/** Whether the NPC lines' NPCs stand: once the character first entered the world. */
	private boolean _spawned;
	/** While true, packets and statements are not recorded: the probe's own setup. */
	private boolean _muted;

	/** An npc line: params holds the three spawn parameters the line gives (a number, an NPC role or character), null for none; ai the AI parameters of its ai option, which replace the template's as the Go runner's fixture template has only them, null for none. */
	private record NpcLine(String role, int id, String type, int dx, int dy, int dz, String[] params, Map<String, String> ai, int level, String[][] skills)
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
	 * Plays every scenario of the {@code scenarios} directories, or, when {@code only} is not blank, the ones whose
	 * file name matches one of its comma-separated globs. A trace depends on its scenario alone,
	 * so playing a subset writes the same traces as a full run.
	 */
	static void run(File[] scenarios, String only, File out, Map<NpcTemplate, Map<EventHandler, List<Quest>>> loaded) throws Exception
	{
		final List<PathMatcher> globs = new ArrayList<>();
		for (String g : only.split(","))
			if (!g.isBlank())
				globs.add(FileSystems.getDefault().getPathMatcher("glob:" + g.strip()));
		final List<File> listed = new ArrayList<>();
		for (File dir : scenarios)
		{
			final File[] in = dir.listFiles((d, n) -> n.endsWith(".scenario")
				&& (globs.isEmpty() || globs.stream().anyMatch(m -> m.matches(Path.of(n)))));
			if (in == null)
				throw new IllegalStateException("no scenario directory " + dir);
			listed.addAll(Arrays.asList(in));
		}
		final File[] files = listed.toArray(new File[0]);
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
		// view of the new list, as is the race packet, which is built again: sorting invalidates the
		// view the countdown's packet holds).
		final Field runnersField = DerbyTrackManager.class.getDeclaredField("_runners");
		runnersField.setAccessible(true);
		@SuppressWarnings("unchecked")
		final List<Npc> runners = (List<Npc>) runnersField.get(derby);
		runners.sort(Comparator.comparingInt(Npc::getObjectId));
		final Field chosenField = DerbyTrackManager.class.getDeclaredField("_chosenRunners");
		chosenField.setAccessible(true);
		chosenField.set(derby, runners.subList(0, 8));
		final Field codesField = DerbyTrackManager.class.getDeclaredField("CODES");
		codesField.setAccessible(true);
		final int[][] codes = (int[][]) codesField.get(null);
		final Field packetField = DerbyTrackManager.class.getDeclaredField("_packet");
		packetField.setAccessible(true);
		packetField.set(derby, new MonRaceInfo(codes[0][0], codes[0][1], derby.getRunners(), derby.getSpeeds()));
		LotteryManager.getInstance();
		CoupleManager.getInstance();
		if (Config.ALLOW_FISH_CHAMPIONSHIP)
			FishingChampionshipManager.getInstance();
		RelationManager.getInstance();
		final Ids ids = Ids.take();
		for (File f : files)
		{
			final List<String> text = Files.readAllLines(f.toPath(), StandardCharsets.UTF_8);
			// A scenario without a trace line is not played, so its other lines are not the probe's to read.
			if (text.stream().noneMatch(l -> l.startsWith("trace ")))
				continue;
			final ScenarioTrace t = new ScenarioTrace(f.getName());
			t.parse(text);
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
				Rnd.scriptCallers(t._draws, t._always, d ->
			{
				// The draws of the probe's own setup (the NPC lines' spawns) are not a step's.
				if (!t._muted)
					t._lines.add("# draw " + d);
			});
			// The script events an NPC hears, from a script's send or broadcast, are the step's lines.
			ProbeHeard.listen((npc, id, a1, a2) ->
			{
				if (!t._muted)
					t._lines.add("# heard " + npc.getNpcId() + " (object " + npc.getObjectId() + ") " + id + " " + a1 + " " + a2);
			});
			t.restrict(loaded);
			ConnectionPool.keep(KEPT);
			ProbeClock.virtual(WALL + t._clock);
			set(GameTimeTaskManager.class, GameTimeTaskManager.getInstance(), "_startTime", WALL + t._clock);
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
				ProbeHeard.listen(null);
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
			for (int id : HALL_OWNED)
				ClanHallManager.getInstance().getClanHall(id).setOwnerId(0);
			HALL_OWNED.clear();
			for (Map.Entry<Integer, Long> e : HALL_DATES.entrySet())
			{
				final SiegableHall hall = ClanHallManager.getInstance().getSiegableHall(e.getKey());
				hall.updateSiegeStatus(SiegeStatus.REGISTRATION_OPENED);
				hall.setNextSiegeDate(e.getValue());
			}
			HALL_DATES.clear();
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
			if ((raw.startsWith(" ") || raw.startsWith("\t")) && !_steps.isEmpty() && seedCheck(line))
			{
				// A step's expectation of the world once it is over: a door's open state, the dungeon flag, the character's standing or attack stance.
				_checks.computeIfAbsent(_steps.size() - 1, k -> new ArrayList<>()).add(line.split("\\s+"));
				continue;
			}
			if (line.startsWith("spawned ") && (raw.startsWith(" ") || raw.startsWith("\t")) && !_steps.isEmpty())
			{
				// A step's expectation: where the NPCs of that template no npc line placed stand once it is over.
				_spawns.computeIfAbsent(_steps.size() - 1, k -> new ArrayList<>()).add(Integer.parseInt(line.split("\\s+")[1]));
				continue;
			}
			if (line.isEmpty() || line.startsWith("#") || raw.startsWith(" ") || raw.startsWith("\t"))
				continue;
			final String[] f = line.split("\\s+");
			if (steps || isStep(f[0]))
			{
				steps = true;
				if (f[0].equals("stance"))
					_stances = true;
				// A bypass keeps its own spacing.
				_steps.add(f[0].equals("bypass") ? new String[] { "bypass", line.substring("bypass".length()).strip() } : asBypass(f, line));
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
				case "clan" ->
				{
					_clan = Integer.parseInt(f[1]);
					if (f.length > 3 && f[2].equals("hall"))
						_clanHall = Integer.parseInt(f[3]);
				}
				case "hall" ->
				{
					_halls.add(new long[]
					{
						Integer.parseInt(f[1]),
						f.length > 4 ? Long.parseLong(f[4]) : 0
					});
					_hallStatuses.add(f[2]);
				}
				case "mate" ->
				{
					// A traced scenario's only mate leads the clan.
					if (!Arrays.asList(f).contains("leader"))
						_unread.add(f[0]);
					_leader = new String[]
					{
						f[1],
						f[2]
					};
					mateFlags(f);
				}
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
				case "clock" -> _clock = clock(f[1]);
				case "npc" -> _npcs.put(f[1], new NpcLine(f[1], Integer.parseInt(f[2]), f[3], Integer.parseInt(f[4]), Integer.parseInt(f[5]), Integer.parseInt(f[6]), npcParams(f), npcAi(f), npcLevel(f), npcSkills(f)));
				case "script", "stub" -> _scripts.add(ScriptProbe.PKG + f[1]);
				case "hold" -> _holds.add(new int[]
				{
					Integer.parseInt(f[1]),
					Integer.parseInt(f[2])
				});
				case "zone" -> _zones.add(new int[]
				{
					Integer.parseInt(f[2]),
					Integer.parseInt(f[3]),
					Integer.parseInt(f[4]),
					Integer.parseInt(f[5]),
					Integer.parseInt(f[6]),
					Integer.parseInt(f[7]),
					Integer.parseInt(f[8])
				});
				case "door" -> _doors.put(f[1], Integer.parseInt(f[2]));
				case "equip" -> _equips.add(Integer.parseInt(f[1]));
				case "standing" -> _standing = Integer.parseInt(f[1]);
				case "pk" -> _pk = Integer.parseInt(f[1]);
				case "karma" -> _karma = Integer.parseInt(f[1]);
				case "hero" -> _hero = true;
				case "warehouse" -> _warehouse.add(new int[]
				{
					Integer.parseInt(f[1]),
					Integer.parseInt(f[2])
				});
				case "noble" -> _noble = true;
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
				case "roll" ->
				{
					// The Go runner pins an NPC's combat draws; the reference's come from its seeded generator,
					// and the trace shows the outcome the pin must reproduce.
				}
				case "item", "page", "teleports" ->
				{
					// The reference loads every item, page and teleport list.
				}
				default -> _unread.add(f[0]);
			}
		}
	}

	/** Whether an indented line is an expectation the probe reads: open, dungeon, or the character's standing or stance. */
	private static boolean seedCheck(String line)
	{
		final String[] f = line.split("\\s+");
		return switch (f[0])
		{
			case "open", "dungeon" -> true;
			case "standing", "stance" -> f.length > 1 && f[1].equals("player");
			default -> false;
		};
	}

	/** Reads a clock line's hh:mm[:ss] as milliseconds since midnight. */
	private static long clock(String time)
	{
		final String[] p = time.split(":");
		return (Long.parseLong(p[0]) * 3600 + Long.parseLong(p[1]) * 60 + (p.length > 2 ? Long.parseLong(p[2]) : 0)) * 1000;
	}

	/** The AI parameters of an npc line's ai option (name=value pairs, comma-separated), null without one. */
	private static Map<String, String> npcAi(String[] f)
	{
		for (int i = 7; i + 1 < f.length; i++)
			if (f[i].equals("ai"))
			{
				final Map<String, String> ai = new LinkedHashMap<>();
				for (String kv : f[i + 1].split(","))
				{
					final int eq = kv.indexOf('=');
					ai.put(kv.substring(0, eq), kv.substring(eq + 1));
				}
				return ai;
			}
		return null;
	}

	/**
	 * The three spawn parameters of an npc line's params option, null without one. The other options
	 * (walks, maker, ...) are not read: the probe plays the shipped template.
	 */
	private static String[] npcParams(String[] f)
	{
		for (int i = 7; i + 3 < f.length; i++)
			if (f[i].equals("params"))
				return new String[] { f[i + 1], f[i + 2], f[i + 3] };
		return null;
	}

	/**
	 * The skills of an npc line's skill options ({type, id, level} each), none without one. Past the
	 * three offsets the options are read as the Go runner reads them: a word that is not an option
	 * name is an operand of the one before.
	 */
	private static String[][] npcSkills(String[] f)
	{
		final List<String[]> skills = new ArrayList<>();
		for (int i = 7; i + 3 < f.length; i++)
			if (f[i].equals("skill"))
				skills.add(new String[] { f[i + 1], f[i + 2], f[i + 3] });
		return skills.toArray(new String[0][]);
	}

	/** The template level of an npc line's level option, 0 without one. */
	private static int npcLevel(String[] f)
	{
		for (int i = 7; i + 1 < f.length; i++)
			if (f[i].equals("level"))
				return Integer.parseInt(f[i + 1]);
		return 0;
	}

	/** The mate line's flags: sponsor or apprentice, online, hold lines, and the offset. */
	private void mateFlags(String[] f)
	{
		int i = 3;
		for (; i < f.length; i++)
		{
			switch (f[i])
			{
				case "leader" ->
				{
				}
				case "sponsor", "apprentice" -> _mateRelation = f[i];
				case "online" -> _mateOnline = true;
				case "hold" ->
				{
					_mateHolds.add(new int[]
					{
						Integer.parseInt(f[i + 1]),
						Integer.parseInt(f[i + 2])
					});
					i += 2;
				}
				default ->
				{
					if (i + 2 != f.length - 1)
						_unread.add(f[0]);
					else
						for (int k = 0; k < 3; k++)
							_mateAt[k] = Integer.parseInt(f[i + k]);
					return;
				}
			}
		}
	}

	/** An as step of a bypass keeps the bypass's own spacing too. */
	private static String[] asBypass(String[] f, String line)
	{
		if (!f[0].equals("as") || f.length < 4 || !f[2].equals("bypass"))
			return f;
		final int at = line.indexOf("bypass", line.indexOf(f[1]) + f[1].length());
		return new String[] { "as", f[1], "bypass", line.substring(at + "bypass".length()).strip() };
	}

	private static boolean isStep(String verb)
	{
		return switch (verb)
		{
			case "action", "bypass", "link", "hit", "kill", "advance", "approach", "tick", "relog", "pickup", "mark", "restart", "enter", "create", "hallstatus", "event", "as", "away", "stance" -> true;
			default -> false;
		};
	}

	private void play() throws Exception
	{
		final GameClient client = new GameClient(null);
		client.setAccountName(ACCOUNT);
		client.setSessionId(new SessionKey(1, 2, 3, 4));
		_client = client;
		_actor = client;

		if (!_steps.isEmpty() && _steps.get(0)[0].equals("create"))
			client.setState(GameClientState.AUTHED);
		else
			seed();

		// A zone line stands the shipped zone of that id in the box, as the Go run stands its own there.
		for (int[] z : _zones)
		{
			final ZoneType zone = ZoneManager.getInstance().getZoneById(z[0]);
			if (zone == null)
				throw new IllegalStateException(_name + ": the datapack has no zone " + z[0]);
			_shapes.put(z[0], zone.getZone());
			set(ZoneType.class, zone, "_zone", new ZoneCuboid(X + z[1], X + z[3], Y + z[2], Y + z[4], Z + z[5], Z + z[6]));
		}
		// A door a scenario names starts closed, whatever an earlier scenario left it.
		for (String role : _doors.keySet())
			door(role).closeMe();
		record();
		for (int index = 0; index < _steps.size(); index++)
		{
			final String[] s = _steps.get(index);
			switch (s[0])
			{
				case "action", "bypass", "link" -> client(s);
				case "as" -> as(s);
				case "away" -> away();
				case "stance" -> stance(s[2]);
				case "kill" ->
				{
					final int n = s.length > 2 ? Integer.parseInt(s[2].substring(1)) : 1;
					for (int i = 0; i < n; i++)
						kill(s[1]);
				}
				case "hit" -> hit(s[1], Integer.parseInt(s[2]));
				case "advance" -> advance(s[1]);
				case "approach" -> approach(s[1]);
				case "event" -> event(s[1], s[2], s[3], s[4]);
				case "tick" ->
				{
					final int n = s.length > 1 ? Integer.parseInt(s[1].substring(1)) : 1;
					for (int i = 0; i < n; i++)
						aiTick();
				}
				case "pickup" -> pickup(Integer.parseInt(s[1]));
				case "mark" -> mark(Integer.parseInt(s[1]));
				case "hallstatus" -> hallStatus(Integer.parseInt(s[1]), s[2]);
				case "relog" ->
				{
					restart();
					enter();
				}
				case "restart" -> restart();
				case "enter" ->
				{
					if (s.length > 1)
						enterMate(s[1]);
					else
						enter();
				}
				case "create" -> create();
				default -> throw new IllegalStateException(_name + ": unknown step " + s[0]);
			}
			for (int id : _lives.getOrDefault(index, List.of()))
				_lines.add("# live " + id + " " + live(id));
			for (int id : _spawns.getOrDefault(index, List.of()))
				_lines.add("# spawned " + id + " " + spawned(id));
			for (String[] c : _checks.getOrDefault(index, List.of()))
			{
				// The line reads as the expectation does, with the value the probe's world holds in place of the expected one.
				final List<String> key = new ArrayList<>(Arrays.asList(c).subList(0, c.length - 1));
				key.add(seedValue(c));
				_lines.add("# check " + String.join(" ", key));
			}
		}
	}

	/** An action, bypass or link step of the current actor. */
	private void client(String[] s) throws Exception
	{
		switch (s[0])
		{
			case "action" -> action(npc(s[1]));
			case "bypass" -> bypass(roles(s[1]));
			default -> link(s[1]);
		}
	}

	/** An as step: the mate sends the step's packet from its client. */
	private void as(String[] s) throws Exception
	{
		if (_mate == null || !_mate.getName().equals(s[1]))
			throw new IllegalStateException(_name + ": as " + s[1] + ": no mate line names it");
		if (_mateClient == null)
			throw new IllegalStateException(_name + ": as " + s[1] + ": the mate is not in the world");
		final String[] step = Arrays.copyOfRange(s, 2, s.length);
		if (!List.of("action", "bypass", "link").contains(step[0]))
			throw new IllegalStateException(_name + ": as " + s[1] + ": the probe plays no " + step[0] + " step of a mate");
		_actor = _mateClient;
		_tag = "@" + s[1];
		try
		{
			client(step);
		}
		finally
		{
			_actor = _client;
			_tag = "";
		}
	}

	/** Where (x and y; the probe has no geodata, so its heights are not the ground's) the NPCs of the template that no npc line placed stand alive, sorted, or - for none. */
	private String spawned(int npcId)
	{
		final List<String> at = World.getInstance().getObjects().stream().filter(o -> o instanceof Npc n && n.getNpcId() == npcId && !n.isDead() && !_spawnedNpcs.contains(o)).map(o -> o.getX() + "," + o.getY()).sorted().toList();
		return at.isEmpty() ? "-" : String.join(" ", at);
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
			final Clan clan = createClan(player);
			clan.changeLevel(_clan);
			if (_siege != null)
				siege(clan);
			if (_clanHall != 0)
				owner(clan, _clanHall);
			if (_mate != null)
				mateTies(player);
		}
		else if (_siege != null)
			throw new IllegalStateException(_name + ": a siege line needs a clan line");
		for (int i = 0; i < _halls.size(); i++)
			hallStatus((int) _halls.get(i)[0], _hallStatuses.get(i), _halls.get(i)[1]);
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
		if (_mate != null && _mateOnline)
			mateOnline();
		spawnNpcs();
	}

	/**
	 * The clan line's clan, led by the character, or by the mate line's character when there is
	 * one: a character in no client, so an offline leader, which the clan then lists the
	 * character under as a plain member.
	 */
	private Clan createClan(Player player) throws Exception
	{
		if (_leader == null)
		{
			final Clan clan = ClanTable.getInstance().createClan(player, CLAN);
			if (clan == null)
				throw new IllegalStateException(_name + ": the clan line's clan was not created");
			return clan;
		}
		final Player leader = Player.create(IdFactory.getInstance().getNextId(), PlayerData.getInstance().getTemplate(ClassId.VALUES[_class]), MATE_ACCOUNT, _leader[0], (byte) 0, (byte) 0, (byte) 0, Sex.MALE);
		leader.getStatus().setLevel(Integer.parseInt(_leader[1]));
		leader.getStatus().setMaxHpMp();
		final Clan clan = ClanTable.getInstance().createClan(leader, CLAN);
		if (clan == null)
			throw new IllegalStateException(_name + ": the clan line's clan was not created");
		clan.addClanMember(player);
		_mate = leader;
		_mateObjectId = leader.getObjectId();
		return clan;
	}

	/**
	 * The mate line's relation: the mate is the character's sponsor (the character its apprentice)
	 * or its apprentice (the character its sponsor), on both sides. The mate is then stored where
	 * the line puts it, so a mate that enters the world by an enter step restores as written.
	 */
	private void mateTies(Player player)
	{
		// A character that entered the world by the character select is in the world's players, where a
		// login notice finds its sponsor or apprentice; the seeded character is added only for a mate that has a relation.
		if (_mateRelation != null)
			World.getInstance().addPlayer(player);
		if ("sponsor".equals(_mateRelation))
		{
			player.setSponsor(_mate.getObjectId());
			_mate.setApprentice(player.getObjectId());
		}
		else if ("apprentice".equals(_mateRelation))
		{
			player.setApprentice(_mate.getObjectId());
			_mate.setSponsor(player.getObjectId());
		}
		_mate.setXYZ(X + _mateAt[0], Y + _mateAt[1], Z + _mateAt[2]);
		if (_mateOnline)
			return;
		if (!_mateHolds.isEmpty())
			throw new IllegalStateException(_name + ": the probe reads a mate's hold lines only for an online mate");
		if (_steps.stream().anyMatch(s -> s[0].equals("enter") && s.length > 1))
			_mate.store();
	}

	/** The online mate line's mate: in the world, in a client of its own, beside the character as the Go fixture's is. */
	private void mateOnline() throws Exception
	{
		final GameClient client = mateClient();
		client.setState(GameClientState.IN_GAME);
		World.getInstance().addObject(_mate);
		client.setPlayer(_mate);
		_mate.setClient(client);
		_mate.spawnMe(X + _mateAt[0], Y + _mateAt[1], Z + _mateAt[2]);
		World.getInstance().addPlayer(_mate);
		for (int[] h : _mateHolds)
			hold(_mate, h);
		_mateClient = client;
	}

	/** A client of the mate's own account. */
	private static GameClient mateClient()
	{
		final GameClient client = new GameClient(null);
		client.setAccountName(MATE_ACCOUNT);
		client.setSessionId(new SessionKey(1, 2, 3, 4));
		return client;
	}

	/**
	 * An enter step of a mate: its client selects the stored character of slot 0 and enters the world,
	 * as the character's own enter step does, so the clan's members and the mate's sponsor or apprentice
	 * are told. The character select's list is sent unrecorded, as the login the Go runner skips.
	 */
	private void enterMate(String name) throws Exception
	{
		if (_mate == null || !_mate.getName().equals(name))
			throw new IllegalStateException(_name + ": enter " + name + ": no mate line names it");
		if (_mateClient != null)
			throw new IllegalStateException(_name + ": enter " + name + ": the mate is in the world");
		final GameClient client = mateClient();
		client.setState(GameClientState.AUTHED);
		final boolean muted = _muted;
		_muted = true;
		try
		{
			client.sendPacket(new CharSelectInfo(client.getAccountName(), client.getSessionId().playOkId1()));
		}
		finally
		{
			_muted = muted;
		}
		Arrays.fill(client.getFloodProtectors(), 0L);
		_actor = client;
		_tag = "@" + name;
		_entering = true;
		try
		{
			final ByteBuffer b = body(18);
			b.putInt(0).putShort((short) 0).putInt(0).putInt(0).putInt(0);
			receive("RequestGameStart 0", new RequestGameStart(), b);
			receive("EnterWorld", new EnterWorld(), body(0));
		}
		finally
		{
			_entering = false;
			_actor = _client;
			_tag = "";
		}
		if (client.getPlayer() == null)
			throw new IllegalStateException(_name + ": the mate did not enter the world");
		_mateClient = client;
		spawnNpcs();
	}

	/** The character's attack stance is set, as a blow it deals sets it, or ended. */
	private void stance(String state)
	{
		_lines.add("# stance " + state);
		if (state.equals("on"))
			AttackStanceTaskManager.getInstance().add(_player);
		else
			AttackStanceTaskManager.getInstance().remove(_player);
	}

	/** What a door, dungeon, standing or stance expectation reads in the probe's world now. */
	private String seedValue(String[] c)
	{
		return switch (c[0])
		{
			case "open" -> yesNo(door(c[1]).isOpened());
			case "dungeon" -> yesNo(_player.isIn7sDungeon());
			case "standing" -> String.valueOf(_player.getAllianceWithVarkaKetra());
			default -> AttackStanceTaskManager.getInstance().isInAttackStance(_player) ? "on" : "off";
		};
	}

	private static String yesNo(boolean b)
	{
		return b ? "yes" : "no";
	}

	/** The datapack door a door line names. */
	private Door door(String role)
	{
		final Integer id = _doors.get(role);
		final Door door = id == null ? null : DoorData.getInstance().getDoor(id);
		if (door == null)
			throw new IllegalStateException(_name + ": no door line names " + role);
		return door;
	}

	/** The character moves 1500 behind where it entered the world, out of the NPCs' sight and the mate's reach, and one NPC AI tick runs. */
	private void away()
	{
		_lines.add("# away");
		_player.setXYZ(X - 1500, Y, Z);
		_player.revalidateZone(true);
		AiTaskManager.getInstance().run();
		ThreadPool.advance(0);
	}

	/** The hall lines of the halls a scenario touched, with the next siege date each had before. */
	private static final Map<Integer, Long> HALL_DATES = new HashMap<>();

	/** The hall line: where the siegable hall's siege stands, and the next siege date it stores. */
	private void hallStatus(int id, String status) throws Exception
	{
		_lines.add("# hallstatus " + id + " " + status);
		hallStatus(id, status, 0);
	}

	private void hallStatus(int id, String status, long date) throws Exception
	{
		final SiegableHall hall = ClanHallManager.getInstance().getSiegableHall(id);
		if (hall == null)
			throw new IllegalStateException(_name + ": no siegable hall " + id);
		HALL_DATES.putIfAbsent(id, hall.getNextSiegeTime());
		hall.updateSiegeStatus(switch (status)
		{
			case "registering" -> SiegeStatus.REGISTRATION_OPENED;
			case "waiting" -> SiegeStatus.REGISTRATION_OVER;
			case "siege" -> SiegeStatus.IN_PROGRESS;
			default -> throw new IllegalStateException(_name + ": no hall status " + status);
		});
		if (date != 0)
			hall.setNextSiegeDate(date);
	}

	/** The clan line's hall: the clan owns the clan hall, as the clan hall restore leaves a stored owner. */
	private void owner(Clan clan, int id)
	{
		final ClanHall hall = ClanHallManager.getInstance().getClanHall(id);
		if (hall == null)
			throw new IllegalStateException(_name + ": no clan hall " + id);
		HALL_OWNED.add(id);
		hall.setOwnerId(clan.getClanId());
		clan.setClanHallId(id);
	}

	private static final Set<Integer> HALL_OWNED = new HashSet<>();

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
	
	/** The hold line h (item id, count): stackable items in one stack, others one by one. */
	private static void hold(Player player, int[] h)
	{
		if (ItemData.getInstance().getTemplate(h[0]).isStackable())
			player.getInventory().addItem(h[0], h[1]);
		else
			for (int i = 0; i < h[1]; i++)
				player.getInventory().addItem(h[0], 1);
	}

	/** The hold, equip, standing, pk, cabal, festival, sealowner, memo and spoil lines, as the Go runner seeds them. */
	private void holdings(Player player) throws Exception
	{
		for (int[] h : _holds)
			hold(player, h);
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
		if (_karma != 0)
			player.setKarma(_karma);
		for (int[] h : _warehouse)
			if (ItemData.getInstance().getTemplate(h[0]).isStackable())
				player.getWarehouse().addItem(h[0], h[1]);
			else
				for (int i = 0; i < h[1]; i++)
					player.getWarehouse().addItem(h[0], 1);
		if (_hero)
			player.setHero(true);
		if (_noble)
			player.setNoble(true, false);
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
			// The Go runner's world holds the scenario's spawns alone: the datapack's makers (loaded, never
			// spawned here) are not what a script's spawn lookup finds.
			alter(SpawnManager.class, SpawnManager.getInstance(), "_makers", ConcurrentHashMap.newKeySet());
			for (NpcLine n : _npcs.values())
			{
				final NpcTemplate template = NpcData.getInstance().getTemplate(n.id());
				casting(template, n.type());
				if (n.ai() != null)
					alter(NpcTemplate.class, template, "_aiParams", new NpcMemo(n.ai()));
				if (fought.contains(n.role()))
				{
					fixture(template, n.type());
					if (n.level() > 0)
						alter(NpcTemplate.class, template, "_level", (byte) n.level());
				}
				if (n.skills().length > 0)
				{
					// The Go runner's template lists the line's skills alone.
					final Map<NpcSkillType, L2Skill> skills = new EnumMap<>(NpcSkillType.class);
					for (String[] sk : n.skills())
						skills.put(NpcSkillType.valueOf(sk[0]), SkillTable.getInstance().getInfo(Integer.parseInt(sk[1]), Integer.parseInt(sk[2])));
					alter(NpcTemplate.class, template, "_skills", skills);
				}
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
		final NpcTemplate template = NpcData.getInstance().getTemplate(n.id());
		final Spawn spawn = new Spawn(template);
		spawn.setLoc(X + n.dx(), Y + n.dy(), Z + n.dz(), 0);
		final Npc npc = n.params() == null ? spawn.doSpawn(false) : spawn(spawn, template, n.params());
		// A spawn clears the spoil state, so each fresh monster of the role is marked after it.
		if (_spoils.containsKey(n.role()))
			((Monster) npc).getSpoilState().setSpoilerId(_spoils.get(n.role()) ? OTHER_SPOILER : _player.getObjectId());
		_live.put(n.role(), npc);
		_spawnedNpcs.add(npc);
		_lines.add("# npc " + n.role() + " " + npc.getNpcId() + " (object " + npc.getObjectId() + ")");
		return npc;
	}

	/**
	 * Spawns as {@code Spawn.doSpawn(false)} does, with the three spawn parameters set on the NPC
	 * before it enters the world (as {@code MinionSpawn.doSpawn(boolean, int, int, int)} does), so
	 * its created hook reads them.
	 */
	private Npc spawn(Spawn spawn, NpcTemplate template, String[] params) throws Exception
	{
		final Constructor<?> constructor = (Constructor<?>) field(ASpawn.class, spawn, "_constructor");
		final Npc npc = (Npc) constructor.newInstance(IdFactory.getInstance().getNextId(), template);
		npc._param1 = ref(params[0]);
		npc._param2 = ref(params[1]);
		npc._param3 = ref(params[2]);
		npc.setSpawn(spawn);
		npc.setShowSummonAnimation(false);
		final Npc spawned = spawn.initializeAndSpawn(npc);
		set(Spawn.class, spawn, "_npc", spawned);
		SpawnManager.getInstance().addSpawn(spawn);
		return spawned;
	}

	/** Resolves an NPC parameter or event argument: a number, the character or the NPC of a role. */
	private int ref(String word)
	{
		if (word.equals("character"))
			return _player.getObjectId();
		if (word.matches("-?\\d+"))
			return Integer.parseInt(word);
		return npc(word).getObjectId();
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

	/** Sends the NPC of role a script event, then runs what it queued now. */
	private void event(String role, String id, String a1, String a2)
	{
		final Npc npc = npc(role);
		_lines.add("# event " + npc.getNpcId() + " (object " + npc.getObjectId() + ") " + id + " " + a1 + " " + a2);
		// The step's own delivery is its line; what a script sends in answer is heard.
		ProbeHeard.own(npc);
		try
		{
			npc.sendScriptEvent(Integer.parseInt(id), ref(a1), ref(a2));
		}
		finally
		{
			ProbeHeard.own(null);
		}
		ThreadPool.advance(0);
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
			// The stance task is not left queued between scenarios, so it runs on the scenario's own time.
			if (_stances)
				AttackStanceTaskManager.getInstance().run();
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
		_lines.add("C" + _tag + " " + label);
		ProbeWire.receive(_actor, packet, b.array());
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
			if (_muted)
				return;
			if (client == _client)
				_lines.add("S " + line);
			else if (!_entering && (client == _mateClient || client == _actor))
				_lines.add("S@" + _leader[0] + " " + line);
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
	private void clear() throws Exception
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
		for (String role : _doors.keySet())
			door(role).closeMe();
		for (Map.Entry<Integer, ZoneForm> e : _shapes.entrySet())
			set(ZoneType.class, ZoneManager.getInstance().getZoneById(e.getKey()), "_zone", e.getValue());
		if (_player != null)
			_player.deleteMe();
		if (_mateClient != null && _mateClient.getPlayer() != null)
			_mateClient.getPlayer().deleteMe();
		if (_mate != null)
			PlayerInfoTable.getInstance().removePlayer(_mateObjectId);
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
			if (_mateClient != null)
				w.printf("# mate %s object %d%n", _leader[0], _mateObjectId);
			for (String l : _lines)
				w.println(l);
		}
	}
}
