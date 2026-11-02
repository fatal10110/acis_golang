package main

import (
	"archive/zip"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	// maxDownload caps the artifact zip and each binary inside it; both
	// binaries together are ~60 MB.
	maxDownload = 512 << 20

	artifactName = "deploy-binaries"
	// deployWorkflow is the only workflow whose artifacts may be deployed.
	deployWorkflow = ".github/workflows/deploy.yml"
	deployBranch   = "main"
)

// binaries are the files the artifact must carry, each saved under the same
// name in the binary directory.
var binaries = []string{"gameserver", "loginserver"}

// services are restarted in this order once the new binaries are in place.
var services = []string{"acis-loginserver", "acis-gameserver"}

type config struct {
	token   string // bearer token the caller must present
	api     string // GitHub API base URL
	repo    string // owner/name the artifact is pulled from
	binDir  string
	client  *http.Client
	restart func(unit string) error
}

func main() {
	cfg := config{
		token:   os.Getenv("DEPLOY_TOKEN"),
		api:     "https://api.github.com",
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

// newHandler takes a POST naming a Deploy workflow run, pulls that run's
// binaries artifact from GitHub with the caller's short-lived job token, swaps
// the binaries in, restarts the services, and answers only once that is done
// so the caller sees the real outcome. One deploy runs at a time.
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
		runID, err := strconv.ParseUint(r.Header.Get("X-Deploy-Run"), 10, 64)
		ghToken := r.Header.Get("X-GitHub-Token")
		if err != nil || ghToken == "" {
			http.Error(w, "X-Deploy-Run and X-GitHub-Token required", http.StatusBadRequest)
			return
		}
		if !mu.TryLock() {
			http.Error(w, "deploy already running", http.StatusConflict)
			return
		}
		defer mu.Unlock()

		gh := github{cfg: cfg, token: ghToken}
		sha, err := gh.pull(runID)
		if err != nil {
			log.Printf("deploy run %d: %v", runID, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		log.Printf("deploy run %d: installed %s", runID, sha)
		for _, unit := range services {
			if err := cfg.restart(unit); err != nil {
				log.Printf("deploy run %d: %v", runID, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		log.Printf("deploy run %d: finished", runID)
		fmt.Fprintf(w, "deployed %s\n", sha)
	})
}

type github struct {
	cfg   config
	token string
}

// pull checks that the run is a Deploy run on main, downloads its binaries
// artifact, and installs the binaries. It returns the run's commit.
func (g github) pull(runID uint64) (string, error) {
	var run struct {
		Path       string `json:"path"`
		HeadBranch string `json:"head_branch"`
		HeadSHA    string `json:"head_sha"`
	}
	if err := g.getJSON(fmt.Sprintf("/repos/%s/actions/runs/%d", g.cfg.repo, runID), &run); err != nil {
		return "", err
	}
	if run.Path != deployWorkflow || run.HeadBranch != deployBranch {
		return "", fmt.Errorf("run is %s on %s, want %s on %s", run.Path, run.HeadBranch, deployWorkflow, deployBranch)
	}

	var list struct {
		Artifacts []struct {
			URL     string `json:"archive_download_url"`
			Expired bool   `json:"expired"`
		} `json:"artifacts"`
	}
	if err := g.getJSON(fmt.Sprintf("/repos/%s/actions/runs/%d/artifacts?name=%s", g.cfg.repo, runID, artifactName), &list); err != nil {
		return "", err
	}
	if len(list.Artifacts) != 1 || list.Artifacts[0].Expired {
		return "", fmt.Errorf("run has no live %s artifact", artifactName)
	}

	zipPath, err := g.download(list.Artifacts[0].URL)
	if err != nil {
		return "", err
	}
	defer os.Remove(zipPath)
	return run.HeadSHA, install(zipPath, g.cfg.binDir)
}

func (g github) get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	// The archive URL redirects to presigned storage, which must never see
	// the job token.
	client := *g.cfg.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		req.Header.Del("Authorization")
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", req.URL.Path, resp.Status)
	}
	return resp, nil
}

func (g github) getJSON(path string, v any) error {
	resp, err := g.get(g.cfg.api + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// download saves the artifact zip next to the binaries; zip needs random
// access, so it cannot be streamed.
func (g github) download(url string) (string, error) {
	resp, err := g.get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	f, err := os.CreateTemp(g.cfg.binDir, "deploy-*.zip")
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxDownload+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxDownload {
		err = errors.New("artifact too large")
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// install extracts every expected binary as <name>.new and renames them all
// into place only after each one was extracted intact, so a bad artifact never
// touches the running binaries.
func install(zipPath, binDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	staged := map[string]string{}
	defer func() {
		for _, tmp := range staged {
			os.Remove(tmp)
		}
	}()

	for _, name := range binaries {
		zf, err := zr.Open(name)
		if err != nil {
			return fmt.Errorf("artifact: %w", err)
		}
		tmp := filepath.Join(binDir, name+".new")
		staged[name] = tmp
		err = save(tmp, io.LimitReader(zf, maxDownload))
		zf.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
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
	head := make([]byte, 4)
	if _, err := io.ReadFull(src, head); err != nil || string(head) != "\x7fELF" {
		return errors.New("not an ELF binary")
	}
	// Artifact zips drop file modes, so the mode is set here.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
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
