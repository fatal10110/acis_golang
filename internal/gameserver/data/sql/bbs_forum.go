package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
)

// ForumStore reads and writes bbs_forum, bbs_topic and bbs_post, the
// community board's forums.
type ForumStore struct {
	db *sql.DB
}

// NewForumStore returns a ForumStore backed by db.
func NewForumStore(db *sql.DB) *ForumStore {
	return &ForumStore{db: db}
}

// Load reads every stored forum, every topic in descending id order and
// every post in ascending id order.
func (s *ForumStore) Load(ctx context.Context) ([]bbs.ForumRow, []bbs.Topic, []bbs.Post, error) {
	forums, err := queryRows(ctx, s.db, `SELECT id, type, access, owner_id FROM bbs_forum`, func(rows *sql.Rows) (bbs.ForumRow, error) {
		var r bbs.ForumRow
		err := rows.Scan(&r.ID, &r.Type, &r.Access, &r.OwnerID)
		return r, err
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load forums: %w", err)
	}
	topics, err := queryRows(ctx, s.db, `SELECT id, forum_id, name, date, owner_name, owner_id FROM bbs_topic ORDER BY id DESC`, func(rows *sql.Rows) (bbs.Topic, error) {
		var t bbs.Topic
		err := rows.Scan(&t.ID, &t.ForumID, &t.Name, &t.Date, &t.OwnerName, &t.OwnerID)
		return t, err
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load topics: %w", err)
	}
	posts, err := queryRows(ctx, s.db, `SELECT id, owner_name, owner_id, date, topic_id, forum_id, txt FROM bbs_post ORDER BY id ASC`, func(rows *sql.Rows) (bbs.Post, error) {
		var p bbs.Post
		err := rows.Scan(&p.ID, &p.OwnerName, &p.OwnerID, &p.Date, &p.TopicID, &p.ForumID, &p.Text)
		return p, err
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load posts: %w", err)
	}
	return forums, topics, posts, nil
}

// queryRows runs query and reads each row with scan.
func queryRows[T any](ctx context.Context, db *sql.DB, query string, scan func(*sql.Rows) (T, error)) ([]T, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// InsertForum stores a new forum.
func (s *ForumStore) InsertForum(ctx context.Context, f bbs.Forum) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO bbs_forum (id,type,access,owner_id) VALUES (?,?,?,?)`,
		f.ID, f.Type.String(), f.Access.String(), f.OwnerID); err != nil {
		return fmt.Errorf("insert forum %d: %w", f.ID, err)
	}
	return nil
}

// InsertTopic stores a new topic, then its first post; a topic that
// cannot be stored stores no post.
func (s *ForumStore) InsertTopic(ctx context.Context, t bbs.Topic, first bbs.Post) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO bbs_topic (id,forum_id,name,date,owner_name,owner_id) VALUES (?,?,?,?,?,?)`,
		t.ID, t.ForumID, t.Name, t.Date, t.OwnerName, t.OwnerID); err != nil {
		return fmt.Errorf("insert topic %d of forum %d: %w", t.ID, t.ForumID, err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO bbs_post (id,owner_name,owner_id,date,topic_id,forum_id,txt) VALUES (?,?,?,?,?,?,?)`,
		first.ID, first.OwnerName, first.OwnerID, first.Date, first.TopicID, first.ForumID, first.Text); err != nil {
		return fmt.Errorf("insert post of topic %d of forum %d: %w", t.ID, t.ForumID, err)
	}
	return nil
}

// DeleteTopic removes a topic, then every post of it.
func (s *ForumStore) DeleteTopic(ctx context.Context, forumID, topicID int32) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM bbs_topic WHERE id=? AND forum_id=?`, topicID, forumID); err != nil {
		return fmt.Errorf("delete topic %d of forum %d: %w", topicID, forumID, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM bbs_post WHERE forum_id=? AND topic_id=?`, forumID, topicID); err != nil {
		return fmt.Errorf("delete posts of topic %d of forum %d: %w", topicID, forumID, err)
	}
	return nil
}

// UpdatePostText stores a post's text.
func (s *ForumStore) UpdatePostText(ctx context.Context, p bbs.Post) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE bbs_post SET txt=? WHERE id=? AND topic_id=? AND forum_id=?`,
		p.Text, p.ID, p.TopicID, p.ForumID); err != nil {
		return fmt.Errorf("update post %d of topic %d of forum %d: %w", p.ID, p.TopicID, p.ForumID, err)
	}
	return nil
}

// favoriteDateLayout is how bbs_favorite.date is written and read: the
// local wall time, to the second.
const favoriteDateLayout = "2006-01-02 15:04:05"

// FavoriteStore reads and writes bbs_favorite, the favorites board.
type FavoriteStore struct {
	db *sql.DB
}

// NewFavoriteStore returns a FavoriteStore backed by db.
func NewFavoriteStore(db *sql.DB) *FavoriteStore {
	return &FavoriteStore{db: db}
}

// Load reads every stored favorite in ascending id order. A row missing
// its title, bypass or date, or whose date does not read, is marked
// unreadable.
func (s *FavoriteStore) Load(ctx context.Context) ([]bbs.Favorite, error) {
	favs, err := queryRows(ctx, s.db, `SELECT id, player_id, title, bypass, DATE_FORMAT(date, '%Y-%m-%d %H:%i:%s') FROM bbs_favorite ORDER BY id ASC`,
		func(rows *sql.Rows) (bbs.Favorite, error) {
			var (
				f                   bbs.Favorite
				title, bypass, date sql.NullString
			)
			if err := rows.Scan(&f.ID, &f.PlayerID, &title, &bypass, &date); err != nil {
				return f, err
			}
			f.Title, f.Bypass = title.String, bypass.String
			at, err := time.ParseInLocation(favoriteDateLayout, date.String, time.Local)
			f.Date = at
			f.Unreadable = !title.Valid || !bypass.Valid || !date.Valid || err != nil
			return f, nil
		})
	if err != nil {
		return nil, fmt.Errorf("load favorites: %w", err)
	}
	return favs, nil
}

// InsertFavorite stores a new favorite.
func (s *FavoriteStore) InsertFavorite(ctx context.Context, f bbs.Favorite) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO bbs_favorite (id,player_id,title,bypass,date) VALUES (?,?,?,?,?)`,
		f.ID, f.PlayerID, f.Title, f.Bypass, f.Date.Local().Format(favoriteDateLayout)); err != nil {
		return fmt.Errorf("insert favorite %d: %w", f.ID, err)
	}
	return nil
}

// DeleteFavorite removes a favorite.
func (s *FavoriteStore) DeleteFavorite(ctx context.Context, id int32) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM bbs_favorite WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete favorite %d: %w", id, err)
	}
	return nil
}
