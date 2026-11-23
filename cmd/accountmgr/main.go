// Command accountmgr is an interactive tool for creating, updating, and
// deleting login server accounts, and for listing accounts by access level.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
	"github.com/fatal10110/acis_golang/internal/loginserver/network/clientpackets"
)

func main() {
	configPath := flag.String("config", "config/loginserver.properties", "login server properties file (Database URL/Login/Password keys)")
	flag.Parse()

	props, err := config.LoadFile(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	pool, err := db.Open(db.Config{
		URL:      props.String("URL", "jdbc:mariadb://localhost/acis"),
		Login:    props.String("Login", "root"),
		Password: props.String("Password", ""),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := run(context.Background(), os.Stdin, os.Stdout, sql.NewAccountStore(pool)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run drives the interactive command loop, reading whitespace-separated
// tokens from in.
func run(ctx context.Context, in io.Reader, out io.Writer, store *sql.AccountStore) error {
	sc := bufio.NewScanner(in)
	sc.Split(bufio.ScanWords)
	next := func() (string, bool) {
		if !sc.Scan() {
			return "", false
		}
		return sc.Text(), true
	}
	ask := func(prompt string) (string, bool) {
		fmt.Fprint(out, prompt)
		return next()
	}
	choose := func(prompt string, valid ...string) (string, bool) {
		for {
			v, ok := ask(prompt)
			if !ok {
				return "", false
			}
			for _, want := range valid {
				if v == want {
					return v, true
				}
			}
		}
	}

	for {
		fmt.Fprintln(out, "Please choose an option:")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "1 - Create new account or update existing one (change pass and access level)")
		fmt.Fprintln(out, "2 - Change access level")
		fmt.Fprintln(out, "3 - Delete existing account")
		fmt.Fprintln(out, "4 - List accounts and access levels")
		fmt.Fprintln(out, "5 - Exit")

		mode, ok := choose("Your choice: ", "1", "2", "3", "4", "5")
		if !ok {
			return sc.Err()
		}

		var login, password, levelText string
		if mode == "1" || mode == "2" || mode == "3" {
			v, ok := ask("Username: ")
			if !ok {
				return sc.Err()
			}
			login = storedLogin(ctx, store, strings.ToLower(v))

			if mode == "1" {
				if v, ok = ask("Password: "); !ok {
					return sc.Err()
				}
				password = v
			}
			if mode == "1" || mode == "2" {
				if v, ok = ask("Access level: "); !ok {
					return sc.Err()
				}
				levelText = v
			}
		}

		switch mode {
		// The access level goes to the database as typed; a value the
		// column cannot hold is reported as the database's error.
		case "1":
			// Credentials longer than a client can send would make an
			// account no one can log into (#5048).
			if len(login) > clientpackets.MaxUsernameBytes || len(password) > clientpackets.MaxPasswordBytes {
				fmt.Fprintf(out, "The username can be at most %d bytes and the password at most %d bytes; the account was not created or updated.\n",
					clientpackets.MaxUsernameBytes, clientpackets.MaxPasswordBytes)
			} else {
				addOrUpdateAccount(ctx, out, store, login, password, levelText)
			}

		case "2":
			changeAccountLevel(ctx, out, store, login, levelText)

		case "3":
			fmt.Fprintln(out, "WARNING: This will not delete the gameserver data (characters, items, etc..) it will only delete the account login server data.")
			v, ok := ask("Do you really want to delete this account? Y/N: ")
			if !ok {
				return sc.Err()
			}
			if strings.EqualFold(v, "y") {
				deleteAccount(ctx, out, store, login)
			} else {
				fmt.Fprintln(out, "Deletion cancelled.")
			}

		case "4":
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Please choose a listing mode:")
			fmt.Fprintln(out)
			fmt.Fprintln(out, "1 - Banned accounts only (accessLevel < 0)")
			fmt.Fprintln(out, "2 - GM/privileged accounts (accessLevel > 0")
			fmt.Fprintln(out, "3 - Regular accounts only (accessLevel = 0)")
			fmt.Fprintln(out, "4 - List all")

			listMode, ok := choose("Your choice: ", "1", "2", "3", "4")
			if !ok {
				return sc.Err()
			}
			fmt.Fprintln(out)
			printAccountInfo(ctx, out, store, listMode)

		case "5":
			return nil
		}

		fmt.Fprintln(out)
	}
}

// storedLogin returns the login of the account row the database matches to
// login, so every option acts on and reports the stored name even when the
// table's collation equates a differently spelled one (#5046). With no
// matching row, or when the lookup fails, login is returned unchanged.
func storedLogin(ctx context.Context, store *sql.AccountStore, login string) string {
	if acc, err := store.Account(ctx, login); err == nil {
		return acc.Login
	}
	return login
}

func listFilter(mode string) sql.AccountFilter {
	switch mode {
	case "1":
		return sql.BannedAccounts
	case "2":
		return sql.PrivilegedAccounts
	case "3":
		return sql.RegularAccounts
	default:
		return sql.AllAccounts
	}
}

func printAccountInfo(ctx context.Context, out io.Writer, store *sql.AccountStore, mode string) {
	accounts, err := store.ListAccounts(ctx, listFilter(mode))
	if err != nil {
		fmt.Fprintln(out, "There was error while displaying accounts:")
		fmt.Fprintln(out, err)
		return
	}
	for _, a := range accounts {
		fmt.Fprintf(out, "%s -> %d\n", a.Login, a.AccessLevel)
	}
	fmt.Fprintf(out, "Displayed accounts: %d\n", len(accounts))
}

func addOrUpdateAccount(ctx context.Context, out io.Writer, store *sql.AccountStore, login, password, level string) {
	hashed, err := model.HashPassword(password)
	if err != nil {
		fmt.Fprintln(out, "There was error while adding/updating account:")
		fmt.Fprintln(out, err)
		return
	}
	changed, err := store.UpsertAccount(ctx, login, hashed, level)
	if err != nil {
		fmt.Fprintln(out, "There was error while adding/updating account:")
		fmt.Fprintln(out, err)
		return
	}
	if changed {
		fmt.Fprintf(out, "Account %s has been created or updated\n", login)
	} else {
		fmt.Fprintf(out, "Account %s doesn't exist\n", login)
	}
}

func changeAccountLevel(ctx context.Context, out io.Writer, store *sql.AccountStore, login, level string) {
	changed, err := store.ChangeAccessLevel(ctx, login, level)
	if err != nil {
		fmt.Fprintln(out, "There was error while updating account:")
		fmt.Fprintln(out, err)
		return
	}
	if changed {
		fmt.Fprintf(out, "Account %s has been updated\n", login)
	} else {
		fmt.Fprintf(out, "Account %s doesn't exist\n", login)
	}
}

func deleteAccount(ctx context.Context, out io.Writer, store *sql.AccountStore, login string) {
	deleted, err := store.DeleteAccount(ctx, login)
	if err != nil {
		fmt.Fprintln(out, "There was error while deleting account:")
		fmt.Fprintln(out, err)
		return
	}
	if deleted {
		fmt.Fprintf(out, "Account %s has been deleted\n", login)
	} else {
		fmt.Fprintf(out, "Account %s doesn't exist\n", login)
	}
}
