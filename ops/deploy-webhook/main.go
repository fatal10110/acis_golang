package main

import (
	"crypto/hmac"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
)

// maxUpload caps one deploy request; both binaries together are ~60 MB.
const maxUpload = 512 << 20

// binaries are the multipart field names a deploy must carry, each saved under
// the same name in the binary directory.
var binaries = []string{"gameserver", "loginserver"}

// services are restarted in this order once the new binaries are in place.
var services = []string{"acis-loginserver", "acis-gameserver"}

func main() {
	token := os.Getenv("DEPLOY_TOKEN")
	if token == "" {
		log.Fatal("DEPLOY_TOKEN env var required")
	}
	binDir := os.Getenv("BIN_DIR")
	if binDir == "" {
		binDir = "/opt/acis"
	}

	http.Handle("/deploy", newHandler(token, binDir, restartService))

	log.Println("listening on 127.0.0.1:8080")
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", nil))
}

func restartService(unit string) error {
	out, err := exec.Command("sudo", "systemctl", "restart", unit).CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart %s: %w: %s", unit, err, out)
	}
	return nil
}

// newHandler accepts a multipart POST carrying the CI-built binaries, swaps
// them in, restarts the services, and answers only once that is done so the
// caller sees the real outcome. One deploy runs at a time.
func newHandler(token, binDir string, restart func(unit string) error) http.Handler {
	var mu sync.Mutex
	want := []byte("Bearer " + token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		auth := []byte(r.Header.Get("Authorization"))
		if len(auth) != len(want) || !hmac.Equal(auth, want) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !mu.TryLock() {
			http.Error(w, "deploy already running", http.StatusConflict)
			return
		}
		defer mu.Unlock()

		commit := r.Header.Get("X-Deploy-Commit")
		log.Printf("deploy %s: receiving binaries", commit)
		r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
		if err := receive(r, binDir); err != nil {
			log.Printf("deploy %s: upload rejected: %v", commit, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, unit := range services {
			if err := restart(unit); err != nil {
				log.Printf("deploy %s: %v", commit, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		log.Printf("deploy %s: finished", commit)
		fmt.Fprintln(w, "deployed")
	})
}

// receive stores every expected binary as <name>.new and renames them all into
// place only after the whole upload arrived, so a partial or malformed request
// never touches the running binaries.
func receive(r *http.Request, binDir string) error {
	mr, err := r.MultipartReader()
	if err != nil {
		return err
	}

	staged := map[string]string{}
	defer func() {
		for _, tmp := range staged {
			os.Remove(tmp)
		}
	}()

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := part.FormName()
		if !slices.Contains(binaries, name) {
			return fmt.Errorf("unexpected field %q", name)
		}
		if _, dup := staged[name]; dup {
			return fmt.Errorf("duplicate field %q", name)
		}
		tmp := filepath.Join(binDir, name+".new")
		staged[name] = tmp
		if err := save(tmp, part); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	for _, name := range binaries {
		if _, ok := staged[name]; !ok {
			return fmt.Errorf("missing field %q", name)
		}
	}
	for _, name := range binaries {
		if err := os.Rename(staged[name], filepath.Join(binDir, name)); err != nil {
			return err
		}
		delete(staged, name)
	}
	return nil
}

func save(path string, src io.Reader) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(src, head); err != nil || string(head) != "\x7fELF" {
		f.Close()
		return errors.New("not an ELF binary")
	}
	if _, err := f.Write(head); err != nil {
		f.Close()
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
