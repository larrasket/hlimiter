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
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		panic("CONFIG_PATH environment variable is required")
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
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		panic("PORT environment variable is required")
	}

	fmt.Printf("starting server on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
