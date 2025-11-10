package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/larrasket/hlimiter/internal/config"
	"github.com/larrasket/hlimiter/internal/handler"
	"github.com/larrasket/hlimiter/internal/limiter"
)

func main() {
	// load config from env or default
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	fmt.Printf("[main] using config: %s\n", cfgPath)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// init rate limiter
	limiter := limiter.New(cfg)
	h := handler.New(limiter)

	// setup routes
	http.HandleFunc("/check", h.Check)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// simple health check
		// fmt.Printf("[health] check from %s\n", r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	// TODO: add /metrics endpoint?
	// TODO: add /config endpoint to view current config?

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("starting server on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
