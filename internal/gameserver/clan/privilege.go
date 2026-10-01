package clan

// Privilege is one clan rank privilege bit, as the clan_privs privs column
// and the client's privilege window encode it.
type Privilege int32

// The clan rank privileges.
const (
	PrivNone Privilege = 0

	PrivInvite          Privilege = 2
	PrivManageTitles    Privilege = 4
	PrivWarehouseSearch Privilege = 8
	PrivManageRanks     Privilege = 16
	PrivClanWar         Privilege = 32
	PrivDismiss         Privilege = 64
	PrivEditCrest       Privilege = 128
	PrivMasterRights    Privilege = 256
	PrivManageLevels    Privilege = 512

	PrivHallEntryExit Privilege = 1024
	PrivHallFunctions Privilege = 2048
	PrivHallAuction   Privilege = 4096
	PrivHallDismiss   Privilege = 8192
	PrivHallSetFuncs  Privilege = 16384

	PrivCastleEntryExit  Privilege = 32768
	PrivCastleManor      Privilege = 65536
	PrivCastleSiege      Privilege = 131072
	PrivCastleFunctions  Privilege = 262144
	PrivCastleDismiss    Privilege = 524288
	PrivCastleTaxes      Privilege = 1048576
	PrivCastleMercenary  Privilege = 2097152
	PrivCastleSetFuncs   Privilege = 4194304
	PrivAll              Privilege = 8388606
	academyPrivilegeMask           = PrivWarehouseSearch | PrivHallEntryExit | PrivHallFunctions | PrivCastleEntryExit | PrivCastleFunctions
)

// The power grades a rank privilege can be set on; 9 is the academy's.
const (
	minPrivilegeRank = 1
	academyRank      = 9
)
