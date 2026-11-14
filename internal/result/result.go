// Package result defines the JSON format of a benchmark run.
package result

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const SchemaVersion = 1

type Run struct {
	SchemaVersion  int       `json:"schema_version"`
	KernelsVersion int       `json:"kernels_version"`
	ToolVersion    string    `json:"tool_version"`
	RunID          string    `json:"run_id"`
	Label          string    `json:"label"`
	Hostname       string    `json:"hostname"`
	OS             string    `json:"os"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
	Partial        bool      `json:"partial"`
	CPU            CPUInfo   `json:"cpu"`
	Params         Params    `json:"params"`
	Points         []Point   `json:"points"`
	Warnings       []string  `json:"warnings"`
}

type CPUInfo struct {
	Model      string       `json:"model"`
	Vendor     string       `json:"vendor"`
	Features   []string     `json:"features"`
	FreqSource string       `json:"freq_source"`
	Logical    []LogicalCPU `json:"logical"`
	Cores      []CoreInfo   `json:"cores"`
	L3Groups   []L3Group    `json:"l3_groups"`
	Packages   int          `json:"packages"`
}

type LogicalCPU struct {
	ID      int `json:"id"`
	Core    int `json:"core"`
	Package int `json:"package"`
	NUMA    int `json:"numa"`
	L3Group int `json:"l3_group"`
}

type CoreInfo struct {
	ID      int   `json:"id"`
	Package int   `json:"package"`
	L3Group int   `json:"l3_group"`
	Threads []int `json:"threads"`
}

type L3Group struct {
	ID      int   `json:"id"`
	SizeKB  int   `json:"size_kb"`
	Package int   `json:"package"`
	Cores   []int `json:"cores"`
	Big     bool  `json:"big"`
}

type Params struct {
	Profile     string         `json:"profile"`
	Mix         map[string]int `json:"mix"`
	MemSizeMB   int            `json:"mem_size_mb"`
	DurationMS  int            `json:"duration_ms"`
	WarmupMS    int            `json:"warmup_ms"`
	Reps        int            `json:"reps"`
	Step        int            `json:"step"`
	Order       string         `json:"order"`
	BgThreads   int            `json:"bg_threads"`
	Modes       []string       `json:"modes"`
	Cores       []int          `json:"cores"`
	NoiseMaxPct float64        `json:"noise_max_pct"`
}

type Point struct {
	Core        int            `json:"core"`
	Mode        string         `json:"mode"`
	LoadedCores int            `json:"loaded_cores"`
	LoadedCPUs  []int          `json:"loaded_cpus"`
	OutsideL3   int            `json:"outside_l3"`
	Threads     []ThreadResult `json:"threads"`
	NoisePct    float64        `json:"noise_pct"`
	Retries     int            `json:"retries"`
	Noisy       bool           `json:"noisy"`
}

type ThreadResult struct {
	CPU         int     `json:"cpu"`
	TicksPerSec float64 `json:"ticks_per_sec"`
	TickP50us   float64 `json:"tick_p50_us"`
	TickP99us   float64 `json:"tick_p99_us"`
	TickP999us  float64 `json:"tick_p999_us"`
	MHz         float64 `json:"mhz"`
}

var validModes = map[string]bool{"single": true, "solo_b": true, "pair": true, "final": true}

func NewRunID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (r *Run) Validate() error {
	if len(r.RunID) != 32 {
		return errors.New("run_id должен быть из 32 шестнадцатеричных символов")
	}
	if _, err := hex.DecodeString(r.RunID); err != nil {
		return errors.New("run_id должен быть из 32 шестнадцатеричных символов")
	}
	if r.SchemaVersion < 1 || r.SchemaVersion > SchemaVersion {
		return fmt.Errorf("версия формата %d не поддерживается", r.SchemaVersion)
	}
	if len(r.CPU.Logical) == 0 {
		return errors.New("нет данных о процессоре")
	}
	for i, p := range r.Points {
		if !validModes[p.Mode] {
			return fmt.Errorf("точка %d: неизвестный режим %q", i, p.Mode)
		}
	}
	return nil
}

func Decode(b []byte) (*Run, error) {
	var r Run
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("не удалось разобрать JSON: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

func Save(path string, r *Run) error {
	b, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".smtbench-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func Load(path string) (*Run, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(b)
}
