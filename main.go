// iplist serves line-delimited IPv4 allowlists over HTTP.
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

	"git.stellar.study/stellar-study/iplist/internal/azure"
	"git.stellar.study/stellar-study/iplist/internal/github"
	"git.stellar.study/stellar-study/iplist/internal/server"
)

func main() {
	addr := flag.String("addr", getEnv("LISTEN_ADDR", ":8080"), "address to listen on")
	refresh := flag.Duration("refresh", getDurationEnv("REFRESH_INTERVAL", time.Hour), "how often to refresh allowlists")
	timeout := flag.Duration("timeout", getDurationEnv("FETCH_TIMEOUT", 30*time.Second), "HTTP fetch timeout")
	flag.Parse()

	sources := []server.Source{
		azure.NewSource(*timeout),
		github.NewSource(*timeout),
	}

	srv := server.New(*addr, *refresh, sources)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx)
	}()

	select {
	case <-ctx.Done():
		log.Println("shutting down")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("warning: invalid %s=%q, using default %v", key, v, defaultValue)
		return defaultValue
	}
	return d
}
