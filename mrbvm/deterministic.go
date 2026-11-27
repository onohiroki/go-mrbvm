package mrbvm

import "time"

// now is the VM clock. Deterministic mode uses a fixed epoch unless the host
// supplies an explicit clock; this keeps Time.now reproducible.
func (s *State) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	if s.deterministic {
		return time.Unix(0, 0).UTC()
	}
	return time.Now()
}

// objectID returns a stable identity for object_id in deterministic mode.
func (s *State) objectID(value Value) int64 {
	if !s.deterministic || value.obj == nil {
		return objectIdentityID(value)
	}
	s.objectIDsMu.Lock()
	defer s.objectIDsMu.Unlock()
	if id, ok := s.objectIDs[value.obj]; ok {
		return id
	}
	s.nextObjectID++
	s.objectIDs[value.obj] = s.nextObjectID
	return s.nextObjectID
}
