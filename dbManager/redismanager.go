package dbManager

import (
	"fmt"

	"github.com/go-redis/redis"

	"github.com/spf13/viper"
)

func InitRedisClient() redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     viper.GetString("REDIS.HOST") + ":" + viper.GetString("REDIS.PORT"),
		Password: viper.GetString("REDIS.PASSWORD"),
		DB: viper.GetInt("REDIS.DB"),
	})

	pong, err := client.Ping().Result()
	if err != nil {
		fmt.Println("Cannot Initialize Redis Client ", err)
	}
	fmt.Println("Redis Client Successfully Initialized . . .", pong)

	return *client
}
