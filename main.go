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

	"github.com/spf13/viper"
)

type RedisInstance struct {
	RInstance *redis.Client
}

func generateToken() (string, error) {
	charset := viper.GetString("TOKEN.CHARSET")
	result := make([]byte, viper.GetInt("TOKEN.LENGTH"))
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
	//Initialize Viper
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()
	viper.SetConfigType("yml")
	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading config file, %s", err)
	}
	//Initialize Redis Client
	client := dbManager.InitRedisClient()
	redisHandler := &RedisInstance{RInstance: &client}
	mux := http.NewServeMux()
	mux.HandleFunc("/", redisHandler.tokenHandler)
	port := viper.GetString("APP.PORT")
	address := ":" + port
	s := http.Server{
		Addr:         address,
		ReadTimeout:  viper.GetDuration("APP.READ_TIMEOUT") * time.Second,
		WriteTimeout: viper.GetDuration("APP.WRITE_TIMEOUT") * time.Second,
		IdleTimeout:  viper.GetDuration("APP.IDLE_TIMEOUT") * time.Second,
		Handler:      mux,
	}
	fmt.Printf("Listening on port : %s", viper.GetString("APP.PORT"))
	err := s.ListenAndServe()
	if err != nil {
		if err != http.ErrServerClosed {
			panic(err)
		}
	}
}
