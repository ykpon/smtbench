// smtbench-server collects benchmark results and serves the web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smtbench/internal/buildinfo"
	"smtbench/internal/collector"
	"smtbench/web"
)

func main() {
	listen := flag.String("listen", ":8080", "адрес HTTP-сервера")
	dbPath := flag.String("db", "/data/smtbench.db", "файл базы SQLite")
	dlDir := flag.String("download-dir", "/app/download", "каталог с клиентскими бинарниками")
	maxBody := flag.Int64("max-body", 32<<20, "максимальный размер результата, байт")
	flag.Parse()

	st, err := collector.OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("база данных: %v", err)
	}
	defer st.Close()

	srv := &http.Server{
		Addr:              *listen,
		Handler:           collector.NewHandler(st, web.FS, *dlDir, *maxBody),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Printf("smtbench-server %s слушает %s, база %s", buildinfo.Version, *listen, *dbPath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
