package dbtest

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func TestMain(m *testing.M) { os.Exit(Main(m)) }

var testPool = NewPool(PoolConfig{
	Schema: []string{
		"CREATE TABLE plain (id INT NOT NULL PRIMARY KEY)",
		"CREATE TABLE counted (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY, v INT NOT NULL)",
		"CREATE TABLE seeded (id INT NOT NULL PRIMARY KEY, v INT NOT NULL)",
	},
	Seed: []string{"INSERT INTO seeded VALUES (1, 10)"},
})

func dbName(t *testing.T, db *sql.DB) string {
	t.Helper()
	var name string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

func mustExec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func TestPoolResetsAndReusesDatabase(t *testing.T) {
	var first string
	t.Run("dirty", func(t *testing.T) {
		db := testPool.DB(t)
		if again := testPool.DB(t); again != db {
			t.Fatal("same test got a different database")
		}
		first = dbName(t, db)
		mustExec(t, db, "INSERT INTO plain VALUES (1)")
		mustExec(t, db, "INSERT INTO counted (v) VALUES (1), (2)")
		mustExec(t, db, "UPDATE seeded SET v = 99")
	})
	t.Run("clean", func(t *testing.T) {
		if first == "" {
			t.Skip("needs the dirty subtest to run first")
		}
		db := testPool.DB(t)
		if got := dbName(t, db); got != first {
			t.Fatalf("sequential test got database %s, want reused %s", got, first)
		}
		var plain int
		if err := db.QueryRow("SELECT COUNT(*) FROM plain").Scan(&plain); err != nil {
			t.Fatal(err)
		}
		if plain != 0 {
			t.Fatalf("plain has %d rows after reset, want 0", plain)
		}
		var seeded int
		if err := db.QueryRow("SELECT v FROM seeded WHERE id = 1").Scan(&seeded); err != nil {
			t.Fatal(err)
		}
		if seeded != 10 {
			t.Fatalf("seed row v = %d after reset, want 10", seeded)
		}
		mustExec(t, db, "INSERT INTO counted (v) VALUES (1)")
		var id int
		if err := db.QueryRow("SELECT id FROM counted").Scan(&id); err != nil {
			t.Fatal(err)
		}
		if id != 1 {
			t.Fatalf("AUTO_INCREMENT id = %d after reset, want 1", id)
		}
	})
}

func TestPoolGivesHeldDatabaseToNoOtherTest(t *testing.T) {
	outer := dbName(t, testPool.DB(t))
	t.Run("subtest", func(t *testing.T) {
		if inner := dbName(t, testPool.DB(t)); inner == outer {
			t.Fatalf("subtest got database %s still held by its parent", inner)
		}
	})
}
