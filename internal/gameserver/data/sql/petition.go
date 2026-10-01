package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
)

// petitionFeedbackWidth is the petition.feedback column's width in
// characters. Nothing bounds the feedback a petitioner sends, so Save cuts
// it to fit rather than let one over-long feedback fail the whole save.
const petitionFeedbackWidth = 512

// PetitionStore reads and writes the petition and petition_message tables.
type PetitionStore struct {
	db *sql.DB
}

// NewPetitionStore returns a PetitionStore backed by db.
func NewPetitionStore(db *sql.DB) *PetitionStore {
	return &PetitionStore{db: db}
}

// Load returns every stored petition in id order with its chat lines. A
// row it cannot read stops the load: the petitions read before it are
// returned with the error, without their chat lines when the bad row is a
// petition.
func (s *PetitionStore) Load(ctx context.Context) ([]petition.Record, error) {
	records, err := s.loadPetitions(ctx)
	if err != nil {
		return records, err
	}
	return records, s.loadMessages(ctx, records)
}

func (s *PetitionStore) loadPetitions(ctx context.Context) ([]petition.Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT oid, type, petitioner_oid, submit_date, content, is_unread, state, rate, feedback, responders FROM petition ORDER BY oid ASC`)
	if err != nil {
		return nil, fmt.Errorf("load petitions: %w", err)
	}
	defer rows.Close()
	var out []petition.Record
	for rows.Next() {
		var (
			r                            petition.Record
			typ, state, rate, responders string
			unread                       int
		)
		if err := rows.Scan(&r.ID, &typ, &r.Petitioner, &r.SubmitDate, &r.Content, &unread, &state, &rate, &r.Feedback, &responders); err != nil {
			return out, fmt.Errorf("load petitions: %w", err)
		}
		var ok bool
		if r.Type, ok = petition.ParseType(typ); !ok {
			return out, fmt.Errorf("load petition %d: unknown type %q", r.ID, typ)
		}
		if r.State, ok = petition.ParseState(state); !ok {
			return out, fmt.Errorf("load petition %d: unknown state %q", r.ID, state)
		}
		if r.Rate, ok = petition.ParseRate(rate); !ok {
			return out, fmt.Errorf("load petition %d: unknown rate %q", r.ID, rate)
		}
		r.Unread = unread != 0
		if responders != "" {
			for _, field := range strings.Split(responders, ";") {
				id, err := strconv.ParseInt(field, 10, 32)
				if err != nil {
					return out, fmt.Errorf("load petition %d: responders %q: %w", r.ID, responders, err)
				}
				r.Responders = append(r.Responders, int32(id))
			}
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("load petitions: %w", err)
	}
	return out, nil
}

// loadMessages files each stored chat line on its petition in records, in
// the order the lines were said.
func (s *PetitionStore) loadMessages(ctx context.Context, records []petition.Record) error {
	index := make(map[int32]int, len(records))
	for i, r := range records {
		index[r.ID] = i
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT petition_oid, player_oid, type, player_name, content FROM petition_message ORDER BY id ASC, petition_oid ASC`)
	if err != nil {
		return fmt.Errorf("load petition messages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			petitionID int32
			msg        petition.Message
			channel    string
		)
		if err := rows.Scan(&petitionID, &msg.ObjectID, &channel, &msg.Name, &msg.Text); err != nil {
			return fmt.Errorf("load petition messages: %w", err)
		}
		i, ok := index[petitionID]
		if !ok {
			continue
		}
		if msg.Channel, ok = chat.TypeByName(channel); !ok {
			return fmt.Errorf("load petition %d message: unknown channel %q", petitionID, channel)
		}
		records[i].Messages = append(records[i].Messages, msg)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load petition messages: %w", err)
	}
	return nil
}

// Save replaces the stored petitions and their chat lines with records, in
// one transaction: a failed save leaves the previous rows in place.
func (s *PetitionStore) Save(ctx context.Context, records []petition.Record) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save petitions: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	for _, table := range []string{"petition", "petition_message"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("save petitions: clear %s: %w", table, err)
		}
	}
	petitions, err := tx.PrepareContext(ctx,
		`INSERT INTO petition (oid, type, petitioner_oid, submit_date, content, is_unread, state, rate, feedback, responders) VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("save petitions: %w", err)
	}
	defer petitions.Close()
	messages, err := tx.PrepareContext(ctx,
		`INSERT INTO petition_message (id, petition_oid, player_oid, type, player_name, content) VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("save petitions: %w", err)
	}
	defer messages.Close()
	for _, r := range records {
		responders := make([]string, len(r.Responders))
		for i, id := range r.Responders {
			responders[i] = strconv.Itoa(int(id))
		}
		unread := 0
		if r.Unread {
			unread = 1
		}
		if _, err := petitions.ExecContext(ctx, r.ID, r.Type.String(), r.Petitioner, r.SubmitDate, r.Content, unread,
			r.State.String(), r.Rate.String(), truncateRunes(r.Feedback, petitionFeedbackWidth), strings.Join(responders, ";")); err != nil {
			return fmt.Errorf("save petition %d: %w", r.ID, err)
		}
		for i, msg := range r.Messages {
			if _, err := messages.ExecContext(ctx, i, r.ID, msg.ObjectID, msg.Channel.String(), msg.Name, msg.Text); err != nil {
				return fmt.Errorf("save petition %d message %d: %w", r.ID, i, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save petitions: %w", err)
	}
	committed = true
	return nil
}

// truncateRunes returns s cut to at most n characters.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
