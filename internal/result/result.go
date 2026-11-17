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
	// Stability is the tick rate test; nil when it was not run.
	Stability *Stability `json:"stability,omitempty"`
	Warnings  []string   `json:"warnings"`
	// Group links runs started by one command with several variants
	// (for example --profile source,minecraft --bg-threads 1,2).
	GroupID    string `json:"group_id,omitempty"`
	GroupIndex int    `json:"group_index,omitempty"` // 1-based
	GroupTotal int    `json:"group_total,omitempty"`
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
	// Tick rate test settings; zero in runs made before it existed.
	TickRate    int `json:"tick_rate,omitempty"`
	TickLoadPct int `json:"tick_load_pct,omitempty"`
	StabMS      int `json:"stab_ms,omitempty"`
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

// Stability is a game-server-like loop: every 1/TickRate seconds a server
// does WorkTicks profile ticks and sleeps until the next tick is due.
type Stability struct {
	TickRate    int            `json:"tick_rate"`
	TickLoadPct int            `json:"tick_load_pct"`
	WorkTicks   int            `json:"work_ticks"`   // profile ticks per server tick
	IdleWorkUs  float64        `json:"idle_work_us"` // one server tick on an idle machine
	CalibCPU    int            `json:"calib_cpu"`
	DurationMS  int            `json:"duration_ms"` // per scenario
	Scenarios   []StabScenario `json:"scenarios"`
}

type StabScenario struct {
	Name    string       `json:"name"`
	Busy    []int        `json:"busy"`   // CPUs under constant full load
	Bursty  []int        `json:"bursty"` // CPUs switching load on and off at random
	Servers []StabServer `json:"servers"`
}

// StabServer is one server loop pinned to CPU. Times are in microseconds.
type StabServer struct {
	CPU   int `json:"cpu"`
	Ticks int `json:"ticks"`
	// PerSec is the number of ticks started in each second of the scenario.
	PerSec []int `json:"per_sec"`
	// MaxWorkPerSec is the longest tick work in each second.
	MaxWorkPerSec []float64 `json:"max_work_per_sec"`
	WorkP50us     float64   `json:"work_p50_us"`
	WorkP99us     float64   `json:"work_p99_us"`
	WorkMaxUs     float64   `json:"work_max_us"`
	WorkMeanUs    float64   `json:"work_mean_us"`
	// Interval between tick starts.
	IntervalP99us float64 `json:"interval_p99_us"`
	IntervalSDus  float64 `json:"interval_sd_us"`
	// Overruns: ticks whose work took longer than the tick period.
	Overruns int `json:"overruns"`
	// Late: ticks that started more than 10% of the period after their slot.
	Late      int     `json:"late"`
	WakeP99us float64 `json:"wake_p99_us"`
	// NoisePct: share of the time the server was awake but other processes
	// had its CPU, in percent.
	NoisePct float64 `json:"noise_pct"`
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
