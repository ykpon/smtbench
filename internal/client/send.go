// Package client sends results to the collector.
package client

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// backoff between attempts; 3 attempts in total.
var backoff = []time.Duration{time.Second, 3 * time.Second}

type permanentError struct{ msg string }

func (e permanentError) Error() string { return e.msg }

// Send posts a gzipped result to baseURL/api/runs and returns the URL of the run page.
func Send(ctx context.Context, baseURL string, raw []byte) (string, error) {
	base := strings.TrimRight(baseURL, "/")
	var zbuf bytes.Buffer
	zw := gzip.NewWriter(&zbuf)
	zw.Write(raw)
	zw.Close()

	var lastErr error
	for attempt := 0; attempt <= len(backoff); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff[attempt-1]):
			}
		}
		url, err := post(ctx, base, zbuf.Bytes())
		if err == nil {
			return url, nil
		}
		lastErr = err
		if _, ok := err.(permanentError); ok {
			break
		}
	}
	return "", lastErr
}

func post(ctx context.Context, base string, body []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/runs", bytes.NewReader(body))
	if err != nil {
		return "", permanentError{err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var reply struct {
		URL   string `json:"url"`
		Error string `json:"error"`
	}
	json.Unmarshal(b, &reply)
	switch {
	case resp.StatusCode == 200 || resp.StatusCode == 201:
		return base + reply.URL, nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		msg := reply.Error
		if msg == "" {
			msg = resp.Status
		}
		return "", permanentError{fmt.Sprintf("коллектор отклонил результат: %s", msg)}
	}
	return "", fmt.Errorf("коллектор ответил %s", resp.Status)
}
