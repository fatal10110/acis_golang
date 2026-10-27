#!/bin/sh
# Regenerates ../packets.golden from the unmodified reference packet classes.
# ACIS_JAVA points at the reference source root (the directory holding net/);
# by default it is the aCis_gameserver checkout next to the Go repository.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
ref=${ACIS_JAVA:-$(git -C "$here" rev-parse --show-toplevel)/../aCis_gameserver/java}
docker run --rm -v "$ref:/ref:ro" -v "$here:/p" -w /p eclipse-temurin:21-jdk-alpine sh -c '
	set -e
	out=$(mktemp -d)
	# stub/ shadows the engine classes the packets read; the packet classes,
	# their base classes and the value types come from the reference tree.
	javac -nowarn -d "$out" -sourcepath stub:/ref \
		$(find stub -name "*.java") PacketProbe.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/GetOnVehicle.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/GetOffVehicle.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/MoveToLocationInVehicle.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/StopMoveInVehicle.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/ValidateLocationInVehicle.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/VehicleInfo.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/OnVehicleCheckLocation.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/VehicleDeparture.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/VehicleStarted.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/BoatSay.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/CreatureSay.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/FriendAddRequestResult.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/FriendAddRequest.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/L2Friend.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/L2FriendStatus.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/L2FriendSay.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/FriendList.java \
		/ref/net/sf/l2j/gameserver/network/serverpackets/EtcStatusUpdate.java
	java -cp "$out" PacketProbe' > "$here/../packets.golden"
