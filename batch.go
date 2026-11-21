package main

import (
	"errors"
	"os"
	"reflect"
	"slices"
	"time"
)

const batchSize = 10
const syncInterval = 10 * time.Minute

// Items and the journal share one atomic write. A save is visible to both
// windows and survives restart before the caller receives its acknowledgement.
type batchState struct {
	Snapshot
	Changes []Change
}

func (s Store) loadBatch() (b batchState, err error) {
	err = readJSON(s.path("batch.json"), &b)
	if !os.IsNotExist(err) {
		return
	}
	if err = readJSON(s.path("snapshot.json"), &b.Snapshot); err != nil && !os.IsNotExist(err) {
		return
	}
	if err = readJSON(s.path("pending.json"), &b.Changes); err != nil && !os.IsNotExist(err) {
		return
	}
	b.Pending = len(b.Changes) > 0
	if b.Pending {
		b.PendingCount = 1
	}
	b.overlay()
	return b, nil
}

func (b *batchState) overlay() {
	if b.Items == nil {
		b.Items = map[string]Record{}
	}
	for _, c := range b.Changes {
		if c.After == nil {
			delete(b.Items, c.ID)
		} else {
			b.Items[c.ID] = c.After
		}
	}
}

func (s Store) snapshot() (Snapshot, error) {
	b, e := s.loadBatch()
	return b.Snapshot, e
}

func (s Store) save(changes []Change) (v Snapshot, err error) {
	err = s.locked(func() error {
		b, e := s.loadBatch()
		if e != nil {
			return e
		}
		if b.URL == "" || b.Last == "" {
			return errors.New("저장소에 연결한 후 먼저 동기화하세요.")
		}
		if b.Message != "" {
			return errors.New(b.Message)
		}
		index := map[string]int{}
		for i, c := range b.Changes {
			index[c.ID] = i
		}
		changed := false
		for _, c := range changes {
			current := b.Items[c.ID]
			next, conflicts := merge(c, current)
			if len(conflicts) > 0 {
				return errors.New("다른 창에서 일정이 변경되었습니다. 다시 열어 주세요.")
			}
			if e = validateRecord(c.ID, next); e != nil {
				return e
			}
			if reflect.DeepEqual(current, next) {
				continue
			}
			changed = true
			if i, ok := index[c.ID]; ok {
				b.Changes[i].After = next
			} else {
				index[c.ID] = len(b.Changes)
				b.Changes = append(b.Changes, Change{c.ID, current, next})
			}
			if next == nil {
				delete(b.Items, c.ID)
			} else {
				b.Items[c.ID] = next
			}
		}
		if changed {
			b.Changes = slices.DeleteFunc(b.Changes, func(c Change) bool { return reflect.DeepEqual(c.Before, c.After) })
			b.Pending = len(b.Changes) > 0
			b.PendingCount++
			if !b.Pending {
				b.PendingCount = 0
			}
			if e = atomicJSON(s.path("batch.json"), b); e != nil {
				return e
			}
		}
		v = b.Snapshot
		return nil
	})
	return
}

func (b batchState) due(now time.Time) bool {
	if len(b.Conflicts) > 0 {
		return false
	}
	return !now.Before(b.NextSync) || (b.PendingCount >= batchSize && b.Message == "")
}

func (b *batchState) resolve(items map[string]Record, choices []Conflict, local bool) {
	for i, c := range b.Changes {
		remote := items[c.ID]
		_, fields := merge(c, remote)
		for _, f := range fields {
			if !slices.Contains(choices, f) {
				continue
			}
			if f.Field == "*" {
				b.Changes[i].Before = remote
				if !local {
					b.Changes[i].After = remote
				}
			} else {
				c.Before[f.Field] = f.Remote
				if !local {
					c.After[f.Field] = f.Remote
				}
			}
		}
	}
}

func (s Store) synchronize(automatic bool, now time.Time, choices []Conflict, local bool) (v Snapshot, err error) {
	err = s.locked(func() error {
		b, e := s.loadBatch()
		if e != nil {
			return e
		}
		v = b.Snapshot
		if automatic && !b.due(now) {
			return nil
		}
		if _, e = os.Stat(s.path("repo")); os.IsNotExist(e) {
			return nil
		} else if e != nil {
			return e
		}
		b.NextSync = now.Add(syncInterval)
		b.Message = ""
		work := func() error {
			if b.URL == "" {
				b.URL, e = git(s.path("repo"), "", "", "remote", "get-url", "origin")
				if e != nil {
					return e
				}
			}
			if e = s.pull(); e != nil {
				return e
			}
			items, e := s.items()
			if e != nil {
				return e
			}
			if len(b.Changes) > 0 {
				b.resolve(items, choices, local)
				// Persist conflict choices before publishing so an interrupted push is replayable.
				if len(choices) > 0 {
					if e = atomicJSON(s.path("batch.json"), b); e != nil {
						return e
					}
				}
				b.Conflicts, e = s.publish(b.Changes, items)
				if e != nil {
					return e
				}
				if len(b.Conflicts) > 0 {
					b.Items = items
					b.overlay()
					return errors.New("같은 일정에 변경이 겹쳤습니다.")
				}
				items, e = s.items()
				if e != nil {
					return e
				}
			}
			b.Items = items
			b.Pending = false
			b.PendingCount = 0
			b.Changes = nil
			b.Conflicts = nil
			b.Last = now.Format(time.RFC3339)
			return nil
		}
		if e = work(); e != nil {
			b.Message = e.Error()
		}
		v = b.Snapshot
		return atomicJSON(s.path("batch.json"), b)
	})
	return
}
