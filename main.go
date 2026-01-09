package main

import (
	"context"
	"golangdb/database"
	"golangdb/server"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

func LoadENV() error {
	return godotenv.Load()
}

func main() {
	// Loading .env file
	// If there is an error, it is a problem with the .env file -> we panic, nothing more to do
	if err := LoadENV(); err != nil {
		log.Panicf("Error loading .env file: %s", err.Error())
	}
	// Initialing the core db. It is necessary here since by opening the DB core we re-initialize files (WAL and .db file),
	// drop in-memory storage, re-allocate it, then we check for snapshot and replaying wal.
	// Initialize the core in here is a necessity.
	databaseCore, err := database.OpenDB(database.DbPath, database.WalPath, database.WalSizeLimit)

	// Of course, if an error happened, it is a problem with the core -> we panic, nothing more to do
	if err != nil {
		log.Panicf("Database core malfunctions: %s", err.Error())
	}

	// This is a storage, which is basically a struct with a pointer to databaseCore. It is an upper layer, having
	// methods, rules, checking and chain-based operations. Under the hood its methods call for low-level core methods.
	// It cannot return an error because it is fully dependent on core -> if there is a core, this will function.
	myDatabaseStorage := database.NewDB(databaseCore)

	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	myServer := server.NewServer(myDatabaseStorage, port)

	go func() {
		if err := myServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Panicf("Server malfunctions: %s", err.Error())
		}
	}()
	log.Printf("Server started on port: %s", port)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	defer stop()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = myServer.ShutdownGracefully(shutdownCtx)

	if err != nil {
		log.Println("Server forced to stop")
	} else {
		log.Println("Server gracefully stopped")
	}
}
