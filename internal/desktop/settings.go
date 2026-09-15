package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type settingsFile struct {
	LastRequest ScanRequest            `json:"lastRequest"`
	Theme       string                 `json:"theme"`
	Presets     map[string]ScanRequest `json:"presets"`
}

type settingsStore struct {
	mu   sync.Mutex
	path string
	data settingsFile
}

func newSettingsStore() *settingsStore {
	s := &settingsStore{
		path: filepath.Join(ConfigDir(), "settings.json"),
		data: settingsFile{Presets: map[string]ScanRequest{}},
	}
	_ = s.load()
	return s
}

func (s *settingsStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var data settingsFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if data.Presets == nil {
		data.Presets = map[string]ScanRequest{}
	}
	s.data = data
	return nil
}

func (s *settingsStore) save() error {
	if err := ensureDir(filepath.Dir(s.path)); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0644)
}

func (s *settingsStore) last() ScanRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.LastRequest
}

func (s *settingsStore) remember(req ScanRequest) {
	s.mu.Lock()
	s.data.LastRequest = req
	s.mu.Unlock()
	_ = s.save()
}

func (s *settingsStore) theme() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Theme
}

func (s *settingsStore) setTheme(theme string) {
	s.mu.Lock()
	s.data.Theme = theme
	s.mu.Unlock()
	_ = s.save()
}

func (s *settingsStore) listPresets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.data.Presets))
	for name := range s.data.Presets {
		names = append(names, name)
	}
	return names
}

func (s *settingsStore) savePreset(name string, req ScanRequest) {
	s.mu.Lock()
	if s.data.Presets == nil {
		s.data.Presets = map[string]ScanRequest{}
	}
	s.data.Presets[name] = req
	s.mu.Unlock()
	_ = s.save()
}

func (s *settingsStore) loadPreset(name string) (ScanRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	req, ok := s.data.Presets[name]
	return req, ok
}

func (s *settingsStore) deletePreset(name string) {
	s.mu.Lock()
	delete(s.data.Presets, name)
	s.mu.Unlock()
	_ = s.save()
}
