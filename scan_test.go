package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanSizes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("1234567890"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("does-not-exist", filepath.Join(dir, "dangling")); err != nil { // must not break the walk
		t.Fatal(err)
	}

	s := StartScan(dir)
	for !s.done() {
		time.Sleep(2 * time.Millisecond)
	}
	entries, done, _ := s.Snapshot()
	if !done {
		t.Fatal("scan never finished")
	}
	// Symlinks are listed but never followed: dangling shows as the link
	// itself (len("does-not-exist") = 14B) instead of breaking the walk.
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	// Sorted biggest first. sub/ = 5 bytes nested; a = 10; dangling = 14.
	want := []Entry{{Name: "dangling", Size: 14}, {Name: "a", Size: 10}, {Name: "sub", Size: 5}}
	for i, w := range want {
		e := entries[i]
		if e.Name != w.Name || e.Size != w.Size {
			t.Errorf("entry %d = %+v, want %+v", i, e, w)
		}
	}
}