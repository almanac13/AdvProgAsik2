package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

type Server struct {
	mu           sync.Mutex
	data         map[string]string
	totalRequests int
	methodCount  map[string]int
	errorCount   int
	shutdownCh   chan struct{}
}

func NewServer() *Server {
	return &Server{
		data:        make(map[string]string),
		methodCount: make(map[string]int),
		shutdownCh:  make(chan struct{}),
	}
}

// POST
func (s *Server) postDataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		s.incrementError()
		return
	}

	var payload map[string]string
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		s.incrementError()
		return
	}

	s.mu.Lock()
	for k, v := range payload {
		s.data[k] = v
	}
	s.totalRequests++
	s.methodCount[r.Method]++
	s.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// GET
func (s *Server) getDataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		s.incrementError()
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalRequests++
	s.methodCount[r.Method]++
	json.NewEncoder(w).Encode(s.data)
}

// DELETE
func (s *Server) deleteDataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		s.incrementError()
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 3 || parts[2] == "" {
		http.Error(w, "Key not specified", http.StatusBadRequest)
		s.incrementError()
		return
	}
	key := parts[2]

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[key]; ok {
		delete(s.data, key)
		s.totalRequests++
		s.methodCount[r.Method]++
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	} else {
		http.Error(w, "Key not found", http.StatusNotFound)
		s.incrementError()
	}
}

// GET 
func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		s.incrementError()
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalRequests++
	s.methodCount[r.Method]++

	stats := map[string]interface{}{
		"total_requests": s.totalRequests,
		"data_size":      len(s.data),
		"method_count":   s.methodCount,
		"errors":         s.errorCount,
	}
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) incrementError() {
	s.mu.Lock()
	s.errorCount++
	s.mu.Unlock()
}

// Background worker
func (s *Server) startBackgroundWorker() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			fmt.Printf("[Worker] Requests: %d, Data size: %d, Errors: %d\n",
				s.totalRequests, len(s.data), s.errorCount)
			s.mu.Unlock()
		case <-s.shutdownCh:
			fmt.Println("[Worker] Stopped")
			return
		}
	}
}

func main() {
	server := NewServer()
	mux := http.NewServeMux()

	
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			server.getDataHandler(w, r)
		case http.MethodPost:
			server.postDataHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			server.incrementError()
		}
	})

	mux.HandleFunc("/data/", server.deleteDataHandler)
	mux.HandleFunc("/stats", server.statsHandler)

	// Start background worker
	go server.startBackgroundWorker()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Graceful shutdown
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt)
		<-stop
		fmt.Println("\nShutting down server...")

		// Stop background worker
		close(server.shutdownCh)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("Server shutdown failed: %v", err)
		}
		fmt.Println("Server exited gracefully")
	}()

	fmt.Println("Server starting on :8080")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
