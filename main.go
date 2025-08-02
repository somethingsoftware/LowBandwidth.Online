package main

import (
	_ "github.com/joho/godotenv"
	"log"
	_ "somethingsoftware/LowBandwidth.Online/db"
	"somethingsoftware/LowBandwidth.Online/ai"
)

func main() {
	// if err := godotenv.Load(".env"); err != nil {
	// 	log.Fatal("Error loading .env file")
	// }
	// db, err := db.NewDB()
	// if err != nil {
	// 	log.Fatal("Error connecting to database:", err)
	// }
	// defer db.Close()
	// log.Println("Database connection established successfully")
	response, err := ai.AIFunction("What is the current weather in New York?", "mistral")
	if err != nil {
		log.Fatal("Error querying AI: ", err)
	}
	log.Println("AI response:", response)
}
