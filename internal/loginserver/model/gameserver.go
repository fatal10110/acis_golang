package model

// GameServer is a registered game server row.
type GameServer struct {
	ID    int
	HexID []byte
	Host  string
}

// NewGameServer returns a GameServer with its auth key copied.
func NewGameServer(id int, hexID []byte, host string) GameServer {
	key := append([]byte(nil), hexID...)
	return GameServer{ID: id, HexID: key, Host: host}
}
