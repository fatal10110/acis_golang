import java.io.File;
import java.io.PrintWriter;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.commons.mmocore.ProbeWire;
import net.sf.l2j.commons.pool.ConnectionPool;
import net.sf.l2j.commons.pool.ThreadPool;
import net.sf.l2j.commons.random.Rnd;

import net.sf.l2j.gameserver.data.xml.NpcData;
import net.sf.l2j.gameserver.data.xml.PlayerData;
import net.sf.l2j.gameserver.data.xml.PlayerLevelData;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.Sex;
import net.sf.l2j.gameserver.idfactory.IdFactory;
import net.sf.l2j.gameserver.model.World;
import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.spawn.Spawn;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.network.GameClient.GameClientState;
import net.sf.l2j.gameserver.network.clientpackets.Action;
import net.sf.l2j.gameserver.network.clientpackets.L2GameClientPacket;
import net.sf.l2j.gameserver.network.clientpackets.RequestBypassToServer;
import net.sf.l2j.gameserver.scripting.QuestState;

/**
 * Gate 2: two quests played through client packets (target and interact, bypasses) on a
 * player and NPCs placed side by side. Each trace lists, in order, the client packets the
 * probe feeds ({@code C}), the virtual time passed after each of them ({@code T}), every
 * server packet body sent to the player ({@code S}) and every SQL statement executed
 * ({@code Q}). The random source is reseeded before each quest.
 */
final class QuestTrace
{
	private static final int X = -84318;
	private static final int Y = 244579;
	private static final int Z = -3730;
	
	private static final String Q001 = "Q001_LettersOfLove";
	private static final String Q003 = "Q003_WillTheSealBeBroken";

	/** Virtual time passed after each client packet: one run of every 333 ms task. */
	private static final long STEP = 500;

	private final List<String> _lines = new ArrayList<>();
	private final GameClient _client;
	private final Player _player;

	private QuestTrace(ClassId classId, String name, int level) throws Exception
	{
		final Player player = Player.create(IdFactory.getInstance().getNextId(), PlayerData.getInstance().getTemplate(classId), "probe", name, (byte) 0, (byte) 0, (byte) 0, Sex.MALE);
		player.getStatus().setLevel(level);
		player.getStatus().setExp(PlayerLevelData.getInstance().getPlayerLevel(level).requiredExpToLevelUp());
		player.getStatus().setMaxHpMp();
		World.getInstance().addObject(player);

		final GameClient client = new GameClient(null);
		client.setState(GameClientState.IN_GAME);
		client.setPlayer(player);
		player.setClient(client);
		player.spawnMe(X, Y, Z);

		_client = client;
		_player = player;
	}

	static void run(File out) throws Exception
	{
		// Run what the boot queued for immediate execution before any trace starts.
		ThreadPool.advance(0);
		
		Rnd.reseed(1);
		final QuestTrace q001 = new QuestTrace(ClassId.HUMAN_FIGHTER, "ProbeOne", 2);
		q001.letters();
		q001.write(new File(out, "trace_q001.golden"), "Q001_LettersOfLove: accept at Darin, deliver to Roxxy, back to Darin, to Baulro, back to Darin.");

		Rnd.reseed(1);
		final QuestTrace q003 = new QuestTrace(ClassId.DARK_FIGHTER, "ProbeThree", 16);
		q003.seal();
		q003.write(new File(out, "trace_q003.golden"), "Q003_WillTheSealBeBroken: accept at Talloth, kill one of each of the three monster kinds, report to Talloth.");
	}

	private void letters() throws Exception
	{
		final Npc darin = spawn(30048, 40);
		final Npc roxxy = spawn(30006, -40);
		final Npc baulro = spawn(30033, 80);

		record();
		talk(darin, Q001);
		bypass("Quest " + Q001 + " 30048-03.htm");
		bypass("Quest " + Q001 + " 30048-04.htm");
		bypass("Quest " + Q001 + " 30048-06.htm");
		talk(darin, Q001);
		talk(roxxy, Q001);
		talk(darin, Q001);
		talk(baulro, Q001);
		talk(darin, Q001);
		talk(darin, Q001);
		stop();
		requireCompleted(Q001);
	}

	private void seal() throws Exception
	{
		final Npc talloth = spawn(30141, 40);

		record();
		talk(talloth, Q003);
		bypass("Quest " + Q003 + " 30141-03.htm");
		talk(talloth, Q003);
		kill(20031);
		kill(20041);
		kill(20048);
		talk(talloth, Q003);
		stop();
		requireCompleted(Q003);
	}

	/**
	 * Selects the NPC unless it is the current target, interacts with it, then clicks the quest
	 * link of its chat window. An NPC tied to more than one quest answers with the quest chooser
	 * (Roxxy: Q001 and Q006); the probe then clicks the traced quest's entry, as a player would.
	 * The chooser's entry is the only window link of that form, and every window replaces the
	 * player's accepted links, so the check reads exactly what the last window offered.
	 */
	private void talk(Npc npc, String quest) throws Exception
	{
		if (_player.getTarget() != npc)
			action(npc);
		action(npc);
		final String base = "npc_" + npc.getObjectId() + "_Quest";
		bypass(base);
		if (_player.validateBypass(base + " " + quest))
			bypass(base + " " + quest);
	}

	/** Fails the run unless the traced quest reached its completed state. */
	private void requireCompleted(String quest)
	{
		final QuestState st = _player.getQuestList().getQuestState(quest);
		if (st == null || !st.isCompleted())
			throw new IllegalStateException(quest + " trace did not complete: state " + (st == null ? "none" : st.getState() + " cond " + st.getCond()));
	}

	private void kill(int npcId) throws Exception
	{
		final Npc monster = spawn(npcId, 60);
		_lines.add("# kill " + npcId + " (object " + monster.getObjectId() + ")");
		monster.reduceCurrentHp(monster.getStatus().getMaxHp() + 1, _player, null);
		
		// The dying hook runs 3 s after the death.
		for (int i = 0; i < 7; i++)
			tick();
	}

	private Npc spawn(int npcId, int dx) throws Exception
	{
		final Spawn spawn = new Spawn(NpcData.getInstance().getTemplate(npcId));
		spawn.setLoc(X + dx, Y, Z, 0);
		return spawn.doSpawn(false);
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
		final ByteBuffer b = body(2 * command.length() + 2);
		for (char c : command.toCharArray())
			b.putChar(c);
		b.putChar('\0');
		receive("RequestBypassToServer " + command, new RequestBypassToServer(), b);
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

	private void write(File f, String title) throws Exception
	{
		try (PrintWriter w = new PrintWriter(f, StandardCharsets.UTF_8))
		{
			w.println("# Quest packet trace of the reference server: generated by run.sh, do not edit.");
			w.println("# " + title);
			w.printf("# player object %d; format: README.md in this directory.%n", _player.getObjectId());
			for (String l : _lines)
				w.println(l);
		}
	}
}
