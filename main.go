package main

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"net/http"
	"time"
)

func generateToken() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 11)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result), nil
}

func generateTokenPool(poolSize int) ([]string, error) {
	tokenPool := make([]string, poolSize)
	for i := range tokenPool {
		token, err := generateToken()
		if err != nil {
			return nil, err
		}
		tokenPool[i] = token
	}
	return tokenPool, nil
}

func tokenHandler(w http.ResponseWriter, r *http.Request) {
	tokens, err := generateTokenPool(10)

	if err != nil {
		slog.Error("Failed to generate token", "msg", err)
		http.Error(w, "Something went wrong!", http.StatusInternalServerError)
		return
	}

	data := struct {
		Token []string `json:"availableTokens"`
	}{
		Token: tokens,
	}

	jsonResponse, err := json.Marshal(data)

	if err != nil {
		// If marshaling fails, respond with an internal server error
		slog.Error("Failed to marshal JSON", "msg", err)
		http.Error(w, "Something went wrong!", http.StatusInternalServerError)
		return
	}

	// Set the response header to indicate JSON content
	w.Header().Set("Content-Type", "application/json")

	// Write the JSON response
	w.Write(jsonResponse)

}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/tokens", tokenHandler)
	s := http.Server{
		Addr:         ":3333",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      mux,
	}
	err := s.ListenAndServe()
	if err != nil {
		if err != http.ErrServerClosed {
			panic(err)
		}
	}
}
