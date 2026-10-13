package db

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// StoredText returns s as it is written to the database. Text read from a
// client may hold an unpaired UTF-16 surrogate in its three-byte form (ED
// A0..BF 80..BF), which no utf8mb4 column takes. Such a unit is stored as
// '?', except a leading surrogate that ends the string, which is stored as
// 'c'. A leading surrogate directly followed by a trailing one is stored as
// the supplementary character the two spell. Everything else is unchanged.
func StoredText(s string) string {
	if !strings.Contains(s, "\xED") {
		return s
	}
	var b []byte
	for i := 0; i < len(s); {
		unit, ok := encodedSurrogate(s[i:])
		if !ok {
			if b != nil {
				b = append(b, s[i])
			}
			i++
			continue
		}
		if b == nil {
			b = append(make([]byte, 0, len(s)), s[:i]...)
		}
		if unit < 0xDC00 {
			if low, ok := encodedSurrogate(s[i+3:]); ok && low >= 0xDC00 {
				b = utf8.AppendRune(b, utf16.DecodeRune(unit, low))
				i += 6
				continue
			}
			if i+3 == len(s) {
				b = append(b, 'c')
				i += 3
				continue
			}
		}
		b = append(b, '?')
		i += 3
	}
	if b == nil {
		return s
	}
	return string(b)
}

// encodedSurrogate reports the surrogate unit whose three-byte form begins
// s.
func encodedSurrogate(s string) (rune, bool) {
	if len(s) < 3 || s[0] != 0xED || s[1]&0xE0 != 0xA0 || s[2]&0xC0 != 0x80 {
		return 0, false
	}
	return 0xD000 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), true
}

// textConnector hands out connections that pass every string argument
// through StoredText, so no query parameter can carry a unit the column
// rejects.
type textConnector struct {
	driver.Connector
}

// driverConn is the set of connection features the pool uses and the
// MariaDB driver provides.
type driverConn interface {
	driver.Conn
	driver.ConnPrepareContext
	driver.ConnBeginTx
	driver.ExecerContext
	driver.QueryerContext
	driver.Pinger
	driver.SessionResetter
	driver.Validator
	driver.NamedValueChecker
}

// driverStmt is the set of prepared-statement features the pool uses and
// the MariaDB driver provides.
type driverStmt interface {
	driver.Stmt
	driver.StmtExecContext
	driver.StmtQueryContext
	driver.NamedValueChecker
}

func (c textConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	dc, ok := conn.(driverConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("database driver connection %T lacks a required feature", conn)
	}
	return textConn{dc}, nil
}

type textConn struct {
	driverConn
}

func (c textConn) CheckNamedValue(nv *driver.NamedValue) error {
	return storeArg(c.driverConn, nv)
}

func (c textConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c textConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	stmt, err := c.driverConn.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	ds, ok := stmt.(driverStmt)
	if !ok {
		_ = stmt.Close()
		return nil, fmt.Errorf("database driver statement %T lacks a required feature", stmt)
	}
	return textStmt{ds}, nil
}

type textStmt struct {
	driverStmt
}

func (s textStmt) CheckNamedValue(nv *driver.NamedValue) error {
	return storeArg(s.driverStmt, nv)
}

// storeArg converts nv as the driver does, then rewrites a string result
// with StoredText.
func storeArg(checker driver.NamedValueChecker, nv *driver.NamedValue) error {
	if err := checker.CheckNamedValue(nv); err != nil {
		return err
	}
	if s, ok := nv.Value.(string); ok {
		nv.Value = StoredText(s)
	}
	return nil
}
