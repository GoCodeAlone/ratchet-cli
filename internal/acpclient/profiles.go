package acpclient

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var (
	ErrProfileNotFound       = errors.New("acp client profile not found")
	ErrProfileShadowsBuiltin = errors.New("acp client profile shadows built-in agent")
)

type Profile struct {
	Name          string    `json:"name"`
	Spec          AgentSpec `json:"spec"`
	Cwd           string    `json:"cwd,omitempty"`
	SourceKind    string    `json:"sourceKind,omitempty"`
	SourceID      string    `json:"sourceId,omitempty"`
	Hash          string    `json:"hash"`
	Trusted       bool      `json:"trusted,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	PluginName    string    `json:"pluginName,omitempty"`
	PluginVersion string    `json:"pluginVersion,omitempty"`
}

type ProfileStore struct {
	path string
}

type profileFile struct {
	Profiles []Profile `json:"profiles"`
}

func NewDefaultProfileStore() (*ProfileStore, error) {
	store, err := NewDefaultStore()
	if err != nil {
		return nil, err
	}
	return NewProfileStore(filepath.Join(filepath.Dir(store.Path()), "profiles.json")), nil
}

func NewProfileStore(path string) *ProfileStore {
	return &ProfileStore{path: path}
}

func (s *ProfileStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *ProfileStore) List() ([]Profile, error) {
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	profiles := slices.Clone(data.Profiles)
	normalizeProfiles(profiles)
	slices.SortFunc(profiles, func(a, b Profile) int {
		return strings.Compare(a.Name, b.Name)
	})
	return profiles, nil
}

func (s *ProfileStore) Get(name string) (Profile, error) {
	profiles, err := s.List()
	if err != nil {
		return Profile{}, err
	}
	for _, profile := range profiles {
		if profile.Name == name {
			return profile, nil
		}
	}
	return Profile{}, fmt.Errorf("%w: %s", ErrProfileNotFound, name)
}

func (s *ProfileStore) Add(profile Profile) error {
	profile.Name = strings.TrimSpace(profile.Name)
	if profile.Name == "" {
		return errors.New("acp client profile name is required")
	}
	if profile.Spec.Name == "" {
		profile.Spec.Name = profile.Name
	}
	if err := profile.Spec.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if profile.CreatedAt.IsZero() {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	profile.Hash = profile.DescriptorHash()
	data, err := s.load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range data.Profiles {
		if data.Profiles[i].Name == profile.Name {
			if !data.Profiles[i].CreatedAt.IsZero() {
				profile.CreatedAt = data.Profiles[i].CreatedAt
			}
			data.Profiles[i] = profile
			replaced = true
			break
		}
	}
	if !replaced {
		data.Profiles = append(data.Profiles, profile)
	}
	return s.save(data)
}

func (s *ProfileStore) Trust(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("acp client profile name is required")
	}
	data, err := s.load()
	if err != nil {
		return err
	}
	for i := range data.Profiles {
		if data.Profiles[i].Name == name {
			data.Profiles[i].Hash = data.Profiles[i].DescriptorHash()
			data.Profiles[i].Trusted = true
			data.Profiles[i].UpdatedAt = time.Now().UTC()
			return s.save(data)
		}
	}
	return fmt.Errorf("%w: %s", ErrProfileNotFound, name)
}

func (s *ProfileStore) Remove(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("acp client profile name is required")
	}
	data, err := s.load()
	if err != nil {
		return err
	}
	next := data.Profiles[:0]
	removed := false
	for _, profile := range data.Profiles {
		if profile.Name == name {
			removed = true
			continue
		}
		next = append(next, profile)
	}
	if !removed {
		return fmt.Errorf("%w: %s", ErrProfileNotFound, name)
	}
	data.Profiles = next
	return s.save(data)
}

func (p Profile) DescriptorHash() string {
	args := slices.Clone(p.Spec.Args)
	envKeys := slices.Clone(p.Spec.EnvKeys)
	slices.Sort(envKeys)
	payload := struct {
		Name    string   `json:"name"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
		EnvKeys []string `json:"envKeys"`
		Cwd     string   `json:"cwd,omitempty"`
	}{
		Name:    strings.TrimSpace(p.Name),
		Command: strings.TrimSpace(p.Spec.Command),
		Args:    args,
		EnvKeys: envKeys,
		Cwd:     strings.TrimSpace(p.Cwd),
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (p Profile) TrustValid() bool {
	if !p.Trusted {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(p.Hash), []byte(p.DescriptorHash())) == 1
}

func (s *ProfileStore) load() (profileFile, error) {
	var data profileFile
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return data, nil
	}
	if err != nil {
		return profileFile{}, err
	}
	if len(b) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return profileFile{}, err
	}
	normalizeProfiles(data.Profiles)
	return data, nil
}

func (s *ProfileStore) save(data profileFile) error {
	normalizeProfiles(data.Profiles)
	slices.SortFunc(data.Profiles, func(a, b Profile) int {
		return strings.Compare(a.Name, b.Name)
	})
	return writeJSONFileAtomic(s.path, data, 0o600)
}

func normalizeProfiles(profiles []Profile) {
	for i := range profiles {
		if profiles[i].Spec.Name == "" {
			profiles[i].Spec.Name = profiles[i].Name
		}
		profiles[i].Spec.Args = slices.Clone(profiles[i].Spec.Args)
		profiles[i].Spec.EnvKeys = slices.Clone(profiles[i].Spec.EnvKeys)
		if profiles[i].Hash == "" && !profiles[i].Trusted {
			profiles[i].Hash = profiles[i].DescriptorHash()
		}
	}
}
