package profiles

import (
	"encoding/json"
	"embed"
	"fmt"
	"sort"
	"sync"

	"github.com/potatoinspector/potato-inspector/internal/store"
)

//go:embed data/*.json
var builtinFS embed.FS

type Profile struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	DelayMs      int     `json:"delayMs"`
	DownloadMbps float64 `json:"downloadMbps"`
	UploadMbps   float64 `json:"uploadMbps"`
	LossPercent  float64 `json:"lossPercent"`
	Passthrough  bool    `json:"passthrough"`
	Builtin      bool    `json:"builtin"`
}

type Registry struct {
	mu       sync.RWMutex
	builtins map[string]Profile
	customs  map[string]Profile
	store    *store.Store
}

func NewRegistry(st *store.Store) (*Registry, error) {
	r := &Registry{
		builtins: make(map[string]Profile),
		customs:  make(map[string]Profile),
		store:    st,
	}
	entries, err := builtinFS.ReadDir("data")
	if err != nil {
		return nil, fmt.Errorf("read builtin profiles: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := builtinFS.ReadFile("data/" + e.Name())
		if err != nil {
			return nil, err
		}
		var p Profile
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		p.Builtin = true
		r.builtins[p.ID] = p
	}
	if err := r.ReloadCustoms(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Registry) ReloadCustoms() error {
	list, err := r.store.LoadCustomProfiles()
	if err != nil {
		return err
	}
	customs := make(map[string]Profile)
	for _, m := range list {
		data, _ := json.Marshal(m)
		var p Profile
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}
		p.Builtin = false
		if p.ID == "" {
			continue
		}
		customs[p.ID] = p
	}
	r.mu.Lock()
	r.customs = customs
	r.mu.Unlock()
	return nil
}

func (r *Registry) Get(id string) (Profile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.builtins[id]; ok {
		return p, true
	}
	p, ok := r.customs[id]
	return p, ok
}

func (r *Registry) List() []Profile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Profile, 0, len(r.builtins)+len(r.customs))
	for _, p := range r.builtins {
		out = append(out, p)
	}
	for _, p := range r.customs {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Builtin != out[j].Builtin {
			return out[i].Builtin
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) SaveCustom(p Profile) error {
	if p.ID == "" {
		return fmt.Errorf("profile id required")
	}
	r.mu.RLock()
	_, isBuiltin := r.builtins[p.ID]
	r.mu.RUnlock()
	if isBuiltin {
		return fmt.Errorf("cannot overwrite builtin profile %s", p.ID)
	}
	p.Builtin = false
	r.mu.Lock()
	r.customs[p.ID] = p
	list := make([]map[string]any, 0, len(r.customs))
	for _, c := range r.customs {
		data, _ := json.Marshal(c)
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		list = append(list, m)
	}
	r.mu.Unlock()
	return r.store.SaveCustomProfiles(list)
}

func (r *Registry) DeleteCustom(id string) error {
	r.mu.Lock()
	delete(r.customs, id)
	list := make([]map[string]any, 0, len(r.customs))
	for _, c := range r.customs {
		data, _ := json.Marshal(c)
		var m map[string]any
		_ = json.Unmarshal(data, &m)
		list = append(list, m)
	}
	r.mu.Unlock()
	return r.store.SaveCustomProfiles(list)
}
