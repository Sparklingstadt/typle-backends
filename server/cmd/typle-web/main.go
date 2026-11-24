// Command typle-web serves the Go backend of the backend comparison (/api/health, /api/workspace and
// /api/workspace/records) locally, with the same handler as the Vercel function api/go.go (server/vercel), but without
// its TYPLE_BACKEND_COMPARE gate. It reads the environment only (DATABASE_URL or POSTGRES_URL, AUTH_SECRET and
// AUTH_URL); it does not load .env.local.
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sparklingstadt/typle-backends/server/internal/serve"
	"github.com/sparklingstadt/typle-backends/server/vercel"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8081", "address to listen on")
	flag.Parse()

	backend := vercel.New(false)
	defer backend.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// A client that sends or reads slowly, or keeps an idle connection open, does not hold it for ever.
	server := &http.Server{Handler: backend, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: time.Minute, IdleTimeout: 2 * time.Minute}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("serving the Go backend of the comparison on http://%s", *addr)
	// On SIGINT or SIGTERM, requests in flight get 5 seconds to finish before the deferred Close releases the database.
	if err := serve.Serve(ctx, server, listener, 5*time.Second); err != nil {
		log.Fatal(err)
	}
}
