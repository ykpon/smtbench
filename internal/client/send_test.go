package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func init() { backoff = []time.Duration{time.Millisecond, time.Millisecond} }

func TestSendRetries(t *testing.T) {
	var n atomic.Int32
	body := []byte(`{"run_id":"x"}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 3 {
			w.WriteHeader(500)
			return
		}
		if r.URL.Path != "/api/runs" || r.Header.Get("Content-Encoding") != "gzip" {
			t.Errorf("path %s enc %s", r.URL.Path, r.Header.Get("Content-Encoding"))
		}
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		got, _ := io.ReadAll(zr)
		if !bytes.Equal(got, body) {
			t.Errorf("body %s", got)
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"id":"x","url":"/#/run/x"}`))
	}))
	defer srv.Close()
	url, err := Send(context.Background(), srv.URL+"/", body)
	if err != nil {
		t.Fatal(err)
	}
	if url != srv.URL+"/#/run/x" || n.Load() != 3 {
		t.Fatalf("url %s requests %d", url, n.Load())
	}
}

func TestSendNoRetryOn400(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"плохо"}`))
	}))
	defer srv.Close()
	_, err := Send(context.Background(), srv.URL, []byte("{}"))
	if err == nil || !strings.Contains(err.Error(), "плохо") || n.Load() != 1 {
		t.Fatalf("err %v requests %d", err, n.Load())
	}
}

func TestSendUnreachable(t *testing.T) {
	if _, err := Send(context.Background(), "http://127.0.0.1:1", []byte("{}")); err == nil {
		t.Fatal("expected error")
	}
}

func TestSendAddsScheme(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte(`{"id":"x","url":"/#/run/x"}`))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	url, err := Send(context.Background(), host, []byte("{}"))
	if err != nil || url != srv.URL+"/#/run/x" {
		t.Fatalf("%q %v", url, err)
	}
}
