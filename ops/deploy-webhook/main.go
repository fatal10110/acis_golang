package main

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// maxBinary caps each downloaded binary; both together are ~60 MB.
	maxBinary = 512 << 20
	// sumsAsset lists the sha256 of every binary in the release.
	sumsAsset = "SHA256SUMS"
)

// tagPattern keeps the tag safe to place in a URL path.
var tagPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// binaries are the release assets a deploy installs, each saved under the
// same name in the binary directory.
var binaries = []string{"gameserver", "loginserver"}

// services are restarted in this order once the new binaries are in place.
var services = []string{"acis-loginserver", "acis-gameserver"}

type config struct {
	token   string // bearer token the caller must present
	base    string // GitHub web URL; release assets are public downloads
	repo    string // owner/name the release belongs to
	binDir  string
	client  *http.Client
	restart func(unit string) error
}

func main() {
	cfg := config{
		token:   os.Getenv("DEPLOY_TOKEN"),
		base:    "https://github.com",
		repo:    envOr("GITHUB_REPO", "fatal10110/acis_golang"),
		binDir:  envOr("BIN_DIR", "/opt/acis"),
		client:  &http.Client{Timeout: 5 * time.Minute},
		restart: restartService,
	}
	if cfg.token == "" {
		log.Fatal("DEPLOY_TOKEN env var required")
	}

	http.Handle("/deploy", newHandler(cfg))

	log.Println("listening on 127.0.0.1:8080")
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", nil))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func restartService(unit string) error {
	out, err := exec.Command("sudo", "systemctl", "restart", unit).CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart %s: %w: %s", unit, err, out)
	}
	return nil
}

// newHandler takes a POST naming a release tag, pulls that release's binaries,
// swaps them in, restarts the services, and answers only once that is done so
// the caller sees the real outcome. Any existing release can be named, which
// is also how a rollback is done. One deploy runs at a time.
func newHandler(cfg config) http.Handler {
	var mu sync.Mutex
	want := []byte("Bearer " + cfg.token)

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
		tag := r.Header.Get("X-Deploy-Tag")
		if !tagPattern.MatchString(tag) {
			http.Error(w, "X-Deploy-Tag missing or malformed", http.StatusBadRequest)
			return
		}
		if !mu.TryLock() {
			http.Error(w, "deploy already running", http.StatusConflict)
			return
		}
		defer mu.Unlock()

		if err := pull(cfg, tag); err != nil {
			log.Printf("deploy %s: %v", tag, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		log.Printf("deploy %s: binaries installed", tag)
		for _, unit := range services {
			if err := cfg.restart(unit); err != nil {
				log.Printf("deploy %s: %v", tag, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		log.Printf("deploy %s: finished", tag)
		fmt.Fprintf(w, "deployed %s\n", tag)
	})
}

// pull downloads every binary of the release as <name>.new, checks it against
// the release's SHA256SUMS, and renames them all into place only after each
// one verified, so a bad release never touches the running binaries.
func pull(cfg config, tag string) error {
	assetURL := func(name string) string {
		return fmt.Sprintf("%s/%s/releases/download/%s/%s", cfg.base, cfg.repo, tag, name)
	}

	sums, err := fetchSums(cfg.client, assetURL(sumsAsset))
	if err != nil {
		return err
	}

	staged := map[string]string{}
	defer func() {
		for _, tmp := range staged {
			os.Remove(tmp)
		}
	}()
	for _, name := range binaries {
		want, ok := sums[name]
		if !ok {
			return fmt.Errorf("%s has no entry for %s", sumsAsset, name)
		}
		tmp := filepath.Join(cfg.binDir, name+".new")
		staged[name] = tmp
		if err := download(cfg.client, assetURL(name), tmp, want); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	for _, name := range binaries {
		if err := os.Rename(staged[name], filepath.Join(cfg.binDir, name)); err != nil {
			return err
		}
		delete(staged, name)
	}
	return nil
}

func get(client *http.Client, url string) (io.ReadCloser, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

// fetchSums parses sha256sum output: "<hex>  <name>" per line.
func fetchSums(client *http.Client, url string) (map[string]string, error) {
	body, err := get(client, url)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	sums := map[string]string{}
	sc := bufio.NewScanner(io.LimitReader(body, 64<<10))
	for sc.Scan() {
		sum, name, ok := strings.Cut(sc.Text(), "  ")
		if ok {
			sums[name] = sum
		}
	}
	return sums, sc.Err()
}

// download saves url to path with mode 0755 and fails unless the content is an
// ELF binary whose sha256 matches wantSum.
func download(client *http.Client, url, path, wantSum string) error {
	body, err := get(client, url)
	if err != nil {
		return err
	}
	defer body.Close()
	src := io.LimitReader(body, maxBinary)

	head := make([]byte, 4)
	if _, err := io.ReadFull(src, head); err != nil || string(head) != "\x7fELF" {
		return errors.New("not an ELF binary")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	h.Write(head)
	_, err = f.Write(head)
	if err == nil {
		_, err = io.Copy(io.MultiWriter(f, h), src)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSum {
		return fmt.Errorf("sha256 %s, release lists %s", got, wantSum)
	}
	return nil
}
