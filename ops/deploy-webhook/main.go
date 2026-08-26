package main

import (
	"crypto/hmac"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

func main() {
	token := os.Getenv("DEPLOY_TOKEN")
	if token == "" {
		log.Fatal("DEPLOY_TOKEN env var required")
	}
	srcDir := os.Getenv("SRC_DIR")
	if srcDir == "" {
		srcDir = "/opt/acis/src"
	}

	http.HandleFunc("/deploy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		auth := r.Header.Get("Authorization")
		want := "Bearer " + token
		if len(auth) != len(want) || !hmac.Equal([]byte(auth), []byte(want)) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		log.Println("deploy triggered")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintln(w, "deploy started")

		go runDeploy(srcDir)
	})

	log.Println("listening on 127.0.0.1:8080")
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", nil))
}

func runDeploy(srcDir string) {
	steps := [][]string{
		{"git", "-C", srcDir, "fetch", "origin", "main"},
		{"git", "-C", srcDir, "reset", "--hard", "origin/main"},
		{"go", "build", "-C", srcDir, "-o", "/opt/acis/gameserver.new", "./cmd/gameserver"},
		{"go", "build", "-C", srcDir, "-o", "/opt/acis/loginserver.new", "./cmd/loginserver"},
		{"mv", "/opt/acis/gameserver.new", "/opt/acis/gameserver"},
		{"mv", "/opt/acis/loginserver.new", "/opt/acis/loginserver"},
		{"sudo", "systemctl", "restart", "acis-loginserver"},
		{"sudo", "systemctl", "restart", "acis-gameserver"},
	}
	for _, s := range steps {
		cmd := exec.Command(s[0], s[1:]...)
		cmd.Env = append(os.Environ(), "HOME=/home/deploy")
		out, err := cmd.CombinedOutput()
		log.Printf("+ %v\n%s", s, out)
		if err != nil {
			log.Printf("deploy step failed: %v", err)
			return
		}
	}
	log.Printf("deploy finished at %s", time.Now().Format(time.RFC3339))
}
