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
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		panic("CONFIG_PATH environment variable is required")
	}
	fmt.Printf("[main] loading config: %s\n", configPath)

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	rl := limiter.New(cfg)
	h := handler.New(rl)

	http.HandleFunc("/check", h.Check)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		panic("PORT environment variable is required")
	}

	fmt.Printf("starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
