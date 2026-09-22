package main

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type Entry struct {
	Name  string
	Size  uint64
	IsDir bool
}

// Scan sizes the top-level entries of a directory in the background. The
// display reads Snapshot while Count reports how many files have been walked
// so far, so big trees show progress instead of freezing the UI.
type Scan struct {
	mu      sync.Mutex
	Entries []Entry
	Count   uint64
	Done    bool
}

func StartScan(path string) *Scan {
	s := &Scan{}
	go s.run(path)
	return s
}

func (s *Scan) run(path string) {
	ents, err := os.ReadDir(path)
	if err != nil {
		s.mu.Lock()
		s.Done = true
		s.mu.Unlock()
		return
	}
	for _, e := range ents {
		info, err := e.Info()
		if err != nil {
			continue // dangling symlink, vanished mid-walk
		}
		var sz uint64
		if info.IsDir() {
			sz = s.walk(filepath.Join(path, e.Name()))
		} else {
			sz = uint64(info.Size())
		}
		s.mu.Lock()
		s.Count++
		s.Entries = append(s.Entries, Entry{e.Name(), sz, info.IsDir()})
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.Done = true
	s.mu.Unlock()
}

// walk sums the size of everything under dir (files and nested dirs).
func (s *Scan) walk(dir string) (total uint64) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range ents {
		info, err := e.Info()
		if err != nil {
			continue
		}
		s.mu.Lock()
		s.Count++
		s.mu.Unlock()
		if info.IsDir() {
			total += s.walk(filepath.Join(dir, e.Name()))
		} else {
			total += uint64(info.Size())
		}
	}
	return total
}

// done reports whether the walk has finished.
func (s *Scan) done() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Done
}

// Snapshot returns a copy of the entries seen so far, biggest first.
func (s *Scan) Snapshot() ([]Entry, bool, uint64) {
	s.mu.Lock()
	cp := append([]Entry(nil), s.Entries...)
	done, count := s.Done, s.Count
	s.mu.Unlock()
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Size != cp[j].Size {
			return cp[i].Size > cp[j].Size
		}
		return cp[i].Name < cp[j].Name
	})
	return cp, done, count
}
