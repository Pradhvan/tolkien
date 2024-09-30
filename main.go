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
	DeletedRInstance *redis.Client
}

const errMsg = "Something went wrong"
const jsonMarshalErrMsg = "Failed to marshal JSON"
const contentTypeHeader = "Content-Type"
const applicationJSON = "application/json"
const serviceIDRequiredErrMsg = "serviceID is required"
const tokenPoolSizeKey = "TOKEN.POOL_SIZE"

func generateToken() (string, error) {
	charset := viper.GetString("TOKEN.CHARSET")
	result := make([]byte, viper.GetInt("TOKEN.LENGTH"))
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result), nil
}

func generateTokenPool(poolSize int) ([]string, error) {
	tokenPool := make([]string, 0, poolSize)
	
	deletedTokenClient := dbManager.InitDeletedTokenRedisClient()

	deletedTokens := deletedTokenClient.Keys("*").Val()

	for len(tokenPool) < poolSize {
		token, err := generateToken()
		if err != nil {
			return nil, err
		}

		if !contains(deletedTokens, token) {
			tokenPool = append(tokenPool, token)
		}
	}

	return tokenPool, nil
}

func contains(tokens []string, token string) bool {
	for _, t := range tokens {
		if t == token {
			return true
		}
	}
	return false
}

func (c *RedisInstance) tokenHandler(w http.ResponseWriter, r *http.Request) {
	tokenPool := c.RInstance.Keys("*").Val()
	
	validTokens := []string{}
	for _, token := range tokenPool {
		ttl := c.RInstance.TTL(token).Val()
		if ttl > 0 {
			validTokens = append(validTokens, token)
		}
	}

	if len(validTokens) == 0 {
		tokens, err := generateTokenPool(viper.GetInt(tokenPoolSizeKey))
		if err != nil {
			slog.Error("Failed to generate token", "msg", err)
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}
		err = c.storeTokenPool(tokens)
		if err != nil {
			slog.Error("Failed to store token", "msg", err)
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}
		validTokens = tokens
	}

	data := struct {
		Token []string `json:"availableTokens"`
	}{
		Token: validTokens,
	}

	jsonResponse, err := json.Marshal(data)
	if err != nil {
		slog.Error(jsonMarshalErrMsg, "msg", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, applicationJSON)
	w.Write(jsonResponse)
}

func (c *RedisInstance) storeTokenPool(tokenPool []string) error {
	for _, token := range tokenPool {
		err := c.RInstance.HMSet(token, map[string]interface{}{
			"serviceID": "",
			"is_blocked": 0,
		}).Err()
		if err != nil {
			return err
		}
		c.RInstance.Expire(token, 5*time.Minute)
	}
	return nil
}

func (c *RedisInstance) assignToken(w http.ResponseWriter, r *http.Request) {
	serviceID := r.URL.Query().Get("serviceID")
	if serviceID == "" {
		http.Error(w, serviceIDRequiredErrMsg, http.StatusBadRequest)
		return
	}


	var assignedToken string
	tokenPool := c.RInstance.Keys("*").Val()
	for _, token := range tokenPool {
		isBlocked, err := c.RInstance.HGet(token, "is_blocked").Int()
		if err != nil {
			slog.Error("Failed to get token", "msg", err)
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}

		if isBlocked == 0 {
			c.RInstance.HMSet(token, map[string]interface{}{
				"serviceID": serviceID,
				"is_blocked": 1,
				"ticker": 60,
			})
			assignedToken = token
			break
		}
	}

	if assignedToken == "" {
		http.Error(w, "No token available", http.StatusNotFound)
		return
	}

	data := struct {
		AssignedToken string `json:"assignedToken"`
	}{
		AssignedToken: assignedToken,
	}

	jsonResponse, err := json.Marshal(data)
	if err != nil {
		slog.Error(jsonMarshalErrMsg, "msg", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, applicationJSON)
	w.Write(jsonResponse)
}

func (c *RedisInstance) monitorTickers() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		tokenPool := c.RInstance.Keys("*").Val()
		for _, token := range tokenPool {
			isBlocked, _ := c.RInstance.HGet(token, "is_blocked").Int()
			if isBlocked == 1 {
				currentTicker, _ := c.RInstance.HGet(token, "ticker").Int()
				if currentTicker >0 {
					c.RInstance.HSet(token, "ticker", currentTicker-1)
				}

				if currentTicker-1 == 0 {
					c.RInstance.HMSet(token, map[string]interface{}{
						"serviceID": "",
						"is_blocked": 0,
						"ticker": 0,
					})
				}
			}
		}
	}
}

func (c *RedisInstance) keepAliveToken(w http.ResponseWriter, r *http.Request) {
	serviceID := r.URL.Query().Get("serviceID")

	if serviceID == "" {
		http.Error(w, serviceIDRequiredErrMsg, http.StatusBadRequest)
		return
	}

	tokenPool := c.RInstance.Keys("*").Val()
	var token string
	for _, t := range tokenPool {
		currentServiceID, err := c.RInstance.HGet(t, "serviceID").Result()
		if err == nil && currentServiceID == serviceID {
			token = t
			break
		} 
	}

	if token == "" {
		http.Error(w, "Token not found", http.StatusNotFound)
		return
	}

	err := c.RInstance.HSet(token, "ticker", 60).Err()
	if err != nil {
		slog.Error("Failed to set ticker", "msg", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	data := struct {
		Message string `json:"message"`
	} {
		Message: "Keep-Alive successful, token is valid for next 60 seconds",
	}

	jsonResponse, err := json.Marshal(data)
	if err != nil {
		slog.Error(jsonMarshalErrMsg, "msg", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, applicationJSON)
	w.Write(jsonResponse)
}

func (c *RedisInstance) deleteToken(w http.ResponseWriter, r *http.Request) {
	serviceID := r.URL.Query().Get("serviceID")
	if serviceID == "" {
		http.Error(w, serviceIDRequiredErrMsg, http.StatusBadRequest)
		return
	}


	tokenPool := c.RInstance.Keys("*").Val()
	var tokenToDelete string
	for _, token := range tokenPool {
		currentServiceID, err := c.RInstance.HGet(token, "serviceID").Result()
		if err == nil && currentServiceID == serviceID {
			tokenToDelete = token
			break
		}
	}

	if tokenToDelete == "" {
		http.Error(w, "Token not found", http.StatusNotFound)
		return
	}

	err := c.DeletedRInstance.HMSet(tokenToDelete, map[string]interface{}{
		"serviceID": serviceID,
		"deleted_at": time.Now().Format(time.RFC3339),
	}).Err()

	if err != nil {
		slog.Error("Failed to delete token", "msg", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	err = c.RInstance.Del(tokenToDelete).Err()
	if err != nil {
		slog.Error("Failed to delete token", "msg", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	remainingTokens := c.RInstance.Keys("*").Val()
	if len(remainingTokens) < viper.GetInt(tokenPoolSizeKey) {
		newTokens, err := generateTokenPool(viper.GetInt(tokenPoolSizeKey) - len(remainingTokens))
		if err != nil {
			slog.Error("Failed to generate token", "msg", err)
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}
		
		err = c.storeTokenPool(newTokens)
		if err != nil {
			slog.Error("Failed to store token", "msg", err)
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}
	}


	data := struct {
		Message string `json:"message"`
	}{
		Message: "Token deleted successfully",
	}

	jsonResponse, err := json.Marshal(data)
	if err != nil {
		slog.Error(jsonMarshalErrMsg, "msg", err)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentTypeHeader, applicationJSON)
	w.Write(jsonResponse)
}

func main() {
	runEnv := "testing"

	//Initialize Viper
	viper.SetConfigType("yml")
	switch runEnv {
	case "testing":
		viper.SetConfigName("config_testing")
	case "production":
		viper.SetConfigName("config_production")
	}
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	const appPortKey = "APP.PORT"
	//Set Default Values
	viper.SetDefault(appPortKey, "3333")
	viper.SetDefault("APP.READ_TIMEOUT", 30)
	viper.SetDefault("APP.WRITE_TIMEOUT", 90)
	viper.SetDefault("APP.IDLE_TIMEOUT", 120)
	viper.SetDefault("TOKEN.LENGTH", 11)
	viper.SetDefault("TOKEN.CHARSET", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	viper.SetDefault(tokenPoolSizeKey, 10)
	viper.SetDefault("REDIS.HOST", "localhost")
	viper.SetDefault("REDIS.PORT", "6379")
	viper.SetDefault("REDIS.PASSWORD", "")
	viper.SetDefault("REDIS.DB", 0)

	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading config file, %s", err)
	}
	//Initialize Redis Client
	client := dbManager.InitRedisClient()
	if client == nil {
		panic("Cannot Initialize Redis Client")
	}

	// Initialize Deleted Token Redis Client
	deletedTokenClient := dbManager.InitDeletedTokenRedisClient()
	if deletedTokenClient == nil {
		panic("Cannot Initialize Deleted Tokens Redis Client")
	}

	redisHandler := RedisInstance{
		RInstance: client,
		DeletedRInstance: deletedTokenClient,
	}

	mux := http.NewServeMux()
	go redisHandler.monitorTickers()
	mux.HandleFunc("/", redisHandler.tokenHandler)
	mux.HandleFunc("/assign", redisHandler.assignToken)
	mux.HandleFunc("/keep-alive", redisHandler.keepAliveToken)
	mux.HandleFunc("/delete", redisHandler.deleteToken)
	port := viper.GetString(appPortKey)
	address := ":" + port
	s := http.Server{
		Addr:         address,
		ReadTimeout:  viper.GetDuration("APP.READ_TIMEOUT") * time.Second,
		WriteTimeout: viper.GetDuration("APP.WRITE_TIMEOUT") * time.Second,
		IdleTimeout:  viper.GetDuration("APP.IDLE_TIMEOUT") * time.Second,
		Handler:      mux,
	}
	fmt.Printf("Listening on port : %s", viper.GetString(appPortKey))
	err := s.ListenAndServe()
	if err != nil {
		if err != http.ErrServerClosed {
			panic(err)
		}
	}
}
