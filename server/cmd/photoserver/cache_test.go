package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testID = "0123456789abcdef0123456789abcdef"
const testID2 = "fedcba9876543210fedcba9876543210"

func writeAged(t *testing.T, path string, size int, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Add(-age)
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestEnforceCapEvictsLRU(t *testing.T) {
	dir := t.TempDir()
	c := newOrientCache(dir, 250)
	oldest := c.path(testID, 1, 1)
	middle := c.path(testID, 2, 2)
	newest := c.path(testID, 3, 3)
	writeAged(t, oldest, 100, 3*time.Hour)
	writeAged(t, middle, 100, 2*time.Hour)
	writeAged(t, newest, 100, 1*time.Hour)

	// A cache hit on the oldest file makes it most-recently used.
	c.touch(oldest)

	c.mu.Lock()
	res := c.enforceCapLocked()
	c.mu.Unlock()
	if res.files != 1 || res.bytes != 100 {
		t.Fatalf("evicted %d files / %d bytes, want 1 / 100", res.files, res.bytes)
	}
	if exists(middle) {
		t.Fatal("least-recently-used file should have been evicted")
	}
	if !exists(oldest) || !exists(newest) {
		t.Fatal("recently used files should be kept")
	}
}

func TestEnforceCapUnlimited(t *testing.T) {
	dir := t.TempDir()
	c := newOrientCache(dir, 0)
	writeAged(t, c.path(testID, 1, 1), 1000, time.Hour)
	c.mu.Lock()
	res := c.enforceCapLocked()
	c.mu.Unlock()
	if res.files != 0 {
		t.Fatalf("unlimited cache evicted %d files", res.files)
	}
}

func TestStartupSweepStaleAndTemp(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(src, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(src)
	c := newOrientCache(dir, 1<<30)
	current := c.path(testID, info.ModTime().UnixNano(), info.Size())
	staleVer := c.path(testID, info.ModTime().UnixNano()-1, info.Size())
	gone := c.path(testID2, 5, 5)
	tmp := filepath.Join(dir, cacheTempPrefix+"123")
	other := filepath.Join(dir, "notes.txt")
	for _, p := range []string{current, staleVer, gone, tmp, other} {
		writeAged(t, p, 10, time.Minute)
	}

	stale, temp, evicted := c.startupSweep(map[string]string{testID: src})
	if stale.files != 2 || temp.files != 1 || evicted.files != 0 {
		t.Fatalf("stale=%d temp=%d evicted=%d, want 2/1/0", stale.files, temp.files, evicted.files)
	}
	if !exists(current) || !exists(other) {
		t.Fatal("current entry / unrelated file must be kept")
	}
	if exists(staleVer) || exists(gone) || exists(tmp) {
		t.Fatal("stale entries and temp files must be removed")
	}

	// Without an index, nothing is treated as stale.
	writeAged(t, gone, 10, time.Minute)
	stale, _, _ = c.startupSweep(nil)
	if stale.files != 0 || !exists(gone) {
		t.Fatal("stale cleanup must be skipped without a library index")
	}
}

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	c := newOrientCache(filepath.Join(dir, "sub"), 0)
	p := c.path(testID, 1, 1)
	if err := c.write(p, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := c.write(p, []byte("world")); err != nil { // overwrite existing
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "world" {
		t.Fatalf("got %q, %v", b, err)
	}
	des, _ := os.ReadDir(filepath.Dir(p))
	if len(des) != 1 {
		t.Fatalf("expected only the final file, found %d entries", len(des))
	}
}

func TestParseCacheName(t *testing.T) {
	id, m, s, ok := parseCacheName(testID + "_1700000000000000000_12345.jpg")
	if !ok || id != testID || m != 1700000000000000000 || s != 12345 {
		t.Fatalf("parse failed: %v %v %v %v", id, m, s, ok)
	}
	for _, bad := range []string{"x_1_2.jpg", testID + "_1.jpg", testID + "_a_2.jpg", testID + "_1_2.png"} {
		if _, _, _, ok := parseCacheName(bad); ok {
			t.Fatalf("should not parse %q", bad)
		}
	}
}
