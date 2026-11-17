package collector

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"smtbench/internal/result"
	"smtbench/internal/summary"
)

type Meta struct {
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Hostname    string    `json:"hostname"`
	CPUModel    string    `json:"cpu_model"`
	OS          string    `json:"os"`
	Profile     string    `json:"profile"`
	ToolVersion string    `json:"tool_version"`
	StartedAt   time.Time `json:"started_at"`
	ReceivedAt  time.Time `json:"received_at"`
	Partial     bool      `json:"partial"`
	Headline    Headline  `json:"headline"`
	// Params are the settings the run was started with.
	Params result.Params `json:"params"`
	// Runs started by one command with several variants share GroupID.
	GroupID    string `json:"group_id,omitempty"`
	GroupIndex int    `json:"group_index,omitempty"`
	GroupTotal int    `json:"group_total,omitempty"`
}

type Headline struct {
	FullSinglePct  float64                   `json:"full_single_pct"`
	FullAlonePct   float64                   `json:"full_alone_pct"`
	FullPairPct    float64                   `json:"full_pair_pct"`
	HTPerThreadPct float64                   `json:"ht_per_thread_pct"`
	BaselineTPS    float64                   `json:"baseline_tps"`
	HasPair        bool                      `json:"has_pair"`
	HasPoints      bool                      `json:"has_points"`
	LagRatio       float64                   `json:"lag_ratio"`
	Ratings        map[string]summary.Rating `json:"ratings"`
	// Tick rate test, the key scenario (see summary.Stab.Key); nil without it.
	Stab *StabHeadline `json:"stab,omitempty"`
}

type StabHeadline struct {
	TickRate    int     `json:"tick_rate"`
	Scenario    string  `json:"scenario"`
	WorstSecPct float64 `json:"worst_sec_pct"`
	WorkP99Pct  float64 `json:"work_p99_pct"`
}

var downloads = map[string]bool{"smtbench-linux-amd64": true, "smtbench-windows-amd64.exe": true}

type server struct {
	st          *Store
	downloadDir string
	maxBody     int64

	mu    sync.Mutex
	cache map[string]summary.Summary
}

func NewHandler(st *Store, web fs.FS, downloadDir string, maxBody int64) http.Handler {
	s := &server{st: st, downloadDir: downloadDir, maxBody: maxBody, cache: map[string]summary.Summary{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/runs", s.postRun)
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok\n") })
	mux.HandleFunc("GET /download/{file}", s.download)
	mux.Handle("GET /", http.FileServerFS(web))
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *server) summary(id string, raw []byte) (summary.Summary, error) {
	s.mu.Lock()
	sum, ok := s.cache[id]
	s.mu.Unlock()
	if ok && sum.Version == summary.Version {
		return sum, nil
	}
	r, err := result.Decode(raw)
	if err != nil {
		return summary.Summary{}, err
	}
	sum = summary.Compute(r)
	s.mu.Lock()
	s.cache[id] = sum
	s.mu.Unlock()
	return sum, nil
}

func (s *server) postRun(w http.ResponseWriter, r *http.Request) {
	var body io.Reader = http.MaxBytesReader(w, r.Body, s.maxBody)
	if r.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "тело запроса не в формате gzip")
			return
		}
		body = io.LimitReader(zr, s.maxBody+1)
	}
	raw, err := io.ReadAll(body)
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || int64(len(raw)) > s.maxBody {
		writeErr(w, http.StatusRequestEntityTooLarge, "результат слишком большой")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, "не удалось прочитать тело запроса")
		return
	}
	var head struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		writeErr(w, http.StatusBadRequest, "тело запроса не JSON")
		return
	}
	if head.SchemaVersion > result.SchemaVersion {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("формат результата версии %d не поддерживается — обновите сервер", head.SchemaVersion))
		return
	}
	run, err := result.Decode(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.st.Put(r.Context(), run, raw)
	if err != nil {
		log.Printf("put %s: %v", run.RunID, err)
		writeErr(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	s.mu.Lock()
	delete(s.cache, run.RunID)
	s.mu.Unlock()
	code := http.StatusOK
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, map[string]string{"id": run.RunID, "url": "/#/run/" + run.RunID})
}

func (s *server) listRuns(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.list(r.Context())
	if err != nil {
		log.Printf("list: %v", err)
		writeErr(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	group := r.URL.Query().Get("group")
	out := make([]Meta, 0, len(rows))
	for _, x := range rows {
		var head struct {
			Params     result.Params `json:"params"`
			GroupID    string        `json:"group_id"`
			GroupIndex int           `json:"group_index"`
			GroupTotal int           `json:"group_total"`
		}
		if json.Unmarshal(x.raw, &head) == nil {
			x.Params, x.GroupID, x.GroupIndex, x.GroupTotal = head.Params, head.GroupID, head.GroupIndex, head.GroupTotal
		}
		if group != "" && x.GroupID != group {
			continue
		}
		if sum, err := s.summary(x.ID, x.raw); err == nil {
			x.Headline = Headline{
				FullSinglePct:  sum.FullSinglePct,
				FullAlonePct:   sum.FullAlonePct,
				FullPairPct:    sum.FullPairPct,
				HTPerThreadPct: sum.HTPerThreadPct,
				BaselineTPS:    sum.BaselineTPS,
				HasPair:        sum.HasPair,
				HasPoints:      len(sum.Cores) > 0,
				LagRatio:       sum.LagRatio,
				Ratings:        sum.Ratings,
			}
			if row, ok := sum.Stab.KeyRow(); ok {
				x.Headline.Stab = &StabHeadline{TickRate: sum.Stab.TickRate, Scenario: row.Name,
					WorstSecPct: row.WorstSecPct, WorkP99Pct: row.WorkP99Pct}
			}
		}
		out = append(out, x.Meta)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	raw, err := s.st.Get(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		writeErr(w, http.StatusNotFound, "прогон не найден")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	sum, err := s.summary(id, raw)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "повреждённый результат")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": json.RawMessage(raw), "summary": sum})
}

func (s *server) download(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !downloads[name] {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.downloadDir, name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	http.ServeFile(w, r, path)
}
