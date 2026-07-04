// Command gsregister manages game server registrations in the login
// database. It assigns a server id from the known name list, generates the
// server's auth key, persists the registration, and writes the hexid file
// the game server presents when linking to the login server.
package main

import (
	"bufio"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

// authKeyLen is the size in bytes of a generated game server auth key.
const authKeyLen = 16

const noNamesMsg = "No server names available, be sure 'serverNames.xml' is in the LoginServer directory."

func main() {
	configPath := flag.String("config", "config/loginserver.properties", "login server properties file (Database URL/Login/Password keys)")
	namesPath := flag.String("names", "serverNames.xml", "server id/name list file")
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

	names, err := manager.LoadServerNames(*namesPath)
	if err != nil {
		// Keep running with an empty list: list/cleanall still work, and
		// register/clean report the missing name list themselves.
		fmt.Fprintln(os.Stderr, err)
		names = &manager.ServerNames{}
	}

	if err := run(os.Stdin, os.Stdout, names, sql.NewGameServerStore(pool), "."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run drives the interactive command loop, reading whitespace-separated
// tokens from in. Registered servers are loaded from the store once at
// start and kept in step with the store as commands mutate it. Hexid files
// are written into dir.
func run(in io.Reader, out io.Writer, names *manager.ServerNames, store *sql.GameServerStore, dir string) error {
	registered, err := store.GameServers()
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "OPTIONS : a number : register a server ID, if available and existing on list.")
	fmt.Fprintln(out, "          list : get a list of IDs. A '*' means the id is already used.")
	fmt.Fprintln(out, "          clean : unregister a specified gameserver.")
	fmt.Fprintln(out, "          cleanall : unregister all gameservers.")
	fmt.Fprintln(out, "          exit : exit the program.")

	sc := bufio.NewScanner(in)
	sc.Split(bufio.ScanWords)
	next := func() (string, bool) {
		fmt.Fprintln(out)
		fmt.Fprint(out, "Your choice? ")
		if !sc.Scan() {
			return "", false
		}
		return sc.Text(), true
	}

	for {
		choice, ok := next()
		if !ok {
			return sc.Err()
		}

		switch strings.ToLower(choice) {
		case "list":
			fmt.Fprintln(out)
			for _, id := range names.IDs() {
				name, _ := names.Name(id)
				used := ""
				if _, ok := registered[id]; ok {
					used = "*"
				}
				fmt.Fprintf(out, "%d: %s %s\n", id, name, used)
			}

		case "clean":
			fmt.Fprintln(out)
			if len(names.IDs()) == 0 {
				fmt.Fprintln(out, noNamesMsg)
				continue
			}
			fmt.Fprintln(out, "UNREGISTER a specific server. Here's the current list :")
			for _, id := range sortedIDs(registered) {
				name, _ := names.Name(id)
				fmt.Fprintf(out, "%d: %s\n", id, name)
			}

			choice, ok := next()
			if !ok {
				return sc.Err()
			}
			id, err := strconv.Atoi(choice)
			if err != nil {
				fmt.Fprintln(out, "Type a valid server id.")
				continue
			}
			if _, ok := registered[id]; !ok {
				fmt.Fprintln(out, "This server id isn't used.")
				continue
			}
			if err := store.DeleteGameServer(id); err != nil {
				fmt.Fprintln(out, "SQL error while cleaning registered server:", err)
			}
			delete(registered, id)
			fmt.Fprintf(out, "You successfully dropped gameserver #%d.\n", id)

		case "cleanall":
			fmt.Fprintln(out)
			fmt.Fprint(out, "UNREGISTER ALL servers. Are you sure? (y/n) ")
			if !sc.Scan() {
				return sc.Err()
			}
			if sc.Text() != "y" {
				fmt.Fprintln(out, "'cleanall' processus has been aborted.")
				continue
			}
			if err := store.DeleteAllGameServers(); err != nil {
				fmt.Fprintln(out, "SQL error while cleaning registered servers:", err)
			}
			clear(registered)
			fmt.Fprintln(out, "You successfully dropped all registered gameservers.")

		case "exit":
			return nil

		default:
			fmt.Fprintln(out)
			if len(names.IDs()) == 0 {
				fmt.Fprintln(out, noNamesMsg)
				continue
			}
			id, err := strconv.Atoi(choice)
			if err != nil {
				fmt.Fprintln(out, "Type a number or list|clean|cleanall commands.")
				continue
			}
			if _, ok := names.Name(id); !ok {
				fmt.Fprintf(out, "No name for server id: %d.\n", id)
				continue
			}
			if _, ok := registered[id]; ok {
				fmt.Fprintln(out, "This server id is already used.")
				continue
			}

			key := make([]byte, authKeyLen)
			if _, err := rand.Read(key); err != nil {
				return fmt.Errorf("generate auth key: %w", err)
			}
			server := model.NewGameServer(id, key, "")
			registered[id] = server
			if err := store.CreateGameServer(server); err != nil {
				fmt.Fprintln(out, "SQL error while saving gameserver data:", err)
			}

			filename := fmt.Sprintf("hexid(server %d).txt", id)
			if err := writeHexID(filepath.Join(dir, filename), id, model.HexKeyText(key)); err != nil {
				fmt.Fprintf(out, "Failed to save hex ID to '%s' file: %v\n", filename, err)
				continue
			}
			fmt.Fprintf(out, "Server registered under '%s'.\n", filename)
			fmt.Fprintln(out, "Put this file in /config gameserver folder and rename it 'hexid.txt'.")
		}
	}
}

// writeHexID writes the server id and signed-hex auth key as a properties
// file the game server loads at boot.
func writeHexID(path string, id int, hexText string) error {
	content := fmt.Sprintf("#the hexID to auth into login\n#%s\nServerID=%d\nHexID=%s\n",
		time.Now().Format("Mon Jan 02 15:04:05 MST 2006"), id, hexText)
	return os.WriteFile(path, []byte(content), 0o644)
}

// sortedIDs returns the registered server ids in ascending order.
func sortedIDs(registered map[int]model.GameServer) []int {
	ids := make([]int, 0, len(registered))
	for id := range registered {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}
