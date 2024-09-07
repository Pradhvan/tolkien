package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
)

var globalTokenPool []string

func generateToken() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 11)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))] // Pick a random character from the charset
	}
	return string(result), nil
}

func generateTokenPool(poolSize int) error {
	tokenPool := make([]string, poolSize)
	for i := range tokenPool {
		token, err := generateToken()
		if err != nil {
			return err
		}
		tokenPool[i] = token
	}
	globalTokenPool = tokenPool
	fmt.Println(tokenPool)
	return nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	tokenList := strings.Join(globalTokenPool, "<br>")
	// Write a simple HTML response
	fmt.Fprintln(w, `
        <html>
            <head><title>Tolkien</title></head>
            <body>
                <h1>Welcome to the Tolkien</h1>
                
                <p>These are available tokens to use:</p>
            </body>
        </html>
    `, tokenList)
}

func main() {
	fmt.Println("Hello, World! Generating token pool")
	err := generateTokenPool(10)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Token pool sucessfully generated!")
	http.HandleFunc("/", handler)
	serverErr := http.ListenAndServe(":3333", nil)
	if serverErr != nil {
		log.Fatal(serverErr)
	}
}
