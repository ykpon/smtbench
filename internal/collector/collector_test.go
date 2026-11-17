package collector

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"smtbench/internal/result"
	"smtbench/internal/summary"
)

func fixture(t *testing.T) []byte {
	b, err := os.ReadFile("testdata/xeon-quick.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// variant returns the fixture with a new run id and start time.
func variant(t *testing.T, id string, started time.Time) []byte {
	r, err := result.Decode(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	r.RunID, r.StartedAt = id, started
	b, _ := json.Marshal(r)
	return b
}

func newServer(t *testing.T, maxBody int64) (*httptest.Server, string) {
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	dl := filepath.Join(dir, "dl")
	os.MkdirAll(dl, 0o755)
	os.WriteFile(filepath.Join(dl, "smtbench-linux-amd64"), []byte("binary"), 0o644)
	web := fstest.MapFS{"index.html": {Data: []byte("<html>ok</html>")}}
	srv := httptest.NewServer(NewHandler(st, web, dl, maxBody))
	t.Cleanup(srv.Close)
	return srv, dir
}

func postGzip(t *testing.T, url string, body []byte) (*http.Response, map[string]string) {
	var z bytes.Buffer
	zw := gzip.NewWriter(&z)
	zw.Write(body)
	zw.Close()
	req, _ := http.NewRequest("POST", url+"/api/runs", &z)
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	m := map[string]string{}
	json.NewDecoder(resp.Body).Decode(&m)
	return resp, m
}

func getJSON(t *testing.T, url string, v any) int {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if v != nil {
		json.NewDecoder(resp.Body).Decode(v)
	}
	return resp.StatusCode
}

func TestPostAndGet(t *testing.T) {
	srv, _ := newServer(t, 32<<20)
	resp, m := postGzip(t, srv.URL, fixture(t))
	if resp.StatusCode != 201 || m["id"] == "" || m["url"] != "/#/run/"+m["id"] {
		t.Fatalf("%d %v", resp.StatusCode, m)
	}
	var got struct {
		Run     result.Run `json:"run"`
		Summary struct {
			Version int `json:"version"`
		} `json:"summary"`
	}
	if code := getJSON(t, srv.URL+"/api/runs/"+m["id"], &got); code != 200 {
		t.Fatalf("code %d", code)
	}
	if got.Run.RunID != m["id"] || got.Summary.Version != summary.Version {
		t.Fatalf("%+v", got)
	}
}

func TestPostIdempotent(t *testing.T) {
	srv, _ := newServer(t, 32<<20)
	r1, _ := postGzip(t, srv.URL, fixture(t))
	r2, _ := postGzip(t, srv.URL, fixture(t))
	if r1.StatusCode != 201 || r2.StatusCode != 200 {
		t.Fatalf("%d %d", r1.StatusCode, r2.StatusCode)
	}
	var list []Meta
	getJSON(t, srv.URL+"/api/runs", &list)
	if len(list) != 1 {
		t.Fatalf("list %d", len(list))
	}
}

func TestPostInvalid(t *testing.T) {
	srv, _ := newServer(t, 32<<20)
	resp, m := postGzip(t, srv.URL, []byte("{}"))
	if resp.StatusCode != 400 || m["error"] == "" {
		t.Fatalf("%d %v", resp.StatusCode, m)
	}
	b := bytes.Replace(fixture(t), []byte(`"schema_version": 1`), []byte(`"schema_version": 99`), 1)
	resp, m = postGzip(t, srv.URL, b)
	if resp.StatusCode != 400 || !strings.Contains(m["error"], "обновите сервер") {
		t.Fatalf("%d %v", resp.StatusCode, m)
	}
	resp, _ = postGzip(t, srv.URL, []byte("not json"))
	if resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestPostPlainJSON(t *testing.T) {
	srv, _ := newServer(t, 32<<20)
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(fixture(t)))
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("%v %v", resp.StatusCode, err)
	}
}

func TestPostTooLarge(t *testing.T) {
	srv, _ := newServer(t, 1024)
	resp, err := http.Post(srv.URL+"/api/runs", "application/json", bytes.NewReader(fixture(t)))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 413 {
		t.Fatalf("%d", resp.StatusCode)
	}
}

func TestListOrderAndHeadline(t *testing.T) {
	srv, _ := newServer(t, 32<<20)
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	postGzip(t, srv.URL, variant(t, strings.Repeat("1", 32), old))
	postGzip(t, srv.URL, variant(t, strings.Repeat("2", 32), old.Add(time.Hour)))
	var list []Meta
	getJSON(t, srv.URL+"/api/runs", &list)
	if len(list) != 2 || list[0].ID != strings.Repeat("2", 32) {
		t.Fatalf("%+v", list)
	}
	h := list[0].Headline
	if h.FullSinglePct <= 0 || !h.HasPair || h.Ratings["full_single"] == "" || list[0].CPUModel == "" || list[0].Profile != "minecraft" {
		t.Fatalf("%+v", list[0])
	}
	if h.FullAlonePct <= 0 {
		t.Fatalf("headline %+v", h)
	}
	if p := list[0].Params; p.Profile != "minecraft" || p.DurationMS <= 0 || len(p.Cores) != 2 || p.Mix["mem"] != 45 {
		t.Fatalf("params %+v", p)
	}
}

func TestGetMissing(t *testing.T) {
	srv, _ := newServer(t, 32<<20)
	if code := getJSON(t, srv.URL+"/api/runs/"+strings.Repeat("f", 32), nil); code != 404 {
		t.Fatalf("%d", code)
	}
}

func TestDownloadWhitelist(t *testing.T) {
	srv, _ := newServer(t, 32<<20)
	for path, want := range map[string]int{
		"/download/smtbench-linux-amd64":       200,
		"/download/smtbench-windows-amd64.exe": 404, // allowed name, file absent
		"/download/other":                      404,
		"/download/..%2fdb.sqlite":             404,
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: %d want %d", path, resp.StatusCode, want)
		}
	}
}

func TestStaticAndHealth(t *testing.T) {
	srv, _ := newServer(t, 32<<20)
	for path, want := range map[string]string{"/": "<html>ok</html>", "/healthz": "ok"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.TrimSpace(string(b)) != want {
			t.Errorf("%s: %q", path, b)
		}
	}
}

func TestStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db.sqlite")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	raw := fixture(t)
	r, _ := result.Decode(raw)
	if _, err := st.Put(context.Background(), r, raw); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, _ = OpenStore(path)
	defer st.Close()
	got, err := st.Get(context.Background(), r.RunID)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("%v", err)
	}
}

func TestListGroup(t *testing.T) {
	srv, _ := newServer(t, 32<<20)
	now := time.Now()
	for i, id := range []string{"a1", "a2", "b1"} {
		r, _ := result.Decode(variant(t, strings.Repeat(id, 16), now.Add(time.Duration(i)*time.Minute)))
		if id != "b1" {
			r.GroupID, r.GroupIndex, r.GroupTotal = "g1", i+1, 2
			r.Params.TickRate, r.Params.TickLoadPct, r.Params.StabMS = 64, 50, 1000
			r.Stability = &result.Stability{TickRate: 64, TickLoadPct: 50, WorkTicks: 1, IdleWorkUs: 7800, DurationMS: 1000,
				Scenarios: []result.StabScenario{{Name: "all_busy", Servers: []result.StabServer{{CPU: 0, Ticks: 60,
					PerSec: []int{60}, MaxWorkPerSec: []float64{16000}, WorkMeanUs: 9000, WorkP99us: 15000}}}}}
		}
		b, _ := json.Marshal(r)
		if resp, _ := postGzip(t, srv.URL, b); resp.StatusCode != 201 {
			t.Fatalf("post %s: %d", id, resp.StatusCode)
		}
	}
	var all, grp []Meta
	getJSON(t, srv.URL+"/api/runs", &all)
	getJSON(t, srv.URL+"/api/runs?group=g1", &grp)
	if len(all) != 3 || len(grp) != 2 || grp[0].GroupTotal != 2 {
		t.Fatalf("all %d group %d", len(all), len(grp))
	}
	h := grp[0].Headline.Stab
	if h == nil || h.Scenario != "all_busy" || h.WorstSecPct > 94 || h.WorstSecPct < 93 || grp[0].Headline.Ratings["stab"] != summary.Warn {
		t.Fatalf("%+v %v", h, grp[0].Headline.Ratings)
	}
}
