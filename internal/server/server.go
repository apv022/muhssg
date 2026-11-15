// Package server is muhssg's development server: it rebuilds on change and
// reloads the browser.
package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apv022/muhssg/internal/site"
	"github.com/fsnotify/fsnotify"
)

// Options configure the dev server.
type Options struct {
	Root   string
	Host   string
	Port   int
	Drafts bool
	Open   func(url string) // optional, called once the server is up
}

const eventsPath = "/__muhssg/events"

const reloadScript = `<script>(()=>{const es=new EventSource("` + eventsPath + `");let box;
es.addEventListener("reload",()=>location.reload());
es.addEventListener("error-msg",e=>{const m=JSON.parse(e.data);if(!box){box=document.createElement("pre");
box.style.cssText="position:fixed;inset:auto 0 0 0;z-index:2147483647;margin:0;padding:16px 20px;max-height:50vh;overflow:auto;background:#1a0006;color:#ffd6dc;font:13px/1.5 ui-monospace,monospace;border-top:3px solid #dc143c;white-space:pre-wrap";
document.body.appendChild(box)}box.textContent="muhssg build failed\n\n"+m});})();</script>`

type hub struct {
	mu      sync.Mutex
	clients map[chan event]bool
	lastErr string
}

type event struct{ name, data string }

func (h *hub) broadcast(e event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e.name == "error-msg" {
		h.lastErr = e.data
	} else {
		h.lastErr = ""
	}
	for c := range h.clients {
		select {
		case c <- e:
		default:
		}
	}
}

func (h *hub) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	c := make(chan event, 4)
	h.mu.Lock()
	h.clients[c] = true
	lastErr := h.lastErr
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()
	fmt.Fprint(w, ": connected\n\n")
	if lastErr != "" {
		fmt.Fprintf(w, "event: error-msg\ndata: %s\n\n", lastErr)
	}
	flusher.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-c:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, e.data)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// Run builds the site, serves it, and rebuilds whenever a source file changes.
func Run(opts Options) error {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}
	out, err := os.MkdirTemp("", "muhssg-serve-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(out)
	out = filepath.Join(out, "site")

	h := &hub{clients: map[chan event]bool{}}
	var mu sync.Mutex
	basePath := ""
	build := func() {
		mu.Lock()
		defer mu.Unlock()
		res, err := site.Build(site.Options{Root: root, OutDir: out, Drafts: opts.Drafts, Dev: true})
		if err != nil {
			log.Printf("✗ build failed: %v", err)
			msg, _ := json.Marshal(err.Error())
			h.broadcast(event{"error-msg", string(msg)})
			return
		}
		basePath = res.Site.Config.BasePath
		log.Printf("✓ built %d pages in %s", res.Pages, res.Duration.Round(time.Millisecond))
		h.broadcast(event{"reload", "{}"})
	}
	build()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	cfg, err := site.LoadConfig(root)
	if err != nil {
		return err
	}
	outputDir := filepath.Clean(cfg.OutputDir())
	// ignored reports whether a path should not trigger rebuilds or be watched.
	ignored := func(p string) bool {
		p = filepath.Clean(p)
		if p == outputDir || strings.HasPrefix(p, outputDir+string(filepath.Separator)) || strings.HasPrefix(p, outputDir+".old-") {
			return true
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return false
		}
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			if strings.HasPrefix(part, ".") || part == "node_modules" {
				return true
			}
		}
		return false
	}
	addTree := func(dir string) {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if ignored(p) {
				return filepath.SkipDir
			}
			_ = watcher.Add(p)
			return nil
		})
	}
	addTree(root)
	for _, t := range cfg.ThemeDirs() {
		if !strings.HasPrefix(t, root+string(filepath.Separator)) {
			addTree(t)
		}
	}

	go func() {
		var timer *time.Timer
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ignored(ev.Name) || strings.HasSuffix(ev.Name, "~") {
					continue
				}
				if ev.Has(fsnotify.Create) {
					if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
						addTree(ev.Name)
					}
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(120*time.Millisecond, build)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("watch error: %v", err)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc(eventsPath, h.serveEvents)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		bp := basePath
		mu.Unlock()
		serveFile(w, r, out, bp)
	})

	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := opts.Port
	var ln net.Listener
	for i := 0; i < 20; i++ {
		ln, err = net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(port+i)))
		if err == nil {
			port += i
			break
		}
	}
	if err != nil {
		return fmt.Errorf("no free port from %d to %d: %w", opts.Port, opts.Port+19, err)
	}
	url := fmt.Sprintf("http://%s%s/", net.JoinHostPort(displayHost(host), fmt.Sprint(port)), basePath)
	log.Printf("serving %s at %s (drafts: %v) — Ctrl+C to stop", filepath.Base(root), url, opts.Drafts)
	if opts.Open != nil {
		opts.Open(url)
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func displayHost(h string) string {
	if h == "0.0.0.0" || h == "::" {
		return "localhost"
	}
	return h
}

// serveFile serves the build output with clean URLs, the 404 page, and the
// live-reload script injected into HTML.
func serveFile(w http.ResponseWriter, r *http.Request, out, basePath string) {
	p := r.URL.Path
	if basePath != "" {
		if p == basePath {
			http.Redirect(w, r, basePath+"/", http.StatusFound)
			return
		}
		if !strings.HasPrefix(p, basePath+"/") {
			notFound(w, out)
			return
		}
		p = strings.TrimPrefix(p, basePath)
	}
	clean := path.Clean("/" + p)
	file := filepath.Join(out, filepath.FromSlash(clean))
	st, err := os.Stat(file)
	if err == nil && st.IsDir() {
		if !strings.HasSuffix(p, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			return
		}
		file = filepath.Join(file, "index.html")
		st, err = os.Stat(file)
	}
	if err != nil {
		if alt := file + ".html"; fileExists(alt) {
			file = alt
		} else {
			notFound(w, out)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasSuffix(file, ".html") {
		serveHTML(w, file, http.StatusOK)
		return
	}
	http.ServeFile(w, r, file)
}

func notFound(w http.ResponseWriter, out string) {
	w.Header().Set("Cache-Control", "no-store")
	if f := filepath.Join(out, "404.html"); fileExists(f) {
		serveHTML(w, f, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, "<!doctype html><title>404</title><p>Not found. Add content/404.md or templates/404.html.</p>"+reloadScript)
}

func serveHTML(w http.ResponseWriter, file string, status int) {
	raw, err := os.ReadFile(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if i := bytes.LastIndex(bytes.ToLower(raw), []byte("</body>")); i >= 0 {
		raw = append(raw[:i:i], append([]byte(reloadScript), raw[i:]...)...)
	} else {
		raw = append(raw, []byte(reloadScript)...)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
