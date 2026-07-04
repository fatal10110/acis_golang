package sql

import (
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func newMockStore(t *testing.T) (*AccountStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewAccountStore(db), mock
}

func TestAccountStore_Account_Found(t *testing.T) {
	store, mock := newMockStore(t)

	rows := sqlmock.NewRows([]string{"password", "access_level", "last_server"}).
		AddRow("hash123", 1, 2)
	mock.ExpectQuery("SELECT password, access_level, last_server FROM accounts WHERE login = ?").
		WithArgs("player1").
		WillReturnRows(rows)

	got, err := store.Account("player1")
	if err != nil {
		t.Fatalf("Account() unexpected error: %v", err)
	}
	want := struct {
		login       string
		password    string
		accessLevel int
		lastServer  int
	}{"player1", "hash123", 1, 2}

	if got.Login != want.login || got.Password != want.password || got.AccessLevel != want.accessLevel || got.LastServer != want.lastServer {
		t.Errorf("Account() = %+v, want %+v", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAccountStore_Account_NotFound(t *testing.T) {
	store, mock := newMockStore(t)

	mock.ExpectQuery("SELECT password, access_level, last_server FROM accounts WHERE login = ?").
		WithArgs("ghost").
		WillReturnRows(sqlmock.NewRows([]string{"password", "access_level", "last_server"}))

	_, err := store.Account("ghost")
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("Account() error = %v, want ErrAccountNotFound", err)
	}
}

func TestAccountStore_CreateAccount(t *testing.T) {
	store, mock := newMockStore(t)

	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mock.ExpectExec("INSERT INTO accounts \\(login, password, last_active\\) VALUES \\(\\?, \\?, \\?\\)").
		WithArgs("newplayer", "hashedpw", createdAt.UnixMilli()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := store.CreateAccount("newplayer", "hashedpw", createdAt)
	if err != nil {
		t.Fatalf("CreateAccount() unexpected error: %v", err)
	}
	if got.Login != "newplayer" || got.Password != "hashedpw" || got.AccessLevel != 0 || got.LastServer != 1 {
		t.Errorf("CreateAccount() = %+v, want login=newplayer password=hashedpw accessLevel=0 lastServer=1", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAccountStore_SetLastActive(t *testing.T) {
	store, mock := newMockStore(t)

	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mock.ExpectExec("UPDATE accounts SET last_active = \\? WHERE login = \\?").
		WithArgs(at.UnixMilli(), "player1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetLastActive("player1", at); err != nil {
		t.Fatalf("SetLastActive() unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAccountStore_SetAccessLevel(t *testing.T) {
	store, mock := newMockStore(t)

	mock.ExpectExec("UPDATE accounts SET access_level = \\? WHERE login = \\?").
		WithArgs(-1, "banneduser").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetAccessLevel("banneduser", -1); err != nil {
		t.Fatalf("SetAccessLevel() unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestAccountStore_SetLastServer(t *testing.T) {
	store, mock := newMockStore(t)

	mock.ExpectExec("UPDATE accounts SET last_server = \\? WHERE login = \\?").
		WithArgs(3, "player1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetLastServer("player1", 3); err != nil {
		t.Fatalf("SetLastServer() unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
