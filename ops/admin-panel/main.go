// Command admin-panel serves a minimal web UI to view and edit the
// production server's .properties config files, and to restart the two
// game services. Authentication is handled by Caddy (basic_auth) in front
// of this app, not here — this binds loopback-only.
package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

var configDir = envOr("CONFIG_DIR", "/opt/acis/config")

var allowedFiles = []string{
	"server.properties",
	"loginserver.properties",
	"players.properties",
	"geoengine.properties",
	"banned_ips.properties",
}

var kvLine = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_.]*)\s*=\s*(.*)$`)

type line struct {
	Index int
	IsKV  bool
	Key   string
	Value string
	Raw   string // full line, used verbatim when not a key/value line
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func isAllowed(name string) bool {
	return slices.Contains(allowedFiles, name)
}

func readLines(path string) ([]line, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	lines := make([]line, len(raw))
	for i, l := range raw {
		if m := kvLine.FindStringSubmatch(l); m != nil {
			lines[i] = line{Index: i, IsKV: true, Key: m[1], Value: m[2], Raw: l}
		} else {
			lines[i] = line{Index: i, IsKV: false, Raw: l}
		}
	}
	return lines, nil
}

const indexTmpl = `<!doctype html><html><head><title>aCis admin</title>
<style>body{font-family:monospace;max-width:700px;margin:40px auto}
h1{font-size:1.2em}a{display:block;margin:8px 0}
.svc{margin:20px 0;padding:12px;border:1px solid #ccc}
button{padding:6px 14px;cursor:pointer}</style></head><body>
<h1>aCis admin panel</h1>
<h2>Config files</h2>
{{range .Files}}<a href="/edit?file={{.}}">{{.}}</a>{{end}}
<h2>Services</h2>
<div class="svc">
<form method="post" action="/restart"><input type="hidden" name="service" value="acis-loginserver">
<button type="submit" onclick="return confirm('Restart loginserver?')">Restart loginserver</button></form>
</div>
<div class="svc">
<form method="post" action="/restart"><input type="hidden" name="service" value="acis-gameserver">
<button type="submit" onclick="return confirm('Restart gameserver?')">Restart gameserver</button></form>
</div>
</body></html>`

const editTmpl = `<!doctype html><html><head><title>{{.File}} - aCis admin</title>
<style>body{font-family:monospace;max-width:900px;margin:40px auto}
.comment{color:#888;white-space:pre}
.row{display:flex;gap:8px;margin:2px 0;align-items:center}
.key{width:280px;text-align:right;color:#333}
input[type=text]{flex:1;font-family:monospace;padding:2px 4px}
button{padding:8px 20px;margin-top:16px;cursor:pointer}
a{display:inline-block;margin-bottom:16px}</style></head><body>
<a href="/">&larr; back</a>
<h1>{{.File}}</h1>
<form method="post" action="/save">
<input type="hidden" name="file" value="{{.File}}">
{{range .Lines}}{{if .IsKV}}<div class="row"><label class="key">{{.Key}} =</label>
<input type="text" name="line_{{.Index}}" value="{{.Value}}"></div>
{{else}}<div class="comment">{{.Raw}}</div>{{end}}{{end}}
<button type="submit">Save</button>
</form></body></html>`

var idx = template.Must(template.New("index").Parse(indexTmpl))
var edt = template.Must(template.New("edit").Parse(editTmpl))

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		files := append([]string{}, allowedFiles...)
		sort.Strings(files)
		idx.Execute(w, map[string]any{"Files": files})
	})

	http.HandleFunc("/edit", func(w http.ResponseWriter, r *http.Request) {
		file := r.URL.Query().Get("file")
		if !isAllowed(file) {
			http.Error(w, "unknown file", http.StatusBadRequest)
			return
		}
		lines, err := readLines(filepath.Join(configDir, file))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		edt.Execute(w, map[string]any{"File": file, "Lines": lines})
	})

	http.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		file := r.FormValue("file")
		if !isAllowed(file) {
			http.Error(w, "unknown file", http.StatusBadRequest)
			return
		}
		path := filepath.Join(configDir, file)
		lines, err := readLines(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		out := make([]string, len(lines))
		for _, l := range lines {
			if l.IsKV {
				if v, ok := r.Form["line_"+strconv.Itoa(l.Index)]; ok {
					out[l.Index] = l.Key + " = " + v[0]
					continue
				}
			}
			out[l.Index] = l.Raw
		}

		backup := path + "." + time.Now().Format("20060102-150405") + ".bak"
		if data, err := os.ReadFile(path); err == nil {
			os.WriteFile(backup, data, 0644)
		}

		tmp := path + ".tmp"
		content := strings.Join(out, "\n") + "\n"
		if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.Rename(tmp, path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("saved %s (backup: %s)", file, backup)
		http.Redirect(w, r, "/edit?file="+file, http.StatusSeeOther)
	})

	http.HandleFunc("/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		svc := r.FormValue("service")
		if svc != "acis-loginserver" && svc != "acis-gameserver" {
			http.Error(w, "unknown service", http.StatusBadRequest)
			return
		}
		cmd := exec.Command("sudo", "systemctl", "restart", svc)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("restart %s failed: %v\n%s", svc, err, out)
			http.Error(w, "restart failed: "+string(out), http.StatusInternalServerError)
			return
		}
		log.Printf("restarted %s", svc)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	log.Println("listening on 127.0.0.1:8081")
	log.Fatal(http.ListenAndServe("127.0.0.1:8081", nil))
}
