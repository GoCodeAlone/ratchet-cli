package hooks

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// TrustStore persists explicit hook trust and disable decisions by descriptor
// hash. Disabled hashes always win over trusted hashes.
type TrustStore struct {
	path     string
	Trusted  map[string]bool `json:"trusted,omitempty"`
	Disabled map[string]bool `json:"disabled,omitempty"`
}

// LoadTrustStore loads hook trust state from path, creating an empty in-memory
// store when the file does not exist.
func LoadTrustStore(path string) (*TrustStore, error) {
	store := &TrustStore{
		path:     path,
		Trusted:  make(map[string]bool),
		Disabled: make(map[string]bool),
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, err
	}
	store.path = path
	if store.Trusted == nil {
		store.Trusted = make(map[string]bool)
	}
	if store.Disabled == nil {
		store.Disabled = make(map[string]bool)
	}
	return store, nil
}

// Trust records hash as trusted and removes any disabled marker.
func (s *TrustStore) Trust(hash string) error {
	if hash == "" {
		return nil
	}
	s.ensureMaps()
	s.Trusted[hash] = true
	delete(s.Disabled, hash)
	return s.save()
}

// Untrust removes explicit trust without enabling a disabled hook.
func (s *TrustStore) Untrust(hash string) error {
	if hash == "" {
		return nil
	}
	s.ensureMaps()
	delete(s.Trusted, hash)
	return s.save()
}

// Disable records hash as disabled and removes any explicit trust.
func (s *TrustStore) Disable(hash string) error {
	if hash == "" {
		return nil
	}
	s.ensureMaps()
	delete(s.Trusted, hash)
	s.Disabled[hash] = true
	return s.save()
}

func (s *TrustStore) IsTrusted(hash string) bool {
	if s == nil || hash == "" {
		return false
	}
	return s.Trusted[hash] && !s.Disabled[hash]
}

func (s *TrustStore) IsDisabled(hash string) bool {
	if s == nil || hash == "" {
		return false
	}
	return s.Disabled[hash]
}

func (s *TrustStore) ensureMaps() {
	if s.Trusted == nil {
		s.Trusted = make(map[string]bool)
	}
	if s.Disabled == nil {
		s.Disabled = make(map[string]bool)
	}
}

func (s *TrustStore) save() error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(data, '\n'), 0o600)
}
