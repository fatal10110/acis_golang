// Command accountmgr is an interactive tool for creating, updating, and
// deleting login server accounts, and for listing accounts by access level.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
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

	if err := run(os.Stdin, os.Stdout, sql.NewAccountStore(pool)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run drives the interactive command loop, reading whitespace-separated
// tokens from in.
func run(in io.Reader, out io.Writer, store *sql.AccountStore) error {
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
		fmt.Fprintln(out)

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
			login = strings.ToLower(v)

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
		case "1":
			level, err := strconv.Atoi(levelText)
			if err != nil {
				fmt.Fprintln(out, "Type a valid access level.")
				break
			}
			addOrUpdateAccount(out, store, login, password, level)

		case "2":
			level, err := strconv.Atoi(levelText)
			if err != nil {
				fmt.Fprintln(out, "Type a valid access level.")
				break
			}
			changeAccountLevel(out, store, login, level)

		case "3":
			fmt.Fprintln(out, "WARNING: This will not delete the gameserver data (characters, items, etc..) it will only delete the account login server data.")
			v, ok := ask("Do you really want to delete this account? Y/N: ")
			if !ok {
				return sc.Err()
			}
			if strings.EqualFold(v, "y") {
				deleteAccount(out, store, login)
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
			printAccountInfo(out, store, listMode)

		case "5":
			return nil
		}

		fmt.Fprintln(out)
	}
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

func printAccountInfo(out io.Writer, store *sql.AccountStore, mode string) {
	accounts, err := store.ListAccounts(listFilter(mode))
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

func addOrUpdateAccount(out io.Writer, store *sql.AccountStore, login, password string, level int) {
	hashed, err := model.HashPassword(password)
	if err != nil {
		fmt.Fprintln(out, "There was error while adding/updating account:")
		fmt.Fprintln(out, err)
		return
	}
	changed, err := store.UpsertAccount(login, hashed, level)
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

func changeAccountLevel(out io.Writer, store *sql.AccountStore, login string, level int) {
	changed, err := store.ChangeAccessLevel(login, level)
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

func deleteAccount(out io.Writer, store *sql.AccountStore, login string) {
	deleted, err := store.DeleteAccount(login)
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
