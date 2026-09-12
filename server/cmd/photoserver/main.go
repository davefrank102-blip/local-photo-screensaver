// Command photoserver is a Phase 0 LAN photo server for the Local Photo Screensaver spike.
// Photos stay on the machine; only opaque IDs and image bytes are exposed on the LAN.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	serverName   = "Local Photo Spike"
	apiVersion   = 1
	playlistCap  = 50
	displaySecs  = 8
	defaultAddr  = "0.0.0.0:8787"
)

type photoMeta struct {
	Path   string
	Width  int
	Height int
	Ext    string // ".jpg" / ".jpeg" / ".png"
}

type server struct {
	id     string
	photos string
	mu     sync.RWMutex
	byID   map[string]photoMeta
}

func main() {
	addr := flag.String("addr", defaultAddr, "listen address (default binds all interfaces; prefer LAN-only firewall)")
	photos := flag.String("photos", "", "directory of JPEG/PNG photos to serve (required)")
	flag.Parse()

	if strings.TrimSpace(*photos) == "" {
		fmt.Fprintln(os.Stderr, "error: --photos is required")
		flag.Usage()
		os.Exit(2)
	}
	abs, err := filepath.Abs(*photos)
	if err != nil {
		log.Fatalf("photos path: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		log.Fatalf("--photos must be an existing directory: %s", abs)
	}

	s := &server{
		id:     newServerID(),
		photos: abs,
		byID:   make(map[string]photoMeta),
	}
	n, err := s.scan()
	if err != nil {
		log.Fatalf("scan: %v", err)
	}
	log.Printf("scanned %d JPEG/PNG under %s (cap %d in playlist)", n, abs, playlistCap)
	log.Printf("listening on http://%s  (LAN trust only — no auth in Phase 0)", *addr)
	log.Printf("WARNING: --addr defaults to %s; restrict to your LAN (firewall / bind carefully).", defaultAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/spike/playlist", s.handlePlaylist)
	mux.HandleFunc("/api/v1/images/", s.handleImage)

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func newServerID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "spike-local"
	}
	return "spike-" + hex.EncodeToString(b)
}

func (s *server) scan() (int, error) {
	count := 0
	err := filepath.WalkDir(s.photos, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return nil
		}
		rel, err := filepath.Rel(s.photos, path)
		if err != nil {
			return err
		}
		// Reject odd paths that could confuse id mapping
		if strings.Contains(rel, "..") {
			return nil
		}
		w, h, ok := probeImage(path)
		if !ok {
			log.Printf("skip unreadable image: %s", rel)
			return nil
		}
		id := opaqueID(rel)
		s.byID[id] = photoMeta{Path: path, Width: w, Height: h, Ext: ext}
		count++
		return nil
	})
	return count, err
}

func opaqueID(rel string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(rel)))
	return hex.EncodeToString(sum[:16]) // 32 hex chars — opaque, not a path
}

func probeImage(path string) (int, int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"status":      "ok",
		"serverId":    s.id,
		"serverName":  serverName,
		"apiVersions": []int{apiVersion},
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.byID))
	for id := range s.byID {
		ids = append(ids, id)
	}
	shuffle(ids)
	if len(ids) > playlistCap {
		ids = ids[:playlistCap]
	}

	items := make([]playlistItem, 0, len(ids))
	for _, id := range ids {
		m := s.byID[id]
		items = append(items, playlistItem{
			ID:       id,
			ImageURL: "/api/v1/images/" + id,
			Width:    m.Width,
			Height:   m.Height,
		})
	}
	writeJSON(w, map[string]any{
		"displaySeconds": displaySecs,
		"transition":     "fade",
		"items":          items,
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
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Defense in depth: ensure resolved path stays under photos root
	clean := filepath.Clean(meta.Path)
	if !underRoot(s.photos, clean) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	switch meta.Ext {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	default:
		w.Header().Set("Content-Type", "image/jpeg")
	}
	http.ServeFile(w, r, clean)
}

func underRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
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

func shuffle(ids []string) {
	n := len(ids)
	for i := n - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := i
		if err == nil {
			j = int(jBig.Int64())
		}
		ids[i], ids[j] = ids[j], ids[i]
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	if err := enc.Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}
