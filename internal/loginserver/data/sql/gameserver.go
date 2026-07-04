package sql

import (
	"database/sql"
	"errors"
	"fmt"
	"math/big"

	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

// ErrGameServerNotFound is returned when no gameservers row matches the id.
var ErrGameServerNotFound = errors.New("game server not found")

// GameServerStore reads and writes the gameservers table.
type GameServerStore struct {
	db *sql.DB
}

// NewGameServerStore returns a GameServerStore backed by db.
func NewGameServerStore(db *sql.DB) *GameServerStore {
	return &GameServerStore{db: db}
}

// GameServer returns the registered game server for id.
func (s *GameServerStore) GameServer(id int) (model.GameServer, error) {
	var hexID, host string
	err := s.db.QueryRow("SELECT hexid, host FROM gameservers WHERE server_id = ?", id).Scan(&hexID, &host)
	if errors.Is(err, sql.ErrNoRows) {
		return model.GameServer{}, ErrGameServerNotFound
	}
	if err != nil {
		return model.GameServer{}, fmt.Errorf("query game server %d: %w", id, err)
	}

	key, err := parseHexID(hexID)
	if err != nil {
		return model.GameServer{}, fmt.Errorf("parse game server %d hex id: %w", id, err)
	}
	return model.NewGameServer(id, key, host), nil
}

// GameServers returns all registered game servers keyed by server id.
func (s *GameServerStore) GameServers() (map[int]model.GameServer, error) {
	rows, err := s.db.Query("SELECT server_id, hexid, host FROM gameservers")
	if err != nil {
		return nil, fmt.Errorf("query game servers: %w", err)
	}
	defer rows.Close()

	servers := make(map[int]model.GameServer)
	for rows.Next() {
		var id int
		var hexID, host string
		if err := rows.Scan(&id, &hexID, &host); err != nil {
			return nil, fmt.Errorf("scan game server: %w", err)
		}
		key, err := parseHexID(hexID)
		if err != nil {
			return nil, fmt.Errorf("parse game server %d hex id: %w", id, err)
		}
		servers[id] = model.NewGameServer(id, key, host)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate game servers: %w", err)
	}
	return servers, nil
}

// CreateGameServer inserts a registered game server row.
func (s *GameServerStore) CreateGameServer(server model.GameServer) error {
	if _, err := s.db.Exec(
		"INSERT INTO gameservers (hexid, server_id, host) VALUES (?, ?, ?)",
		HexIDText(server.HexID),
		server.ID,
		server.Host,
	); err != nil {
		return fmt.Errorf("create game server %d: %w", server.ID, err)
	}
	return nil
}

// DeleteGameServer removes the registered game server row for id. Deleting
// an id with no row is not an error.
func (s *GameServerStore) DeleteGameServer(id int) error {
	if _, err := s.db.Exec("DELETE FROM gameservers WHERE server_id = ?", id); err != nil {
		return fmt.Errorf("delete game server %d: %w", id, err)
	}
	return nil
}

// DeleteAllGameServers removes every registered game server row.
func (s *GameServerStore) DeleteAllGameServers() error {
	if _, err := s.db.Exec("TRUNCATE gameservers"); err != nil {
		return fmt.Errorf("delete all game servers: %w", err)
	}
	return nil
}

// SetGameServerHost updates a registered game server host.
func (s *GameServerStore) SetGameServerHost(id int, host string) error {
	res, err := s.db.Exec("UPDATE gameservers SET host = ? WHERE server_id = ?", host, id)
	if err != nil {
		return fmt.Errorf("set game server %d host: %w", id, err)
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return ErrGameServerNotFound
	}
	return nil
}

func parseHexID(text string) ([]byte, error) {
	n, ok := new(big.Int).SetString(text, 16)
	if !ok {
		return nil, fmt.Errorf("invalid hex id %q", text)
	}
	return signedBigIntBytes(n), nil
}

// HexIDText renders a game server auth key in the signed big-integer hex
// form used by the gameservers table's hexid column and by hexid files.
// Keys whose top bit is set render as a negative hex string; a leading zero
// byte is dropped on the way through, so text -> bytes round-trips to the
// minimal two's-complement form, not necessarily the original length.
func HexIDText(id []byte) string {
	if id == nil {
		return "null"
	}
	return signedBytesInt(id).Text(16)
}

func signedBytesInt(b []byte) *big.Int {
	n := new(big.Int).SetBytes(b)
	if len(b) == 0 || b[0]&0x80 == 0 {
		return n
	}

	mod := new(big.Int).Lsh(big.NewInt(1), uint(len(b)*8))
	return n.Sub(n, mod)
}

func signedBigIntBytes(n *big.Int) []byte {
	if n.Sign() == 0 {
		return []byte{0}
	}
	if n.Sign() > 0 {
		b := n.Bytes()
		if b[0]&0x80 == 0 {
			return b
		}
		return append([]byte{0}, b...)
	}

	length := 1
	for {
		min := new(big.Int).Lsh(big.NewInt(1), uint(8*length-1))
		min.Neg(min)
		if n.Cmp(min) >= 0 {
			break
		}
		length++
	}

	mod := new(big.Int).Lsh(big.NewInt(1), uint(8*length))
	b := new(big.Int).Add(mod, n).Bytes()
	if len(b) >= length {
		return b
	}
	out := make([]byte, length)
	for i := 0; i < length-len(b); i++ {
		out[i] = 0xff
	}
	copy(out[length-len(b):], b)
	return out
}
