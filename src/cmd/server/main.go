package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/signal"
	"src/internal/config"
	"syscall"
	"time"
)

//--------------------------------------------------------------------------------------

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Setup()
	if err != nil {
		log.Fatalf("Failed to setup: %v", err)
	}
	defer cfg.Close()

	addr := fmt.Sprintf(":%d", cfg.Port)
	var listener net.Listener

	listener, err = net.Listen("tcp", addr)

	if err != nil {
		log.Fatalf("[fatal] listen on port %d: %v", cfg.Port, err)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: http.DefaultServeMux,
	}
	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("[error] server shutdown: %v", err)
		}
		if err == nil {
			log.Printf("[server] server shutdown gracefully")
		}
	}()

	log.Printf("[server] running at http://localhost:%d", cfg.Port)
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[fatal] server failed: %v", err)
	}
}
