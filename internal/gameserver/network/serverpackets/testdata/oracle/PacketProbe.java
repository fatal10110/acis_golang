import net.sf.l2j.commons.mmocore.MMOClient;
import net.sf.l2j.commons.mmocore.PacketBytes;
import net.sf.l2j.commons.mmocore.SendablePacket;
import net.sf.l2j.gameserver.data.manager.RelationManager;
import net.sf.l2j.gameserver.data.sql.PlayerInfoTable;
import net.sf.l2j.gameserver.enums.SayType;
import net.sf.l2j.gameserver.enums.actors.WeightPenalty;
import net.sf.l2j.gameserver.model.World;
import net.sf.l2j.gameserver.model.actor.Boat;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.container.player.BoatInfo;
import net.sf.l2j.gameserver.model.actor.move.BoatMove;
import net.sf.l2j.gameserver.model.actor.status.BoatStatus;
import net.sf.l2j.gameserver.model.location.Location;
import net.sf.l2j.gameserver.model.location.SpawnLocation;
import net.sf.l2j.gameserver.network.SystemMessageId;
import net.sf.l2j.gameserver.network.serverpackets.*;

/**
 * Serializes reference packets through their own writeImpl and prints one
 * "name hex" line per case. The names are the keys the Go replay test looks up.
 */
public final class PacketProbe
{
	static void emit(String name, SendablePacket<?> packet) throws Exception
	{
		System.out.println(name + " " + PacketBytes.hex(packet));
	}

	static void emit(String name, SendablePacket<?> packet, MMOClient<?> client) throws Exception
	{
		System.out.println(name + " " + PacketBytes.hex(packet, client));
	}

	private static Player player(int objectId, String name, boolean online)
	{
		final Player p = new Player();
		p.objectId = objectId;
		p.name = name;
		p.online = online;
		return p;
	}

	private static Boat boat(int objectId, int x, int y, int z, int heading)
	{
		final Boat b = new Boat();
		b.objectId = objectId;
		b.x = x;
		b.y = y;
		b.z = z;
		b.heading = heading;
		return b;
	}

	private static Player rider(int objectId, int boatId, SpawnLocation boatPosition)
	{
		final BoatInfo info = new BoatInfo();
		info.player = player(objectId, "rider", true);
		info.boat = boat(boatId, 0, 0, 0, 0);
		info.boatPosition = boatPosition;
		info.player.boatInfo = info;
		return info.player;
	}

	public static void main(String[] args) throws Exception
	{
		// Boat passenger packets.
		emit("GetOnVehicle", new GetOnVehicle(0x0a0b0c0d, 7, 230, -260, -40));
		emit("GetOffVehicle", new GetOffVehicle(0x0a0b0c0d, 7, -96622, 261660, -3610));
		emit("MoveToLocationInVehicle", new MoveToLocationInVehicle(player(0x0a0b0c0d, "p", true), boat(7, 0, 0, 0, 0), 230, -260, -48, 10, -100, -40));
		emit("StopMoveInVehicle", new StopMoveInVehicle(rider(0x0a0b0c0d, 7, new SpawnLocation(10, -100, -40, 32768)), 7));
		emit("ValidateLocationInVehicle", new ValidateLocationInVehicle(rider(0x0a0b0c0d, 7, new SpawnLocation(230, -260, -40, 61440))));

		// Boat packets.
		emit("VehicleInfo", new VehicleInfo(boat(0x01020304, -96622, 261660, -3610, 32768)));
		emit("OnVehicleCheckLocation", new OnVehicleCheckLocation(boat(7, 48950, 190613, -3610, 60800)));
		final Boat departing = boat(7, 0, 0, 0, 0);
		departing.move = new BoatMove();
		departing.move.destination = new Location(51914, 189023, -3610);
		departing.status = new BoatStatus();
		departing.status.moveSpeed = 150f;
		departing.status.rotationSpeed = 800;
		emit("VehicleDeparture", new VehicleDeparture(departing));
		emit("VehicleStarted/moving", new VehicleStarted(boat(0x0a0b0c0d, 0, 0, 0, 0), 1));
		emit("VehicleStarted/stopped", new VehicleStarted(boat(0x0a0b0c0d, 0, 0, 0, 0), 0));
		emit("BoatSay", new BoatSay(new SystemMessageId(1223)));

		// Chat.
		emit("CreatureSay", new CreatureSay(0x01020304, SayType.TELL, "->Bo", "hi"));

		// Friend packets.
		emit("FriendAddRequestResult/accepted", FriendAddRequestResult.STATIC_ACCEPT);
		emit("FriendAddRequestResult/failed", FriendAddRequestResult.STATIC_FAIL);
		emit("FriendAddRequest", new FriendAddRequest("Bo"));
		emit("L2Friend/add-online", new L2Friend(player(0x01020304, "Bo", true), 1));
		emit("L2Friend/remove-by-name", new L2Friend("Bo", 3));
		emit("L2FriendStatus", new L2FriendStatus(player(7, "Bo", true), true));
		emit("L2FriendSay", new L2FriendSay("B", "A", "hi", 620));

		// FriendList reads the relation, name and world registries.
		RelationManager.getInstance().friends = java.util.List.of(9, 5);
		PlayerInfoTable.getInstance().names.put(9, "On");
		PlayerInfoTable.getInstance().names.put(5, "");
		World.getInstance().players.put(9, player(9, "On", true));
		emit("FriendList", new FriendList(player(1, "me", true)));

		final Player etc = player(1, "me", true);
		etc.blockingAll = true;
		emit("EtcStatusUpdate/blocked", new EtcStatusUpdate(etc));
		final Player full = player(1, "me", true);
		full.charges = 3;
		full.weightPenalty = WeightPenalty.LEVEL_2;
		full.chatBanned = true;
		full.dangerArea = true;
		full.armorGradePenalty = 2;
		full.charmOfCourage = true;
		full.deathPenaltyBuffLevel = 4;
		emit("EtcStatusUpdate/all", new EtcStatusUpdate(full));

		ProbeSimple.run();
		ProbeWorld.run();
		ProbeEngine.run();
		ProbeCharacters.run();
		ProbeItems.run();
		ProbeShops.run();
		ProbePolymorph.run();
	}
}
