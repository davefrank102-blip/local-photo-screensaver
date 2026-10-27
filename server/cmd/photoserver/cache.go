package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Oriented-image disk cache management.
//
// Cache files live in <LOCALAPPDATA>\LocalPhotoScreensaver\orient-cache and are
// named <id>_<modNano>_<size>.jpg. Without management the folder grows forever
// (every edit / re-sync of a photo leaves the previous rendition behind), so:
//   - writes are atomic (temp file + rename)
//   - cache hits bump the file mtime so mtime ≈ last use (LRU)
//   - a size cap (default 1 GB) evicts least-recently-used files first; it is
//     enforced at startup, every few minutes, and after every N writes, always
//     off the request path
//   - at startup, entries for photos no longer in the library (or whose
//     modNano/size no longer match the source file) and orphan temp files are
//     deleted.

const (
	defaultCacheMaxMB    = 1024
	cacheMaxEnv          = "LPS_CACHE_MAX_MB"
	cacheTempPrefix      = ".tmp-"
	cacheEnforceInterval = 5 * time.Minute
	cacheEnforceEvery    = 50 // writes
	cacheTempMaxAge      = 10 * time.Minute
)

// defaultCacheMaxMBFromEnv returns LPS_CACHE_MAX_MB if set to a valid integer,
// otherwise 1024. A value of 0 (or negative) disables the size cap.
func defaultCacheMaxMBFromEnv() int64 {
	if v := strings.TrimSpace(os.Getenv(cacheMaxEnv)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
		log.Printf("ignoring invalid %s=%q (want integer MB)", cacheMaxEnv, v)
	}
	return defaultCacheMaxMB
}

func orientCacheDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if runtime.GOOS == "windows" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		} else {
			base, _ = os.UserHomeDir()
			base = filepath.Join(base, ".cache")
		}
	}
	return filepath.Join(base, "LocalPhotoScreensaver", "orient-cache")
}

type orientCache struct {
	dir      string
	maxBytes int64 // <= 0 means unlimited
	writes   atomic.Int64
	kick     chan struct{}
	mu       sync.Mutex // serialises sweeps (cleanup / eviction)
}

func newOrientCache(dir string, maxBytes int64) *orientCache {
	return &orientCache{dir: dir, maxBytes: maxBytes, kick: make(chan struct{}, 1)}
}

// path returns the cache file path for a source photo version.
func (c *orientCache) path(id string, modNano, size int64) string {
	return filepath.Join(c.dir, fmt.Sprintf("%s_%d_%d.jpg", id, modNano, size))
}

// touch marks a cache entry as recently used (best effort).
func (c *orientCache) touch(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// write stores data atomically at path, then occasionally triggers eviction.
func (c *orientCache) write(path string, data []byte) error {
	if err := writeFileAtomic(path, data); err != nil {
		return err
	}
	if c.writes.Add(1)%cacheEnforceEvery == 0 {
		c.trigger()
	}
	return nil
}

// trigger requests a background cap enforcement without blocking.
func (c *orientCache) trigger() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// run enforces the cap periodically and on trigger. Never returns.
func (c *orientCache) run() {
	t := time.NewTicker(cacheEnforceInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-c.kick:
		}
		c.mu.Lock()
		res := c.enforceCapLocked()
		c.mu.Unlock()
		if res.files > 0 {
			log.Printf("orient-cache: evicted %d file(s), freed %s (cap %s)", res.files, humanBytes(res.bytes), humanBytes(c.maxBytes))
		}
	}
}

type sweepResult struct {
	files int
	bytes int64
}

func (r *sweepResult) add(o sweepResult) { r.files += o.files; r.bytes += o.bytes }

type cacheEntry struct {
	path    string
	name    string
	size    int64
	modTime time.Time
}

// listLocked returns cache jpg entries and removes orphan temp files older than
// tempMaxAge (pass 0 to remove all temp files).
func (c *orientCache) listLocked(tempMaxAge time.Duration) ([]cacheEntry, sweepResult) {
	var res sweepResult
	des, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, res
	}
	now := time.Now()
	out := make([]cacheEntry, 0, len(des))
	for _, de := range des {
		if !de.Type().IsRegular() {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		name := de.Name()
		p := filepath.Join(c.dir, name)
		if strings.HasPrefix(name, cacheTempPrefix) {
			if now.Sub(info.ModTime()) >= tempMaxAge {
				if os.Remove(p) == nil {
					res.files++
					res.bytes += info.Size()
				}
			}
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".jpg") {
			continue
		}
		out = append(out, cacheEntry{path: p, name: name, size: info.Size(), modTime: info.ModTime()})
	}
	return out, res
}

// enforceCapLocked deletes least-recently-used entries until the cache is at
// or below maxBytes. Caller holds c.mu.
func (c *orientCache) enforceCapLocked() sweepResult {
	entries, res := c.listLocked(cacheTempMaxAge)
	if c.maxBytes <= 0 {
		return res
	}
	res.add(evictLRU(entries, c.maxBytes))
	return res
}

// evictLRU removes oldest-mtime entries until the total size is <= maxBytes.
func evictLRU(entries []cacheEntry, maxBytes int64) sweepResult {
	var res sweepResult
	var total int64
	for _, e := range entries {
		total += e.size
	}
	if total <= maxBytes {
		return res
	}
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].modTime.Equal(entries[j].modTime) {
			return entries[i].modTime.Before(entries[j].modTime)
		}
		return entries[i].name < entries[j].name
	})
	for _, e := range entries {
		if total <= maxBytes {
			break
		}
		if err := os.Remove(e.path); err != nil {
			continue // e.g. open by a concurrent reader on Windows; retry next sweep
		}
		total -= e.size
		res.files++
		res.bytes += e.size
	}
	return res
}

// parseCacheName splits "<id>_<modNano>_<size>.jpg".
func parseCacheName(name string) (id string, modNano, size int64, ok bool) {
	if !strings.EqualFold(filepath.Ext(name), ".jpg") {
		return "", 0, 0, false
	}
	parts := strings.Split(strings.TrimSuffix(name, filepath.Ext(name)), "_")
	if len(parts) != 3 || !isHexID(parts[0]) {
		return "", 0, 0, false
	}
	m, err1 := strconv.ParseInt(parts[1], 10, 64)
	s, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return "", 0, 0, false
	}
	return parts[0], m, s, true
}

// startupSweep removes orphan temp files and, when lib is non-nil, stale
// entries (photo gone from the library, or source modNano/size changed), then
// enforces the size cap. lib maps photo id -> source file path.
func (c *orientCache) startupSweep(lib map[string]string) (stale, temp, evicted sweepResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, temp := c.listLocked(0)
	if lib != nil {
		type ver struct {
			mod, size int64
			ok        bool
		}
		current := map[string]ver{}
		kept := entries[:0]
		for _, e := range entries {
			id, mod, size, ok := parseCacheName(e.name)
			if !ok {
				kept = append(kept, e)
				continue
			}
			src, inLib := lib[id]
			remove := !inLib
			if inLib {
				v, seen := current[id]
				if !seen {
					if info, err := os.Stat(src); err == nil {
						v = ver{info.ModTime().UnixNano(), info.Size(), true}
					}
					current[id] = v
				}
				// Only treat as stale when the source could be stat'ed; an
				// unreadable source might be a transient (offline drive).
				remove = v.ok && (v.mod != mod || v.size != size)
			}
			if remove {
				if os.Remove(e.path) == nil {
					stale.files++
					stale.bytes += e.size
					continue
				}
			}
			kept = append(kept, e)
		}
		entries = kept
	}
	if c.maxBytes > 0 {
		evicted = evictLRU(entries, c.maxBytes)
	}
	return stale, temp, evicted
}

// writeFileAtomic writes data to a temp file in the same directory and renames
// it into place so readers never see a partial file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, cacheTempPrefix+"*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		_ = os.Chmod(tmp, 0o644) // CreateTemp uses 0600; keep the old 0644 mode
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return werr
	}
	return nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
