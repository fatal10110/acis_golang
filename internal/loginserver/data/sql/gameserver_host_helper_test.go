package sql

import (
	"context"
	"fmt"
)

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
