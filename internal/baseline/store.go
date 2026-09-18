package baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const fileName = "baseline.json"

type File struct {
	HostRtt  map[string]int `json:"hostRtt"`
	ProbedAt time.Time      `json:"probedAt"`
}

func Path(dataDir string) string {
	return filepath.Join(dataDir, fileName)
}

func Load(dataDir string) (File, error) {
	b, err := os.ReadFile(Path(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return File{HostRtt: map[string]int{}}, nil
		}
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, err
	}
	if f.HostRtt == nil {
		f.HostRtt = map[string]int{}
	}
	return f, nil
}

func Save(dataDir string, hostRtt map[string]int, probedAt time.Time) error {
	if probedAt.IsZero() {
		probedAt = time.Now().UTC()
	}
	f := File{HostRtt: hostRtt, ProbedAt: probedAt.UTC()}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(Path(dataDir), b, 0o644)
}

func (f File) Empty() bool {
	return len(f.HostRtt) == 0 || f.ProbedAt.IsZero()
}

func (f File) Age() time.Duration {
	if f.ProbedAt.IsZero() {
		return time.Duration(1<<63 - 1) // "infinite"
	}
	return time.Since(f.ProbedAt)
}
