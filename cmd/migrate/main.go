package main

import (
	"fmt"
	"github/com/anukool23/olx-api/internal/config"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main(){
	args := os.Args
	if len(args) < 2 {
		log.Fatal("usage:migrate <up | down>")
	}
	fmt.Println("Migration started...")

	cfg := config.MustLoad()
	
	m, err := migrate.New("file://migrations",cfg.DbUri)
	if err != nil {
		log.Fatalf("migration:main: %s",err)
	}


	switch (args[1]){
	case "up":
		if err := m.Up(); err != nil {
			log.Fatalf("migration:main: %s",err)
		}
	case "down":
		if err := m.Down(); err != nil {
			log.Fatalf("migration:main: %s",err)
		}
	default:
		log.Fatalf("Unknown command %s",args[1])
	}
	fmt.Println("Migration Completed...")
}
