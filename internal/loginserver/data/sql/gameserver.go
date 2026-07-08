package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
func (s *GameServerStore) GameServer(ctx context.Context, id int) (model.GameServer, error) {
	var hexID, host string
	err := s.db.QueryRowContext(ctx, "SELECT hexid, host FROM gameservers WHERE server_id = ?", id).Scan(&hexID, &host)
	if errors.Is(err, sql.ErrNoRows) {
		return model.GameServer{}, ErrGameServerNotFound
	}
	if err != nil {
		return model.GameServer{}, fmt.Errorf("query game server %d: %w", id, err)
	}

	key, err := model.ParseHexKey(hexID)
	if err != nil {
		return model.GameServer{}, fmt.Errorf("parse game server %d hex id: %w", id, err)
	}
	return model.NewGameServer(id, key, host), nil
}

// GameServers returns all registered game servers keyed by server id.
func (s *GameServerStore) GameServers(ctx context.Context) (map[int]model.GameServer, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT server_id, hexid, host FROM gameservers")
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
		key, err := model.ParseHexKey(hexID)
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
func (s *GameServerStore) CreateGameServer(ctx context.Context, server model.GameServer) error {
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO gameservers (hexid, server_id, host) VALUES (?, ?, ?)",
		model.HexKeyText(server.HexID),
		server.ID,
		server.Host,
	); err != nil {
		return fmt.Errorf("create game server %d: %w", server.ID, err)
	}
	return nil
}

// DeleteGameServer removes the registered game server row for id. Deleting
// an id with no row is not an error.
func (s *GameServerStore) DeleteGameServer(ctx context.Context, id int) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM gameservers WHERE server_id = ?", id); err != nil {
		return fmt.Errorf("delete game server %d: %w", id, err)
	}
	return nil
}

// DeleteAllGameServers removes every registered game server row.
func (s *GameServerStore) DeleteAllGameServers(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "TRUNCATE gameservers"); err != nil {
		return fmt.Errorf("delete all game servers: %w", err)
	}
	return nil
}

// SetGameServerHost updates a registered game server host.
func (s *GameServerStore) SetGameServerHost(ctx context.Context, id int, host string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE gameservers SET host = ? WHERE server_id = ?", host, id)
	if err != nil {
		return fmt.Errorf("set game server %d host: %w", id, err)
	}
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return ErrGameServerNotFound
	}
	return nil
}
