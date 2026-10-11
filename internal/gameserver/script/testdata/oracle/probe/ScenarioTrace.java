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
import net.sf.l2j.commons.pool.ConnectionPool;
import net.sf.l2j.commons.pool.ThreadPool;
import net.sf.l2j.commons.random.Rnd;

import net.sf.l2j.gameserver.data.sql.ClanTable;
import net.sf.l2j.gameserver.data.xml.NpcData;
import net.sf.l2j.gameserver.data.xml.PlayerData;
import net.sf.l2j.gameserver.data.xml.PlayerLevelData;
import net.sf.l2j.gameserver.data.xml.ScriptData;
import net.sf.l2j.gameserver.enums.EventHandler;
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
import net.sf.l2j.gameserver.model.pledge.Clan;
import net.sf.l2j.gameserver.model.spawn.Spawn;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.network.GameClient.GameClientState;
import net.sf.l2j.gameserver.network.clientpackets.Action;
import net.sf.l2j.gameserver.network.clientpackets.L2GameClientPacket;
import net.sf.l2j.gameserver.network.clientpackets.RequestBypassToServer;
import net.sf.l2j.gameserver.network.clientpackets.RequestLinkHtml;
import net.sf.l2j.gameserver.scripting.Quest;
import net.sf.l2j.gameserver.scripting.QuestState;

/**
 * Quest scenario traces: each scenario file of the Go quest suite whose {@code trace} line names
 * a file under {@code oracle/scenarios/} is played here, from the scenario itself, and its trace
 * is written to that file. The header sets the player up as the Go runner does (name, level,
 * class, a led clan of the given level, quest rows, the script draws of the random line) and
 * spawns each NPC line's NPC at its offset from the player, who stands where the Go fixture
 * character enters the world. The NPCs keep only the bindings of the scenario's script and stub
 * lines, as the Go runner registers no other script. A monster the scenario kills or hits is made the Go runner's
 * fixture monster: level 1, 1000 HP, no experience, SP, drops, skills, minions or aggression.
 * <p>
 * The steps run in order until the first one a trace does not record (relog, approach, and the
 * steps of a created character): action, bypass and link through the client packets, kill (each
 * of N kills alone, on a fresh monster once the last one died), hit and advance.
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

	private final String _name;
	private final List<String> _lines = new ArrayList<>();
	private final Map<String, NpcLine> _npcs = new LinkedHashMap<>();
	private final Map<String, Npc> _live = new HashMap<>();
	private final List<String[]> _quests = new ArrayList<>();
	private final List<String[]> _steps = new ArrayList<>();
	/** The script and stub lines' classes: the scripts the scenario runs. */
	private final Set<String> _scripts = new HashSet<>();
	/** The NPC templates' own script bindings, kept while the scenario's replace them. */
	private final Map<NpcTemplate, Map<EventHandler, List<Quest>>> _bindings = new HashMap<>();
	private String _trace;
	private String _character;
	private int _level;
	private int _class;
	private int _clan = -1;
	private int[] _draws;
	private boolean _always;

	private GameClient _client;
	private Player _player;

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

			// Drop what an earlier scenario left queued, so no task of it runs here.
			ThreadPool.cancelAll();
			Rnd.reseed(1);
			if (t._draws != null)
				Rnd.scriptCallers(t._draws, t._always, d -> t._lines.add("# draw " + d));
			t.restrict(loaded);
			try
			{
				t.play();
			}
			finally
			{
				Rnd.endScriptCallers();
				stop();
				t.restore();
			}
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
				default ->
				{
					// item, page: the reference loads every item and page.
				}
			}
		}
	}

	private static boolean isStep(String verb)
	{
		return switch (verb)
		{
			case "action", "bypass", "link", "hit", "kill", "advance", "approach", "relog", "pickup", "mark", "restart", "enter", "create" -> true;
			default -> false;
		};
	}

	private void play() throws Exception
	{
		if (!_steps.isEmpty() && _steps.get(0)[0].equals("create"))
			throw new IllegalStateException(_name + ": a created character is not traced");

		final Player player = Player.create(IdFactory.getInstance().getNextId(), PlayerData.getInstance().getTemplate(ClassId.VALUES[_class]), "probe", _character, (byte) 0, (byte) 0, (byte) 0, Sex.MALE);
		player.getStatus().setLevel(_level);
		player.getStatus().setExp(PlayerLevelData.getInstance().getPlayerLevel(_level).requiredExpToLevelUp());
		player.getStatus().setMaxHpMp();
		World.getInstance().addObject(player);

		final GameClient client = new GameClient(null);
		client.setState(GameClientState.IN_GAME);
		client.setPlayer(player);
		player.setClient(client);
		player.spawnMe(X, Y, Z);
		_client = client;
		_player = player;

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
			}
		}

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
				default ->
				{
					// The first step a trace does not record ends it.
					return;
				}
			}
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
			if (client == _client)
				_lines.add("S " + line);
		});
		ConnectionPool.setSink(sql -> _lines.add("Q " + sql));
	}

	private static void stop()
	{
		ProbeWire.setSink(null);
		ConnectionPool.setSink(null);
	}

	/** Removes the scenario's player and every NPC near it, so the next scenario starts alone. */
	private void clear()
	{
		final List<Npc> near = new ArrayList<>();
		for (WorldObject o : World.getInstance().getObjects())
			if (o instanceof Npc npc && Math.abs(npc.getX() - X) < 3000 && Math.abs(npc.getY() - Y) < 3000)
				near.add(npc);
		for (Npc npc : near)
			npc.deleteMe();
		_player.deleteMe();
		ThreadPool.cancelAll();
	}

	private void write(File f) throws Exception
	{
		try (PrintWriter w = new PrintWriter(f, StandardCharsets.UTF_8))
		{
			w.println("# Quest scenario trace of the reference server: generated by run.sh, do not edit.");
			w.println("# " + _name + ": the scenario's steps up to the first one a trace does not record.");
			w.printf("# player object %d; format: README.md in this directory.%n", _player.getObjectId());
			for (String l : _lines)
				w.println(l);
		}
	}
}
