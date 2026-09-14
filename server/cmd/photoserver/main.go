package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"image/jpeg"
	"fmt"
	"github.com/disintegration/imaging"
	"log"
	"math/big"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	serverName  = "Local Photo Screensaver"
	apiVersion  = 1
	playlistCap = 50
	defaultAddr = "0.0.0.0:8787"
)

var yearFolderRe = regexp.MustCompile(`^(19|20)\d{2}$`)

type photoMeta struct {
	Path string
	Ext  string
}

type config struct {
	Roots          []string `json:"roots"`
	Order          string   `json:"order"`
	DisplaySeconds int      `json:"displaySeconds"`
	ArchiveRoot    string   `json:"archiveRoot"`
}

type server struct {
	id             string
	configPath     string
	junkPath       string
	mu             sync.RWMutex
	roots          []string
	order          string
	displaySeconds int
	archiveRoot    string
	byID           map[string]photoMeta
	idsOrdered     []string
	scanCount      int
	lastScan       time.Time
	scanning       bool
	junkScanning   bool
	excluded       map[string]bool
	kept           map[string]bool
	junkFlags      map[string]junkFlag
}

func main() {
	addr := flag.String("addr", defaultAddr, "listen address")
	photos := flag.String("photos", "", "optional initial photo directory")
	configPath := flag.String("config", "", "path to JSON config")
	flag.Parse()

	cfgPath := strings.TrimSpace(*configPath)
	if cfgPath == "" {
		cfgPath = defaultConfigPath()
	}

	s := &server{
		id:             newServerID(),
		configPath:     cfgPath,
		junkPath:       junkStatePath(cfgPath),
		byID:           make(map[string]photoMeta),
		order:          "shuffle",
		displaySeconds: 8,
		excluded:       map[string]bool{},
		kept:           map[string]bool{},
		junkFlags:      map[string]junkFlag{},
	}
	s.loadJunk()

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Printf("config load (%s): %v — starting empty", cfgPath, err)
		cfg = config{Order: "shuffle", DisplaySeconds: 8}
	}
	if cfg.Order == "" {
		cfg.Order = "shuffle"
	}
	if cfg.DisplaySeconds < 5 || cfg.DisplaySeconds > 120 {
		cfg.DisplaySeconds = 8
	}
	if len(cfg.Roots) == 0 && strings.TrimSpace(*photos) != "" {
		abs, err := filepath.Abs(*photos)
		if err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				cfg.Roots = []string{abs}
				_ = saveConfig(cfgPath, cfg)
			}
		}
	}

	s.applyConfig(cfg)
	if n, err := s.rescan(); err != nil {
		log.Printf("initial scan: %v", err)
	} else {
		log.Printf("scanned %d JPEG/PNG across %d root(s)", n, len(s.roots))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/spike/playlist", s.handlePlaylist)
	mux.HandleFunc("/api/v1/images/", s.handleImage)
	mux.HandleFunc("/api/v1/admin/state", s.requireLocal(s.handleAdminState))
	mux.HandleFunc("/api/v1/admin/browse", s.requireLocal(s.handleAdminBrowse))
	mux.HandleFunc("/api/v1/admin/roots", s.requireLocal(s.handleAdminRoots))
	mux.HandleFunc("/api/v1/admin/settings", s.requireLocal(s.handleAdminSettings))
	mux.HandleFunc("/api/v1/admin/years", s.requireLocal(s.handleAdminYears))
	mux.HandleFunc("/api/v1/admin/rescan", s.requireLocal(s.handleAdminRescan))
	mux.HandleFunc("/api/v1/admin/junk", s.requireLocal(s.handleAdminJunk))
	mux.HandleFunc("/api/v1/admin/junk/scan", s.requireLocal(s.handleAdminJunkScan))
	mux.HandleFunc("/api/v1/admin/junk/keep", s.requireLocal(s.handleAdminJunkKeep))
	mux.HandleFunc("/api/v1/admin/junk/exclude", s.requireLocal(s.handleAdminJunkExclude))
	mux.HandleFunc("/api/v1/admin/junk/delete", s.requireLocal(s.handleAdminJunkDelete))
	mux.HandleFunc("/api/v1/admin/thumb", s.requireLocal(s.handleAdminThumb))

	log.Printf("listening on http://%s", *addr)
	log.Printf("settings UI: http://127.0.0.1:%s/", portOf(*addr))
	log.Printf("config: %s", cfgPath)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *server) applyConfig(cfg config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roots = normalizeRoots(cfg.Roots)
	s.order = cfg.Order
	s.displaySeconds = cfg.DisplaySeconds
	s.archiveRoot = cfg.ArchiveRoot
}

func (s *server) snapshotConfig() config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return config{
		Roots:          append([]string(nil), s.roots...),
		Order:          s.order,
		DisplaySeconds: s.displaySeconds,
		ArchiveRoot:    s.archiveRoot,
	}
}

func portOf(addr string) string {
	_, p, err := net.SplitHostPort(addr)
	if err != nil || p == "" {
		return "8787"
	}
	return p
}

func defaultConfigPath() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, "photoserver-config.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if f, err := os.OpenFile(candidate, os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			f.Close()
			return candidate
		}
	}
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		dir := filepath.Join(base, "LocalPhotoScreensaver")
		_ = os.MkdirAll(dir, 0o755)
		return filepath.Join(dir, "config.json")
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "local-photo-screensaver")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "config.json")
}

func loadConfig(path string) (config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	var c config
	if err := json.Unmarshal(b, &c); err != nil {
		return config{}, err
	}
	return c, nil
}

func saveConfig(path string, c config) error {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func normalizeRoots(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, r := range in {
		abs, err := filepath.Abs(strings.TrimSpace(r))
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			continue
		}
		key := strings.ToLower(abs)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, abs)
	}
	sort.Strings(out)
	return out
}

func newServerID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "local"
	}
	return "local-" + hex.EncodeToString(b)
}

func (s *server) rescan() (int, error) {
	s.mu.Lock()
	if s.scanning {
		s.mu.Unlock()
		return s.scanCount, fmt.Errorf("scan already in progress")
	}
	s.scanning = true
	roots := append([]string(nil), s.roots...)
	s.mu.Unlock()

	next := make(map[string]photoMeta)
	count := 0
	var walkErr error
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				log.Printf("walk skip: %v", err)
				return nil
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
				return nil
			}
			id := opaqueID(root, path)
			next[id] = photoMeta{Path: path, Ext: ext}
			count++
			if count%5000 == 0 {
				log.Printf("scan progress: %d images...", count)
			}
			return nil
		})
		if err != nil {
			walkErr = err
		}
	}

	type pair struct{ id, path string }
	pairs := make([]pair, 0, len(next))
	for id, m := range next {
		pairs = append(pairs, pair{id, m.Path})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return strings.ToLower(pairs[i].path) < strings.ToLower(pairs[j].path)
	})
	ordered := make([]string, 0, len(pairs))
	for _, p := range pairs {
		ordered = append(ordered, p.id)
	}

	s.mu.Lock()
	s.byID = next
	s.idsOrdered = ordered
	s.scanCount = count
	s.lastScan = time.Now()
	s.scanning = false
	s.mu.Unlock()
	return count, walkErr
}

func opaqueID(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	key := filepath.ToSlash(root) + "|" + filepath.ToSlash(rel)
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}

func isLocalhost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *server) requireLocal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLocalhost(r) {
			http.Error(w, "admin API is localhost-only", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !isLocalhost(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "%s is running.\nOpen settings on the PC at http://127.0.0.1:%s/\n", serverName, portOf(r.Host))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, adminHTML)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	writeJSON(w, map[string]any{
		"status":      "ok",
		"serverId":    s.id,
		"serverName":  serverName,
		"apiVersions": []int{apiVersion},
		"roots":       len(s.roots),
		"photos":      s.scanCount,
		"order":       s.order,
	})
}

type playlistItem struct {
	ID       string `json:"id"`
	ImageURL string `json:"imageUrl"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

func (s *server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = playlistCap
	}
	seedStr := strings.TrimSpace(q.Get("seed"))
	var seed int64
	if seedStr != "" {
		seed, _ = strconv.ParseInt(seedStr, 10, 64)
	}

	s.mu.RLock()
	orderedAll := append([]string(nil), s.idsOrdered...)
	ex := copyBoolMap(s.excluded)
	orderMode := s.order
	secs := s.displaySeconds
	s.mu.RUnlock()

	ordered := make([]string, 0, len(orderedAll))
	for _, id := range orderedAll {
		if ex[id] {
			continue
		}
		ordered = append(ordered, id)
	}

	total := len(ordered)
	ids := ordered
	if orderMode != "path" {
		if seed == 0 {
			n, err := rand.Int(rand.Reader, big.NewInt(1<<31-1))
			if err != nil {
				seed = time.Now().Unix() & 0x7fffffff
			} else {
				seed = n.Int64()
			}
		}
		ids = append([]string(nil), ordered...)
		rng := mrand.New(mrand.NewSource(seed))
		rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	} else {
		seed = 0
	}
	seedOut := strconv.FormatInt(seed, 10)

	if secs < 5 {
		secs = 8
	}
	if total == 0 {
		writeJSON(w, map[string]any{
			"displaySeconds": secs, "transition": "fade", "order": orderMode,
			"seed": seedOut, "offset": 0, "limit": limit, "total": 0, "nextOffset": 0,
			"items": []playlistItem{},
		})
		return
	}
	if offset >= total {
		offset = offset % total
	}
	endPos := offset + limit
	if endPos > total {
		endPos = total
	}
	page := ids[offset:endPos]
	nextOffset := endPos
	if nextOffset >= total {
		nextOffset = 0
	}
	items := make([]playlistItem, 0, len(page))
	for _, id := range page {
		items = append(items, playlistItem{ID: id, ImageURL: "/api/v1/images/" + id})
	}
	writeJSON(w, map[string]any{
		"displaySeconds": secs, "transition": "fade", "order": orderMode,
		"seed": seedOut, "offset": offset, "limit": limit, "total": total,
		"nextOffset": nextOffset, "items": items,
	})
}

func (s *server) handleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/images/")
	id = strings.Trim(id, "/")
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") || !isHexID(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.mu.RLock()
	meta, ok := s.byID[id]
	roots := append([]string(nil), s.roots...)
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	clean := filepath.Clean(meta.Path)
	if !underAnyRoot(roots, clean) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	info, err := os.Stat(clean)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	cachePath := orientedCachePath(id, info.ModTime().UnixNano(), info.Size())
	if b, err := os.ReadFile(cachePath); err == nil && len(b) > 0 {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(b)
		return
	}

	img, err := imaging.Open(clean, imaging.AutoOrientation(true))
	if err != nil {
		log.Printf("orient open %s: %v", id, err)
		// Fall back to raw file bytes
		switch meta.Ext {
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		default:
			w.Header().Set("Content-Type", "image/jpeg")
		}
		http.ServeFile(w, r, clean)
		return
	}

	// Fit for Roku FHD — keeps phone originals from being huge over LAN
	const maxEdge = 1920
	b := img.Bounds()
	wdt, hgt := b.Dx(), b.Dy()
	if wdt > maxEdge || hgt > maxEdge {
		img = imaging.Fit(img, maxEdge, maxEdge, imaging.Lanczos)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		http.Error(w, "encode failed", http.StatusInternalServerError)
		return
	}
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
	_ = os.WriteFile(cachePath, buf.Bytes(), 0o644)

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(buf.Bytes())
}

func orientedCachePath(id string, modNano, size int64) string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if runtime.GOOS == "windows" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		} else {
			base, _ = os.UserHomeDir()
			base = filepath.Join(base, ".cache")
		}
	}
	dir := filepath.Join(base, "LocalPhotoScreensaver", "orient-cache")
	name := fmt.Sprintf("%s_%d_%d.jpg", id, modNano, size)
	return filepath.Join(dir, name)
}

func underAnyRoot(roots []string, path string) bool {
	for _, root := range roots {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func (s *server) handleAdminState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	writeJSON(w, map[string]any{
		"roots": s.roots, "photos": s.scanCount, "scanning": s.scanning, "lastScan": s.lastScan,
		"config": s.configPath, "playlistCap": playlistCap, "order": s.order,
		"displaySeconds": s.displaySeconds, "archiveRoot": s.archiveRoot,
	})
}

func (s *server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Order          string `json:"order"`
		DisplaySeconds int    `json:"displaySeconds"`
		ArchiveRoot    string `json:"archiveRoot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	cfg := s.snapshotConfig()
	if body.Order == "shuffle" || body.Order == "path" {
		cfg.Order = body.Order
	}
	if body.DisplaySeconds >= 5 && body.DisplaySeconds <= 120 {
		cfg.DisplaySeconds = body.DisplaySeconds
	}
	if body.ArchiveRoot != "" {
		abs, err := filepath.Abs(body.ArchiveRoot)
		if err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				cfg.ArchiveRoot = abs
			}
		}
	}
	if err := saveConfig(s.configPath, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.applyConfig(cfg)
	writeJSON(w, cfg)
}

func (s *server) handleAdminYears(w http.ResponseWriter, r *http.Request) {
	cfg := s.snapshotConfig()
	root := cfg.ArchiveRoot
	if root == "" {
		writeJSON(w, map[string]any{"archiveRoot": "", "years": []any{}})
		return
	}
	switch r.Method {
	case http.MethodGet:
		ents, err := os.ReadDir(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		selected := map[string]bool{}
		for _, rth := range cfg.Roots {
			selected[strings.ToLower(filepath.Clean(rth))] = true
		}
		years := []map[string]any{}
		for _, e := range ents {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			p := filepath.Join(root, e.Name())
			years = append(years, map[string]any{
				"name": e.Name(), "path": p, "selected": selected[strings.ToLower(filepath.Clean(p))],
			})
		}
		sort.Slice(years, func(i, j int) bool { return years[i]["name"].(string) < years[j]["name"].(string) })
		writeJSON(w, map[string]any{"archiveRoot": root, "years": years})
	case http.MethodPost:
		var body struct {
			Selected []string `json:"selected"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		kept := []string{}
		for _, rth := range cfg.Roots {
			rel, err := filepath.Rel(root, rth)
			if err != nil || strings.HasPrefix(rel, "..") {
				kept = append(kept, rth)
			}
		}
		cfg.Roots = normalizeRoots(append(kept, body.Selected...))
		if err := saveConfig(s.configPath, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.applyConfig(cfg)
		n, err := s.rescan()
		if err != nil {
			log.Printf("rescan: %v", err)
		}
		writeJSON(w, map[string]any{"roots": cfg.Roots, "photos": n})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) handleAdminBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Query().Get("path")
	entries := []map[string]any{}
	if path == "" {
		if runtime.GOOS == "windows" {
			for _, letter := range "CDEFGHIJKLMNOPQRSTUVWXYZ" {
				root := string(letter) + `:\`
				if st, err := os.Stat(root); err == nil && st.IsDir() {
					entries = append(entries, map[string]any{"name": root, "path": root, "dir": true})
				}
			}
		} else {
			home, _ := os.UserHomeDir()
			entries = append(entries, map[string]any{"name": home, "path": home, "dir": true})
		}
		writeJSON(w, map[string]any{"path": "", "parent": "", "entries": entries})
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		http.Error(w, "not a directory", http.StatusBadRequest)
		return
	}
	dirents, err := os.ReadDir(abs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, d := range dirents {
		if !d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			continue
		}
		entries = append(entries, map[string]any{"name": d.Name(), "path": filepath.Join(abs, d.Name()), "dir": true})
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i]["name"].(string)) < strings.ToLower(entries[j]["name"].(string))
	})
	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}
	writeJSON(w, map[string]any{"path": abs, "parent": parent, "entries": entries})
}

func (s *server) handleAdminRoots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Add    string `json:"add"`
		Remove string `json:"remove"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	cfg := s.snapshotConfig()
	roots := append([]string(nil), cfg.Roots...)
	if body.Add != "" {
		abs, err := filepath.Abs(body.Add)
		if err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				roots = append(roots, abs)
			}
		}
	}
	if body.Remove != "" {
		rm := strings.ToLower(filepath.Clean(body.Remove))
		filtered := roots[:0]
		for _, root := range roots {
			if strings.ToLower(root) != rm {
				filtered = append(filtered, root)
			}
		}
		roots = filtered
	}
	cfg.Roots = normalizeRoots(roots)
	if err := saveConfig(s.configPath, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.applyConfig(cfg)
	n, _ := s.rescan()
	writeJSON(w, map[string]any{"roots": cfg.Roots, "photos": n})
}

func (s *server) handleAdminRescan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	n, err := s.rescan()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]any{"photos": n})
}

func isHexID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}


func junkStatePath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "photoserver-junk.json")
}

func copyBoolMap(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (s *server) loadJunk() {
	b, err := os.ReadFile(s.junkPath)
	if err != nil {
		return
	}
	var st junkState
	if err := json.Unmarshal(b, &st); err != nil {
		log.Printf("junk state: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.excluded = map[string]bool{}
	s.kept = map[string]bool{}
	s.junkFlags = map[string]junkFlag{}
	for _, id := range st.Excluded {
		s.excluded[id] = true
	}
	for _, id := range st.Kept {
		s.kept[id] = true
	}
	if st.Flags != nil {
		s.junkFlags = st.Flags
	}
}

func (s *server) saveJunkLocked() error {
	st := junkState{
		Excluded: make([]string, 0, len(s.excluded)),
		Kept:     make([]string, 0, len(s.kept)),
		Flags:    s.junkFlags,
	}
	for id := range s.excluded {
		st.Excluded = append(st.Excluded, id)
	}
	for id := range s.kept {
		st.Kept = append(st.Kept, id)
	}
	sort.Strings(st.Excluded)
	sort.Strings(st.Kept)
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.junkPath, b, 0o644)
}

func (s *server) handleAdminJunk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]map[string]any, 0, len(s.junkFlags))
	for id, fl := range s.junkFlags {
		items = append(items, map[string]any{
			"id": id, "path": fl.Path, "reasons": fl.Reasons,
			"excluded": s.excluded[id], "kept": s.kept[id],
			"thumb": "/api/v1/admin/thumb?id=" + id,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i]["path"].(string) < items[j]["path"].(string)
	})
	writeJSON(w, map[string]any{
		"items": items, "excludedCount": len(s.excluded), "keptCount": len(s.kept),
		"flaggedCount": len(s.junkFlags), "scanning": s.junkScanning,
	})
}

func (s *server) handleAdminJunkScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	if s.junkScanning {
		s.mu.Unlock()
		http.Error(w, "junk scan already running", http.StatusConflict)
		return
	}
	s.junkScanning = true
	// snapshot photos
	type pair struct{ id, path string }
	pairs := make([]pair, 0, len(s.byID))
	for id, m := range s.byID {
		pairs = append(pairs, pair{id, m.Path})
	}
	kept := copyBoolMap(s.kept)
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.junkScanning = false
			s.mu.Unlock()
		}()
		newFlags := map[string]junkFlag{}
		newExcluded := map[string]bool{}
		for i, p := range pairs {
			reasons := detectJunk(p.path)
			if !junkWorthy(reasons) {
				continue
			}
			newFlags[p.id] = junkFlag{ID: p.id, Path: p.path, Reasons: reasons}
			if !kept[p.id] {
				newExcluded[p.id] = true
			}
			if (i+1)%500 == 0 {
				log.Printf("junk scan progress %d/%d", i+1, len(pairs))
			}
		}
		s.mu.Lock()
		// merge: keep manual excludes that aren't in flags? keep all newExcluded + previous excludes that user set
		for id := range newExcluded {
			s.excluded[id] = true
		}
		// refresh flags map (replace auto flags)
		s.junkFlags = newFlags
		// ensure kept items are not excluded
		for id := range s.kept {
			delete(s.excluded, id)
		}
		err := s.saveJunkLocked()
		nEx, nFl := len(s.excluded), len(s.junkFlags)
		s.mu.Unlock()
		if err != nil {
			log.Printf("save junk: %v", err)
		} else {
			log.Printf("junk scan done: flagged=%d excluded=%d", nFl, nEx)
		}
	}()

	writeJSON(w, map[string]any{"started": true, "photos": len(pairs)})
}

func (s *server) handleAdminJunkKeep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct{ ID string `json:"id"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.kept[body.ID] = true
	delete(s.excluded, body.ID)
	err := s.saveJunkLocked()
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *server) handleAdminJunkExclude(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct{ ID string `json:"id"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	delete(s.kept, body.ID)
	s.excluded[body.ID] = true
	err := s.saveJunkLocked()
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *server) handleAdminJunkDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct{ ID string `json:"id"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	meta, ok := s.byID[body.ID]
	roots := append([]string(nil), s.roots...)
	s.mu.Unlock()
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	clean := filepath.Clean(meta.Path)
	if !underAnyRoot(roots, clean) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := os.Remove(clean); err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	delete(s.byID, body.ID)
	// rebuild ordered list
	ordered := s.idsOrdered[:0]
	for _, id := range s.idsOrdered {
		if id != body.ID {
			ordered = append(ordered, id)
		}
	}
	s.idsOrdered = append([]string(nil), ordered...)
	s.scanCount = len(s.byID)
	delete(s.excluded, body.ID)
	delete(s.kept, body.ID)
	delete(s.junkFlags, body.ID)
	err := s.saveJunkLocked()
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "deleted": clean})
}

func (s *server) handleAdminThumb(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if !isHexID(id) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.mu.RLock()
	meta, ok := s.byID[id]
	roots := append([]string(nil), s.roots...)
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	clean := filepath.Clean(meta.Path)
	if !underAnyRoot(roots, clean) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	img, err := imaging.Open(clean, imaging.AutoOrientation(true))
	if err != nil {
		http.Error(w, "open failed", http.StatusInternalServerError)
		return
	}
	thumb := imaging.Fit(img, 320, 320, imaging.Lanczos)
	w.Header().Set("Content-Type", "image/jpeg")
	_ = jpeg.Encode(w, thumb, &jpeg.Options{Quality: 70})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

const adminHTML = `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8"/><meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Local Photo Screensaver</title>
<style>
:root{color-scheme:dark;--bg:#12141c;--card:#1c2030;--fg:#e8eaf0;--muted:#9aa3b5;--accent:#6ea8fe;--danger:#f07178}
*{box-sizing:border-box}body{margin:0;font:15px/1.45 system-ui,Segoe UI,sans-serif;background:var(--bg);color:var(--fg)}
main{max-width:960px;margin:0 auto;padding:24px 16px 80px}h1{font-size:1.35rem;margin:0 0 6px}h2{font-size:1.05rem;margin:0 0 10px}
.muted{color:var(--muted)}.row{display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin:10px 0}
button{background:var(--accent);color:#0b1020;border:0;border-radius:8px;padding:8px 12px;font-weight:600;cursor:pointer}
button.secondary{background:#2a3146;color:var(--fg)}button.danger{background:var(--danger);color:#1a0a0a}
.card{background:var(--card);border-radius:12px;padding:16px;margin-top:16px}
.path{font-family:ui-monospace,Consolas,monospace;font-size:13px;word-break:break-all}
ul{list-style:none;padding:0;margin:0}li{display:flex;justify-content:space-between;gap:12px;align-items:center;padding:10px 0;border-bottom:1px solid #2a3146}
li:last-child{border-bottom:0}.browser li span{cursor:pointer}.browser li span:hover{color:var(--accent)}
.years{display:flex;flex-wrap:wrap;gap:8px}.years label{background:#2a3146;padding:8px 10px;border-radius:8px;cursor:pointer;user-select:none}
.years input{margin-right:6px}select,input[type=number]{background:#0f1320;color:var(--fg);border:1px solid #2a3146;border-radius:8px;padding:8px 10px}
</style></head><body><main>
<h1>Local Photo Screensaver</h1>
<p class="muted">Pick years / folders and how photos play. Set the Roku screensaver server IP to this PC’s LAN address.</p>
<div class="row" id="status">Loading…</div>
<div class="card"><h2>Playback</h2><div class="row">
<label>Order <select id="order"><option value="shuffle">Shuffle</option><option value="path">In order (by folder / filename)</option></select></label>
<label>Seconds per photo <input id="secs" type="number" min="5" max="120" value="8" style="width:5rem"/></label>
<button id="saveSettings">Save</button>
</div></div>
<div class="card"><h2>Years on external drive</h2>
<p class="muted" id="archiveLabel">Archive: …</p>
<p class="muted">Checking a year applies and scans automatically. Rescan only refreshes folders already selected.</p>
<div class="row"><button class="secondary" id="allYears">Select all</button><button class="secondary" id="noYears">Clear</button><button id="saveYears">Apply selected years</button></div>
<div class="years" id="years"></div></div>
<div class="card"><h2>Selected folders</h2><ul id="roots"></ul>
<div class="row"><button class="secondary" id="rescan">Rescan now</button></div></div>

<div class="card"><h2>Junk / not for albums</h2>
<p class="muted">Finds likely screenshots, documents, and similar. Flagged items are <b>excluded from the Roku</b> by default. Delete only removes files you confirm.</p>
<div class="row">
  <button id="junkScan">Scan library for junk</button>
  <button class="secondary" id="junkRefresh">Refresh list</button>
  <span class="muted" id="junkStatus"></span>
</div>
<div id="junkList" style="display:grid;grid-template-columns:repeat(auto-fill,minmax(180px,1fr));gap:12px;margin-top:12px"></div>
</div>
<div class="card"><h2>Browse &amp; add any folder</h2>
<div class="row"><button class="secondary" id="up">Up</button><span class="path" id="cwd"></span></div>
<ul class="browser" id="browser"></ul></div>
</main>
<script>
let cwd='',parent='';
async function api(path,opts){const r=await fetch(path,opts);if(!r.ok)throw new Error(await r.text());return r.json()}
function renderRoots(state){
  const ul=document.getElementById('roots');ul.innerHTML='';
  if(!state.roots?.length)ul.innerHTML='<li class="muted">None yet — tick a year above (auto-applies) or browse below.</li>';
  else for(const root of state.roots){const li=document.createElement('li');const sp=document.createElement('span');sp.className='path';sp.textContent=root;
    const btn=document.createElement('button');btn.className='danger';btn.textContent='Remove';
    btn.onclick=async()=>{await api('/api/v1/admin/roots',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({remove:root})});await refresh()};
    li.append(sp,btn);ul.appendChild(li)}
  document.getElementById('order').value=state.order||'shuffle';
  document.getElementById('secs').value=state.displaySeconds||8;
  document.getElementById('status').textContent=(state.scanning?'Scanning…':(state.photos+' photos indexed'))+' · Roku loads '+state.playlistCap+' at a time, then the next batch';
}
async function loadYears(){
  const data=await api('/api/v1/admin/years');
  document.getElementById('archiveLabel').textContent=data.archiveRoot?('Archive: '+data.archiveRoot):'No archive root set';
  const box=document.getElementById('years');box.innerHTML='';
  for(const y of data.years||[]){const lab=document.createElement('label');const cb=document.createElement('input');cb.type='checkbox';cb.value=y.path;cb.checked=!!y.selected;lab.append(cb,document.createTextNode(y.name));box.appendChild(lab)}
}
async function refresh(){renderRoots(await api('/api/v1/admin/state'));await loadYears()}
async function browse(path){
  const data=await api('/api/v1/admin/browse?path='+encodeURIComponent(path||''));
  cwd=data.path||'';parent=data.parent||'';document.getElementById('cwd').textContent=cwd||'(drives)';
  const ul=document.getElementById('browser');ul.innerHTML='';
  for(const e of data.entries||[]){const li=document.createElement('li');const name=document.createElement('span');name.className='path';name.textContent=e.name;name.onclick=()=>browse(e.path);
    const add=document.createElement('button');add.textContent='Add';
    add.onclick=async(ev)=>{ev.stopPropagation();document.getElementById('status').textContent='Adding & scanning…';await api('/api/v1/admin/roots',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({add:e.path})});await refresh()};
    li.append(name,add);ul.appendChild(li)}
}
async function applyYears(){
  const selected=[...document.querySelectorAll('#years input:checked')].map(cb=>cb.value);
  document.getElementById('status').textContent=selected.length?('Applying '+selected.length+' year folder(s) & scanning…'):'No years checked — clearing selection…';
  await api('/api/v1/admin/years',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({selected})});await refresh();
}
document.getElementById('up').onclick=()=>browse(parent);
document.getElementById('rescan').onclick=async()=>{const state=await api('/api/v1/admin/state');if(!state.roots?.length){document.getElementById('status').textContent='Nothing to scan — tick a year (auto-applies) or Add a folder first.';return}document.getElementById('status').textContent='Scanning…';await api('/api/v1/admin/rescan',{method:'POST'});await refresh()};
document.getElementById('saveSettings').onclick=async()=>{await api('/api/v1/admin/settings',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({order:document.getElementById('order').value,displaySeconds:Number(document.getElementById('secs').value)})});await refresh()};
document.getElementById('allYears').onclick=async()=>{document.querySelectorAll('#years input').forEach(cb=>cb.checked=true);await applyYears()};
document.getElementById('noYears').onclick=async()=>{document.querySelectorAll('#years input').forEach(cb=>cb.checked=false);await applyYears()};
document.getElementById('saveYears').onclick=()=>applyYears();
document.getElementById('years').addEventListener('change',e=>{if(e.target&&e.target.matches('input[type=checkbox]'))applyYears()});

async function loadJunk(){
  const data=await api('/api/v1/admin/junk');
  document.getElementById('junkStatus').textContent=
    (data.scanning?'Scanning… ': '') + data.flaggedCount+' flagged · '+data.excludedCount+' excluded · '+data.keptCount+' kept';
  const box=document.getElementById('junkList'); box.innerHTML='';
  for(const it of (data.items||[]).slice(0,200)){
    const card=document.createElement('div');
    card.style.cssText='background:#0f1320;border-radius:10px;padding:8px;display:flex;flex-direction:column;gap:6px';
    const img=document.createElement('img');
    img.src=it.thumb; img.alt=''; img.style.cssText='width:100%;height:120px;object-fit:cover;border-radius:6px;background:#222';
    const path=document.createElement('div'); path.className='path'; path.style.fontSize='11px';
    path.textContent=(it.path||'').split(/[/\\\\]/).slice(-2).join('/');
    const why=document.createElement('div'); why.className='muted'; why.style.fontSize='11px';
    why.textContent=(it.reasons||[]).join(', ');
    const row=document.createElement('div'); row.className='row'; row.style.margin='0';
    const keep=document.createElement('button'); keep.className='secondary'; keep.textContent='Keep';
    keep.onclick=async()=>{await api('/api/v1/admin/junk/keep',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:it.id})});await loadJunk()};
    const excl=document.createElement('button'); excl.className='secondary'; excl.textContent='Exclude';
    excl.onclick=async()=>{await api('/api/v1/admin/junk/exclude',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:it.id})});await loadJunk()};
    const del=document.createElement('button'); del.className='danger'; del.textContent='Delete';
    del.onclick=async()=>{if(!confirm('Permanently delete this file from disk?'))return;await api('/api/v1/admin/junk/delete',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:it.id})});await loadJunk()};
    row.append(keep,excl,del);
    const badge=document.createElement('div'); badge.className='muted'; badge.style.fontSize='11px';
    badge.textContent=it.kept?'status: kept':(it.excluded?'status: excluded':'status: flagged');
    card.append(img,path,why,badge,row); box.appendChild(card);
  }
}
document.getElementById('junkScan').onclick=async()=>{
  document.getElementById('junkStatus').textContent='Starting scan…';
  await api('/api/v1/admin/junk/scan',{method:'POST'});
  const poll=async()=>{
    const data=await api('/api/v1/admin/junk');
    document.getElementById('junkStatus').textContent=
      (data.scanning?'Scanning… ': 'Done. ') + data.flaggedCount+' flagged · '+data.excludedCount+' excluded';
    if(data.scanning) setTimeout(poll,2000); else await loadJunk();
  };
  setTimeout(poll,1500);
};
document.getElementById('junkRefresh').onclick=()=>loadJunk();
loadJunk().catch(()=>{});
refresh().then(()=>browse('')).catch(err=>{document.getElementById('status').textContent=String(err);browse('')});
</script></body></html>
`
