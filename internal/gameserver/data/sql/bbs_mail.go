package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
)

// mailDateLayout is how bbs_mail.sent_date is written and read: the local
// wall time, to the second.
const mailDateLayout = "2006-01-02 15:04:05"

// MailStore reads and writes bbs_mail, the community board's mail.
type MailStore struct {
	db *sql.DB
}

// NewMailStore returns a MailStore backed by db.
func NewMailStore(db *sql.DB) *MailStore {
	return &MailStore{db: db}
}

// Load reads every stored mail. A row whose folder or send date cannot be
// read is skipped and reported in the returned count.
func (s *MailStore) Load(ctx context.Context) ([]bbs.Mail, int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, receiver_id, sender_id, location, COALESCE(recipients,''),
		COALESCE(subject,''), COALESCE(message,''), DATE_FORMAT(sent_date, '%Y-%m-%d %H:%i:%s'), COALESCE(is_unread,0)
		FROM bbs_mail ORDER BY id`)
	if err != nil {
		return nil, 0, fmt.Errorf("load mail: %w", err)
	}
	defer rows.Close()
	var (
		mails   []bbs.Mail
		skipped int
	)
	for rows.Next() {
		var (
			m        bbs.Mail
			location string
			sent     sql.NullString
			unread   int
		)
		if err := rows.Scan(&m.ID, &m.ReceiverID, &m.SenderID, &location, &m.Recipients, &m.Subject, &m.Message, &sent, &unread); err != nil {
			return nil, 0, fmt.Errorf("load mail: %w", err)
		}
		folder, ok := bbs.ParseFolder(location)
		if !ok || !sent.Valid {
			skipped++
			continue
		}
		at, err := time.ParseInLocation(mailDateLayout, sent.String, time.Local)
		if err != nil {
			skipped++
			continue
		}
		m.Folder, m.Sent, m.Unread = folder, at, unread != 0
		mails = append(mails, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("load mail: %w", err)
	}
	return mails, skipped, nil
}

// InsertMail stores a new mail.
func (s *MailStore) InsertMail(ctx context.Context, m bbs.Mail) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO bbs_mail (id,receiver_id,sender_id,location,recipients,subject,message,sent_date,is_unread)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		m.ID, m.ReceiverID, m.SenderID, m.Folder.Column(), m.Recipients, m.Subject, m.Message,
		m.Sent.Local().Format(mailDateLayout), boolInt(m.Unread))
	if err != nil {
		return fmt.Errorf("insert mail %d: %w", m.ID, err)
	}
	return nil
}

// DeleteMail removes a mail.
func (s *MailStore) DeleteMail(ctx context.Context, id int32) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM bbs_mail WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete mail %d: %w", id, err)
	}
	return nil
}

// MarkMailRead stores a mail as opened.
func (s *MailStore) MarkMailRead(ctx context.Context, id int32) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE bbs_mail SET is_unread=0 WHERE id=?`, id); err != nil {
		return fmt.Errorf("mark mail %d read: %w", id, err)
	}
	return nil
}

// MoveMail stores the folder a mail is filed in.
func (s *MailStore) MoveMail(ctx context.Context, id int32, f bbs.Folder) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE bbs_mail SET location=? WHERE id=?`, f.Column(), id); err != nil {
		return fmt.Errorf("move mail %d: %w", id, err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
