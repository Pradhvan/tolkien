package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/Pradhvan/tolkien/dbManager"
	"github.com/go-redis/redis"
)

type RedisInstance struct {
	RInstance *redis.Client
}

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

func (c *RedisInstance) tokenHandler(w http.ResponseWriter, r *http.Request) {
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
		slog.Error("Failed to marshal JSON", "msg", err)
		http.Error(w, "Something went wrong!", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.Write(jsonResponse)

}

func main() {
	//Initialize Redis Client
	client := dbManager.InitRedisClient()
	redisHandler := &RedisInstance{RInstance: &client}
	mux := http.NewServeMux()
	mux.HandleFunc("/", redisHandler.tokenHandler)
	s := http.Server{
		Addr:         ":3333",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
		Handler:      mux,
	}
	fmt.Println("Listening on port :3333 . . .")
	err := s.ListenAndServe()
	if err != nil {
		if err != http.ErrServerClosed {
			panic(err)
		}
	}
}
