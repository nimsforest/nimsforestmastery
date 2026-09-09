// Package soil reads the forest state from the Soil bucket, a NATS
// JetStream key-value store. The portal is a pure reader: it never
// creates the bucket and never writes a key.
package soil

import (
	"errors"
	"sort"
	"strings"

	"github.com/nats-io/nats.go"
)

// Bucket is the Soil bucket name used everywhere in NimsForest.
const Bucket = "SOIL"

var (
	// ErrNotFound reports that a key does not exist.
	ErrNotFound = errors.New("soil: key not found")

	// ErrDisabled reports that the service runs without a Soil
	// connection. Callers render this state honestly instead of
	// serving stale content.
	ErrDisabled = errors.New("soil: disabled, no NATS connection")
)

// Entry is one Soil key with its value and revision. The revision is
// the anti-drift stamp: every rendering carries the revision of each
// source key.
type Entry struct {
	Key      string
	Value    []byte
	Revision uint64
}

// Reader is the small read surface the domain layer needs.
type Reader interface {
	// Get returns one entry. It returns ErrNotFound for a missing
	// key and ErrDisabled when Soil is not connected.
	Get(key string) (Entry, error)

	// List returns all entries whose key starts with prefix, sorted
	// by key. It returns ErrDisabled when Soil is not connected.
	List(prefix string) ([]Entry, error)

	// Disabled reports whether this reader runs without Soil.
	Disabled() bool
}

// Watcher delivers key change notifications. The domain layer uses
// them to invalidate its render cache.
type Watcher interface {
	// Watch calls onChange with each changed key that matches the
	// NATS wildcard pattern. The initial replay of existing keys is
	// not reported. The returned function stops the watch.
	Watch(pattern string, onChange func(key string)) (stop func(), err error)
}

// KV reads the Soil bucket over a live NATS connection.
type KV struct {
	kv nats.KeyValue
}

// Open binds the existing Soil bucket on the given connection. It
// fails when JetStream or the bucket is absent; it never creates the
// bucket.
func Open(nc *nats.Conn) (*KV, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	kv, err := js.KeyValue(Bucket)
	if err != nil {
		return nil, err
	}
	return &KV{kv: kv}, nil
}

// Get implements Reader.
func (s *KV) Get(key string) (Entry, error) {
	entry, err := s.kv.Get(key)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, err
	}
	return Entry{Key: entry.Key(), Value: entry.Value(), Revision: entry.Revision()}, nil
}

// List implements Reader.
func (s *KV) List(prefix string) ([]Entry, error) {
	keys, err := s.kv.Keys()
	if err != nil {
		if errors.Is(err, nats.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, err
	}
	var entries []Entry
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		entry, err := s.Get(key)
		if errors.Is(err, ErrNotFound) {
			continue // deleted between Keys and Get
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries, nil
}

// Disabled implements Reader.
func (s *KV) Disabled() bool { return false }

// Watch implements Watcher. Deletes and purges also fire onChange:
// a deleted nim must disappear from the rendered role list.
func (s *KV) Watch(pattern string, onChange func(key string)) (func(), error) {
	if pattern == "" {
		pattern = ">"
	}
	watcher, err := s.kv.Watch(pattern)
	if err != nil {
		return nil, err
	}
	go func() {
		replayDone := false
		for entry := range watcher.Updates() {
			if entry == nil {
				// nil marks the end of the initial replay.
				replayDone = true
				continue
			}
			if !replayDone {
				continue
			}
			onChange(entry.Key())
		}
	}()
	return func() { _ = watcher.Stop() }, nil
}

// Disabled is the Reader for a service that runs without NATS. Every
// read returns ErrDisabled so the pages can say so.
type Disabled struct {
	// Reason says why Soil is off, for logs and health output.
	Reason string
}

// Get implements Reader.
func (Disabled) Get(string) (Entry, error) { return Entry{}, ErrDisabled }

// List implements Reader.
func (Disabled) List(string) ([]Entry, error) { return nil, ErrDisabled }

// Disabled implements Reader.
func (Disabled) Disabled() bool { return true }
