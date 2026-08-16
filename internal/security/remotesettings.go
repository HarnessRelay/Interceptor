package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RemoteSettings are daemon access settings managed from the dashboard.
// RemoteAccessEnabled defaults to true to preserve existing reachability;
// turning it off blocks every non-host client (except /api/v1/health).
type RemoteSettings struct {
	RemoteAccessEnabled bool `json:"remote_access_enabled"`
}

type RemoteSettingsStore struct {
	mu   sync.Mutex
	path string
	set  RemoteSettings
}

func NewRemoteSettingsStore(configDir string) (*RemoteSettingsStore, error) {
	s := &RemoteSettingsStore{
		path: filepath.Join(configDir, "remote_settings.json"),
		set:  RemoteSettings{RemoteAccessEnabled: true},
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.set); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *RemoteSettingsStore) Get() RemoteSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set
}

func (s *RemoteSettingsStore) Set(next RemoteSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := saveJSONFile(s.path, next); err != nil {
		return err
	}
	s.set = next
	return nil
}

func saveJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// KnownClient is a user-assigned display name for a network client, keyed by
// MAC address when known (stable) or IP address otherwise.
type KnownClient struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	UpdatedAt int64  `json:"updated_at"`
}

type KnownClientStore struct {
	mu      sync.Mutex
	path    string
	clients map[string]KnownClient
}

func NewKnownClientStore(configDir string) (*KnownClientStore, error) {
	s := &KnownClientStore{
		path:    filepath.Join(configDir, "known_clients.json"),
		clients: make(map[string]KnownClient),
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list struct {
		Clients []KnownClient `json:"clients"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	for _, c := range list.Clients {
		s.clients[c.Key] = c
	}
	return s, nil
}

// Rename sets a display name for a client key; an empty name removes it.
func (s *KnownClientStore) Rename(key, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		delete(s.clients, key)
	} else {
		s.clients[key] = KnownClient{Key: key, Name: name, UpdatedAt: time.Now().Unix()}
	}
	out := struct {
		Clients []KnownClient `json:"clients"`
	}{Clients: make([]KnownClient, 0, len(s.clients))}
	for _, c := range s.clients {
		out.Clients = append(out.Clients, c)
	}
	return saveJSONFile(s.path, out)
}

func (s *KnownClientStore) Name(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clients[key].Name
}

func (s *KnownClientStore) List() []KnownClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]KnownClient, 0, len(s.clients))
	for _, c := range s.clients {
		out = append(out, c)
	}
	return out
}
