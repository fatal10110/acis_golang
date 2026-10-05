import java.io.File;
import java.io.PrintWriter;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Comparator;
import java.util.EnumMap;
import java.util.IdentityHashMap;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;
import java.util.TreeSet;
import java.util.stream.Collectors;

import javax.xml.parsers.DocumentBuilderFactory;

import net.sf.l2j.commons.pool.ConnectionPool;
import net.sf.l2j.commons.pool.ThreadPool;

import net.sf.l2j.Config;
import net.sf.l2j.gameserver.communitybbs.CommunityBoard;
import net.sf.l2j.gameserver.data.SkillTable;
import net.sf.l2j.gameserver.data.cache.CrestCache;
import net.sf.l2j.gameserver.data.cache.HtmCache;
import net.sf.l2j.gameserver.data.manager.BufferManager;
import net.sf.l2j.gameserver.data.manager.BuyListManager;
import net.sf.l2j.gameserver.data.manager.CastleManager;
import net.sf.l2j.gameserver.data.manager.CastleManorManager;
import net.sf.l2j.gameserver.data.manager.ClanHallManager;
import net.sf.l2j.gameserver.data.manager.CursedWeaponManager;
import net.sf.l2j.gameserver.data.manager.FestivalOfDarknessManager;
import net.sf.l2j.gameserver.data.manager.HeroManager;
import net.sf.l2j.gameserver.data.manager.PartyMatchRoomManager;
import net.sf.l2j.gameserver.data.manager.PetitionManager;
import net.sf.l2j.gameserver.data.manager.RaidPointManager;
import net.sf.l2j.gameserver.data.manager.SevenSignsManager;
import net.sf.l2j.gameserver.data.manager.SpawnManager;
import net.sf.l2j.gameserver.data.manager.ZoneManager;
import net.sf.l2j.gameserver.data.sql.BookmarkTable;
import net.sf.l2j.gameserver.data.sql.ClanTable;
import net.sf.l2j.gameserver.data.sql.PlayerInfoTable;
import net.sf.l2j.gameserver.data.sql.ServerMemoTable;
import net.sf.l2j.gameserver.data.xml.AdminData;
import net.sf.l2j.gameserver.data.xml.AnnouncementData;
import net.sf.l2j.gameserver.data.xml.ArmorSetData;
import net.sf.l2j.gameserver.data.xml.AugmentationData;
import net.sf.l2j.gameserver.data.xml.ClanHallDecoData;
import net.sf.l2j.gameserver.data.xml.DoorData;
import net.sf.l2j.gameserver.data.xml.FishData;
import net.sf.l2j.gameserver.data.xml.HealSpsData;
import net.sf.l2j.gameserver.data.xml.HennaData;
import net.sf.l2j.gameserver.data.xml.InstantTeleportData;
import net.sf.l2j.gameserver.data.xml.ItemData;
import net.sf.l2j.gameserver.data.xml.ManorAreaData;
import net.sf.l2j.gameserver.data.xml.MultisellData;
import net.sf.l2j.gameserver.data.xml.NewbieBuffData;
import net.sf.l2j.gameserver.data.xml.NpcData;
import net.sf.l2j.gameserver.data.xml.ObserverGroupData;
import net.sf.l2j.gameserver.data.xml.PlayerData;
import net.sf.l2j.gameserver.data.xml.PlayerLevelData;
import net.sf.l2j.gameserver.data.xml.RecipeData;
import net.sf.l2j.gameserver.data.xml.RestartPointData;
import net.sf.l2j.gameserver.data.xml.ScriptData;
import net.sf.l2j.gameserver.data.xml.SkillTreeData;
import net.sf.l2j.gameserver.data.xml.SoulCrystalData;
import net.sf.l2j.gameserver.data.xml.SpellbookData;
import net.sf.l2j.gameserver.data.xml.StaticObjectData;
import net.sf.l2j.gameserver.data.xml.SummonItemData;
import net.sf.l2j.gameserver.data.xml.TeleportData;
import net.sf.l2j.gameserver.data.xml.WalkerRouteData;
import net.sf.l2j.gameserver.enums.EventHandler;
import net.sf.l2j.gameserver.geoengine.GeoEngine;
import net.sf.l2j.gameserver.idfactory.IdFactory;
import net.sf.l2j.gameserver.model.World;
import net.sf.l2j.gameserver.model.actor.instance.Door;
import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;
import net.sf.l2j.gameserver.model.item.kind.Item;
import net.sf.l2j.gameserver.model.memo.GlobalMemo;
import net.sf.l2j.gameserver.model.olympiad.Olympiad;
import net.sf.l2j.gameserver.model.olympiad.OlympiadGameManager;
import net.sf.l2j.gameserver.model.spawn.NpcMaker;
import net.sf.l2j.gameserver.model.zone.type.subtype.ZoneType;
import net.sf.l2j.gameserver.scripting.Quest;
import net.sf.l2j.gameserver.scripting.ScheduledQuest;
import net.sf.l2j.gameserver.scripting.script.ai.individual.DefaultNpc;
import net.sf.l2j.gameserver.taskmanager.AiTaskManager;
import net.sf.l2j.gameserver.taskmanager.AttackStanceTaskManager;
import net.sf.l2j.gameserver.taskmanager.BoatTaskManager;
import net.sf.l2j.gameserver.taskmanager.DecayTaskManager;
import net.sf.l2j.gameserver.taskmanager.GameTimeTaskManager;
import net.sf.l2j.gameserver.taskmanager.InventoryUpdateTaskManager;
import net.sf.l2j.gameserver.taskmanager.ItemInstanceTaskManager;
import net.sf.l2j.gameserver.taskmanager.ItemsOnGroundTaskManager;
import net.sf.l2j.gameserver.taskmanager.PvpFlagTaskManager;
import net.sf.l2j.gameserver.taskmanager.ShadowItemTaskManager;
import net.sf.l2j.gameserver.taskmanager.WaterTaskManager;

import org.w3c.dom.Element;
import org.w3c.dom.Node;

/**
 * Boots the reference server to its script loader with the probe's shadowed thread pool,
 * random source, send path and connection pool, then writes the registration manifest
 * (gate 1) and the quest packet traces (gate 2).
 */
public final class ScriptProbe
{
	static final String PKG = "net.sf.l2j.gameserver.scripting.";

	public static void main(String[] args) throws Exception
	{
		final File out = new File(args[0]);
		out.mkdirs();

		boot();

		final List<Attempt> attempts = new ArrayList<>();
		final List<NpcTemplate> templates = new ArrayList<>(NpcData.getInstance().getTemplates());
		templates.sort(Comparator.comparingInt(NpcTemplate::getNpcId));

		final Field questEvents = NpcTemplate.class.getDeclaredField("_questEvents");
		questEvents.setAccessible(true);
		for (NpcTemplate t : templates)
		{
			if (!((Map<?, ?>) questEvents.get(t)).isEmpty())
				throw new IllegalStateException("template " + t.getNpcId() + " has script events before the script loader");
			questEvents.set(t, new RecordingMap(t.getNpcId(), attempts));
		}

		// The reference loader: parses scripts.xml, builds every listed class, feeds behaviors.
		ScriptData.getInstance();

		final List<String> paths = listedPaths(new File("./data/xml/scripts.xml"));
		final List<Quest> loaded = ScriptData.getInstance().getQuests();

		try (PrintWriter w = new PrintWriter(new File(out, "manifest.golden"), StandardCharsets.UTF_8))
		{
			new Manifest(w, paths, loaded, attempts, templates).write();
		}

		QuestTrace.run(out);
	}

	/**
	 * Mirrors the reference boot order up to the script loader, minus logging setup,
	 * sockets and the login link.
	 */
	static void boot() throws Exception
	{
		Config.loadGameServer();
		ConnectionPool.init();
		ThreadPool.init();
		IdFactory.getInstance();
		HtmCache.getInstance();
		CrestCache.getInstance();
		World.getInstance();
		AnnouncementData.getInstance();
		ServerMemoTable.getInstance();
		GlobalMemo.getInstance();
		SkillTable.getInstance();
		SkillTreeData.getInstance();
		ItemData.getInstance();
		SummonItemData.getInstance();
		HennaData.getInstance();
		BuyListManager.getInstance();
		MultisellData.getInstance();
		RecipeData.getInstance();
		ArmorSetData.getInstance();
		FishData.getInstance();
		SpellbookData.getInstance();
		SoulCrystalData.getInstance();
		AugmentationData.getInstance();
		CursedWeaponManager.getInstance();
		AdminData.getInstance();
		BookmarkTable.getInstance();
		PetitionManager.getInstance();
		PlayerData.getInstance();
		PlayerInfoTable.getInstance();
		PlayerLevelData.getInstance();
		PartyMatchRoomManager.getInstance();
		RaidPointManager.getInstance();
		HealSpsData.getInstance();
		RestartPointData.getInstance();
		CommunityBoard.getInstance();
		ClanTable.getInstance();
		GeoEngine.getInstance();
		ZoneManager.getInstance();
		DoorData.getInstance().spawn();
		CastleManager.getInstance();
		ClanHallDecoData.getInstance();
		ClanHallManager.getInstance();
		AiTaskManager.getInstance();
		AttackStanceTaskManager.getInstance();
		BoatTaskManager.getInstance();
		DecayTaskManager.getInstance();
		GameTimeTaskManager.getInstance();
		ItemsOnGroundTaskManager.getInstance();
		PvpFlagTaskManager.getInstance();
		ShadowItemTaskManager.getInstance();
		WaterTaskManager.getInstance();
		InventoryUpdateTaskManager.getInstance();
		ItemInstanceTaskManager.getInstance();
		SevenSignsManager.getInstance();
		FestivalOfDarknessManager.getInstance();
		ManorAreaData.getInstance();
		CastleManorManager.getInstance();
		BufferManager.getInstance();
		NpcData.getInstance();
		WalkerRouteData.getInstance();
		StaticObjectData.getInstance();
		SpawnManager.getInstance();
		NewbieBuffData.getInstance();
		InstantTeleportData.getInstance();
		TeleportData.getInstance();
		ObserverGroupData.getInstance();
		CastleManager.getInstance().spawnEntities();
		OlympiadGameManager.getInstance();
		Olympiad.getInstance();
		HeroManager.getInstance();
	}

	/** The script paths of scripts.xml in document order; comments are not elements. */
	static List<String> listedPaths(File f) throws Exception
	{
		final List<String> paths = new ArrayList<>();
		final Element list = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(f).getDocumentElement();
		for (Node n = list.getFirstChild(); n != null; n = n.getNextSibling())
			if (n instanceof Element e && e.getTagName().equals("script") && e.hasAttribute("path"))
				paths.add(e.getAttribute("path"));
		return paths;
	}

	/** One call of the template's event registration. */
	record Attempt(int npcId, EventHandler event, Quest quest)
	{
	}

	/**
	 * Stands in for a template's event map. The template's registration either creates
	 * the first list of an event (a put) or removes the registering script from the
	 * existing list before deciding to append it; both are recorded as one attempt.
	 */
	static final class RecordingMap extends EnumMap<EventHandler, List<Quest>>
	{
		private final int _npcId;
		private final List<Attempt> _attempts;

		RecordingMap(int npcId, List<Attempt> attempts)
		{
			super(EventHandler.class);
			_npcId = npcId;
			_attempts = attempts;
		}

		@Override
		public List<Quest> put(EventHandler key, List<Quest> value)
		{
			for (Quest q : value)
				_attempts.add(new Attempt(_npcId, key, q));
			return super.put(key, new RecordingList(_npcId, key, value, _attempts));
		}
	}

	static final class RecordingList extends ArrayList<Quest>
	{
		private final int _npcId;
		private final EventHandler _event;
		private final transient List<Attempt> _attempts;

		RecordingList(int npcId, EventHandler event, List<Quest> initial, List<Attempt> attempts)
		{
			super(initial);
			_npcId = npcId;
			_event = event;
			_attempts = attempts;
		}

		@Override
		public boolean remove(Object o)
		{
			_attempts.add(new Attempt(_npcId, _event, (Quest) o));
			return super.remove(o);
		}
	}

	static Object field(Object o, Class<?> owner, String name) throws Exception
	{
		final Field f = owner.getDeclaredField(name);
		f.setAccessible(true);
		return f.get(o);
	}

	/** Writes the manifest; see README.md for the format. */
	static final class Manifest
	{
		private final PrintWriter _w;
		private final List<String> _paths;
		private final List<Quest> _loaded;
		private final List<Attempt> _attempts;
		private final List<NpcTemplate> _templates;
		private final Map<Quest, String> _keys = new IdentityHashMap<>();

		Manifest(PrintWriter w, List<String> paths, List<Quest> loaded, List<Attempt> attempts, List<NpcTemplate> templates)
		{
			_w = w;
			_paths = paths;
			_loaded = loaded;
			_attempts = attempts;
			_templates = templates;
		}

		@SuppressWarnings("unchecked")
		void write() throws Exception
		{
			// Match each listed path to its instance; a path whose class failed to build has none.
			final List<Quest> byPath = new ArrayList<>();
			int next = 0;
			for (String path : _paths)
			{
				Quest q = null;
				if (next < _loaded.size() && _loaded.get(next).getClass().getName().equals(PKG + path))
					q = _loaded.get(next++);
				byPath.add(q);
				if (q != null)
					_keys.put(q, (_keys.containsValue(path)) ? path + "#" + _paths.indexOf(path) : path);
			}
			if (next != _loaded.size())
				throw new IllegalStateException("loaded scripts do not follow scripts.xml order");

			_w.println("# Script registration manifest of the reference server: generated by run.sh, do not edit.");
			_w.println("# Format: README.md in this directory.");
			_w.printf("listed %d loaded %d%n", _paths.size(), _loaded.size());
			_w.printf("paths quest.* %d script.* %d task.* %d%n", prefixed("quest."), prefixed("script."), prefixed("task."));
			_w.printf("kinds behavior %d quest %d scheduled %d script %d%n", kinds("behavior"), kinds("quest"), kinds("scheduled"), kinds("script"));

			// Per script.
			final Map<Quest, Map<EventHandler, TreeSet<Integer>>> bindings = new IdentityHashMap<>();
			for (Attempt a : _attempts)
				bindings.computeIfAbsent(a.quest(), k -> new EnumMap<>(EventHandler.class)).computeIfAbsent(a.event(), k -> new TreeSet<>()).add(a.npcId());

			final Map<String, Class<?>> classes = new TreeMap<>();
			for (int i = 0; i < _paths.size(); i++)
			{
				final Quest q = byPath.get(i);
				if (q == null)
				{
					_w.printf("%nscript %s missing%n", _paths.get(i));
					continue;
				}
				final List<String> chain = new ArrayList<>();
				for (Class<?> c = q.getClass(); c != Quest.class; c = c.getSuperclass())
				{
					chain.add(rel(c));
					classes.put(rel(c), c);
				}
				_w.printf("%nscript %s%n", _keys.get(q));
				_w.printf("  kind %s%n", kind(q));
				_w.printf("  quest %d name %s descr %s%n", q.getQuestId(), q.getName(), q.toString().substring(q.toString().indexOf(' ') + 1));
				_w.printf("  chain %s%n", String.join(" < ", chain));
				if (q.getItemsIds() != null)
					_w.printf("  items %s%n", ints(q.getItemsIds()));
				if (q.isTriggeredOnEnterWorld())
					_w.println("  on enter-world");
				if (q.isTriggeredOnDeath())
					_w.println("  on death");
				if (q instanceof ScheduledQuest)
					_w.println("  scheduled");
				for (Map.Entry<TreeSet<Integer>, List<EventHandler>> e : byIds(bindings.getOrDefault(q, Map.of())).entrySet())
					_w.printf("  bind %s %s%n", events(e.getValue()), ranges(e.getKey()));
			}

			// Per class in any script's chain: its own overriding methods and their parent calls.
			_w.printf("%n# classes%n");
			for (Map.Entry<String, Class<?>> e : classes.entrySet())
				writeClass(e.getKey(), e.getValue());

			// The folded per-(npc, event) lists after the whole load.
			_w.printf("%n# folded npc events%n");
			final Field questEvents = NpcTemplate.class.getDeclaredField("_questEvents");
			questEvents.setAccessible(true);
			final Map<String, Map<EventHandler, TreeSet<Integer>>> folded = new TreeMap<>();
			for (NpcTemplate t : _templates)
			{
				final Map<EventHandler, List<Quest>> m = (Map<EventHandler, List<Quest>>) questEvents.get(t);
				for (Map.Entry<EventHandler, List<Quest>> e : m.entrySet())
					if (!e.getValue().isEmpty())
						folded.computeIfAbsent(keys(e.getValue()), k -> new EnumMap<>(EventHandler.class)).computeIfAbsent(e.getKey(), k -> new TreeSet<>()).add(t.getNpcId());
			}
			for (Map.Entry<String, Map<EventHandler, TreeSet<Integer>>> f : folded.entrySet())
				for (Map.Entry<TreeSet<Integer>, List<EventHandler>> e : byIds(f.getValue()).entrySet())
					_w.printf("fold npc %s %s %s%n", events(e.getValue()), f.getKey(), ranges(e.getKey()));

			// The other registration targets, folded the same way.
			_w.printf("%n# folded other events%n");
			final List<Door> doors = new ArrayList<>(DoorData.getInstance().getDoors());
			doors.sort(Comparator.comparingInt(Door::getDoorId));
			for (Door d : doors)
			{
				@SuppressWarnings("unchecked")
				final List<Quest> l = (List<Quest>) field(d, Door.class, "_quests");
				if (l != null && !l.isEmpty())
					_w.printf("fold door %d DOOR_CHANGE %s%n", d.getDoorId(), keys(l));
			}
			for (Item it : ItemData.getInstance().getTemplates())
				if (it != null && !it.getQuestEvents().isEmpty())
					_w.printf("fold item %d ITEM_USE %s%n", it.getItemId(), keys(it.getQuestEvents()));
			final List<ZoneType> zones = new ArrayList<>();
			for (Map<?, ?> byId : ((Map<?, Map<?, ?>>) field(ZoneManager.getInstance(), ZoneManager.class, "_zones")).values())
				for (Object z : byId.values())
					zones.add((ZoneType) z);
			zones.sort(Comparator.comparingInt(ZoneType::getId).thenComparing(z -> z.getClass().getName()));
			for (ZoneType z : zones)
			{
				@SuppressWarnings("unchecked")
				final Map<EventHandler, List<Quest>> m = (Map<EventHandler, List<Quest>>) field(z, ZoneType.class, "_questEvents");
				if (m != null)
					for (Map.Entry<EventHandler, List<Quest>> e : m.entrySet())
						if (!e.getValue().isEmpty())
							_w.printf("fold zone %d %s %s%n", z.getId(), e.getKey(), keys(e.getValue()));
			}
			@SuppressWarnings("unchecked")
			final List<Quest> gameTime = (List<Quest>) field(GameTimeTaskManager.getInstance(), GameTimeTaskManager.class, "_questEvents");
			if (!gameTime.isEmpty())
				_w.printf("fold game-time - GAME_TIME %s%n", keys(gameTime));
			@SuppressWarnings("unchecked")
			final List<NpcMaker> makers = new ArrayList<>((java.util.Set<NpcMaker>) field(SpawnManager.getInstance(), SpawnManager.class, "_makers"));
			makers.sort(Comparator.comparing(NpcMaker::getName));
			for (NpcMaker m : makers)
				if (!m.getQuestEvents().isEmpty())
					_w.printf("fold maker %s MAKER_NPCS_KILLED %s%n", m.getName(), keys(m.getQuestEvents()));
		}

		private void writeClass(String name, Class<?> c) throws Exception
		{
			_w.printf("class %s extends %s%n", name, rel(c.getSuperclass()));
			final Map<String, ClassBytes.MethodCode> code = ClassBytes.read(c);
			final List<Method> own = new ArrayList<>();
			for (Method m : c.getDeclaredMethods())
				if (!m.isSynthetic() && !m.isBridge() && overrides(c, m))
					own.add(m);
			own.sort(Comparator.comparing(Method::getName).thenComparing(m -> ClassBytes.descriptor(m)));
			final List<Method> named = new ArrayList<>();
			for (Method m : c.getDeclaredMethods())
				if (!m.isSynthetic() && !m.isBridge() && !overrides(c, m) && eventOf(m.getName()) != null)
					named.add(m);
			named.sort(Comparator.comparing(Method::getName).thenComparing(m -> ClassBytes.descriptor(m)));
			for (Method m : named)
				_w.printf("  named %s(%s) event %s%n", m.getName(), params(m), eventOf(m.getName()));
			for (Method m : own)
			{
				final String desc = ClassBytes.descriptor(m);
				final ClassBytes.MethodCode mc = code.get(m.getName() + desc);
				final String calls = mc == null ? "" : mc.superCalls().stream().map(sc -> " " + sc.render(c, m, desc)).collect(Collectors.joining());
				_w.printf("  hook %s(%s)%s%s%n", m.getName(), params(m), eventOf(m.getName()) == null ? "" : " event " + eventOf(m.getName()), calls);
			}
		}

		private long prefixed(String prefix)
		{
			return _paths.stream().filter(p -> p.startsWith(prefix)).count();
		}
		
		private long kinds(String kind)
		{
			return _loaded.stream().filter(q -> kind(q).equals(kind)).count();
		}
		
		private String keys(List<Quest> l)
		{
			return l.stream().map(q -> _keys.getOrDefault(q, "?" + q.getClass().getName())).collect(Collectors.joining(","));
		}
	}

	/** True when an ancestor declares a non-private, non-static method of the same signature. */
	static boolean overrides(Class<?> c, Method m)
	{
		if (Modifier.isStatic(m.getModifiers()) || Modifier.isPrivate(m.getModifiers()))
			return false;
		for (Class<?> s = c.getSuperclass(); s != null && s != Object.class; s = s.getSuperclass())
		{
			try
			{
				final Method sm = s.getDeclaredMethod(m.getName(), m.getParameterTypes());
				if (!Modifier.isPrivate(sm.getModifiers()) && !Modifier.isStatic(sm.getModifiers()))
					return true;
			}
			catch (NoSuchMethodException e)
			{
				// Keep walking.
			}
		}
		return false;
	}

	/** The event a method name binds to under the reference's name rule, or null. */
	static EventHandler eventOf(String method)
	{
		for (EventHandler h : EventHandler.values())
			if (("on" + h.name().replace("_", "")).equalsIgnoreCase(method))
				return h;
		return null;
	}

	static String kind(Quest q)
	{
		if (q instanceof DefaultNpc)
			return "behavior";
		if (q.getQuestId() > 0)
			return "quest";
		if (q instanceof ScheduledQuest)
			return "scheduled";
		return "script";
	}

	static String rel(Class<?> c)
	{
		return rel(c.getName());
	}
	
	static String rel(String n)
	{
		return n.startsWith(PKG) ? n.substring(PKG.length()) : n;
	}

	static String params(Method m)
	{
		return Arrays.stream(m.getParameterTypes()).map(Class::getSimpleName).collect(Collectors.joining(","));
	}

	/**
	 * Groups events by identical id sets, keeping the groups in the order of their first
	 * event and each group's events in enum order.
	 */
	static Map<TreeSet<Integer>, List<EventHandler>> byIds(Map<EventHandler, TreeSet<Integer>> m)
	{
		final Map<TreeSet<Integer>, List<EventHandler>> out = new java.util.LinkedHashMap<>();
		final Map<EventHandler, TreeSet<Integer>> sorted = new EnumMap<>(EventHandler.class);
		sorted.putAll(m);
		for (Map.Entry<EventHandler, TreeSet<Integer>> e : sorted.entrySet())
			out.computeIfAbsent(e.getValue(), k -> new ArrayList<>()).add(e.getKey());
		return out;
	}
	
	static String events(List<EventHandler> l)
	{
		return l.stream().map(Enum::name).collect(Collectors.joining(","));
	}
	
	static String ints(int[] a)
	{
		return Arrays.stream(a).mapToObj(Integer::toString).collect(Collectors.joining(","));
	}

	/** Sorted ids with runs of three or more consecutive ids written as a-b. */
	static String ranges(TreeSet<Integer> ids)
	{
		final StringBuilder sb = new StringBuilder();
		final Integer[] a = ids.toArray(new Integer[0]);
		for (int i = 0; i < a.length;)
		{
			int j = i;
			while (j + 1 < a.length && a[j + 1] == a[j] + 1)
				j++;
			if (sb.length() > 0)
				sb.append(',');
			if (j - i >= 2)
				sb.append(a[i]).append('-').append(a[j]);
			else
			{
				sb.append(a[i]);
				if (j > i)
					sb.append(',').append(a[j]);
			}
			i = j + 1;
		}
		return sb.toString();
	}
}
