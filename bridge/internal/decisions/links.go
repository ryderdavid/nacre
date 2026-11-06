package decisions

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// maxLinks bounds the work-to-chat links kept.
const maxLinks = 2000

// LinkWork records that a chat started a round or run, so its work card can
// name the chat. Links survive restarts.
func (s *Store) LinkWork(workID, chatID string) {
	if workID == "" || chatID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	links := s.linksLocked()
	if links[workID] == chatID {
		return
	}
	if len(links) >= maxLinks {
		for k := range links {
			delete(links, k)
			if len(links) < maxLinks/2 {
				break
			}
		}
	}
	links[workID] = chatID
	data, err := json.Marshal(links)
	if err == nil {
		tmp := filepath.Join(s.dir, ".links.tmp")
		if err = os.WriteFile(tmp, data, 0o600); err == nil {
			err = os.Rename(tmp, filepath.Join(s.dir, "links.json"))
		}
	}
	if err != nil {
		s.log.Printf("decisions: save work links: %v", err)
	}
}

// ChatOf is the chat that started a round or run, or "".
func (s *Store) ChatOf(workID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.linksLocked()[workID]
}

func (s *Store) linksLocked() map[string]string {
	if s.links == nil {
		s.links = map[string]string{}
		if data, err := os.ReadFile(filepath.Join(s.dir, "links.json")); err == nil {
			_ = json.Unmarshal(data, &s.links)
		}
	}
	return s.links
}
