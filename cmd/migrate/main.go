package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Santosh27927/Backend-COT/internal/config"
	"github.com/Santosh27927/Backend-COT/internal/database"
	"github.com/Santosh27927/Backend-COT/migrations"
)

func run() int {
	action := flag.String("action", "up", "up or status")
	flag.Parse()
	if *action != "up" && *action != "status" {
		fmt.Fprintln(os.Stderr, "action must be up or status")
		return 1
	}
	settings, err := config.LoadDatabase(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, settings)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database configuration rejected")
		return 1
	}
	defer pool.Close()
	known, err := migrations.Embedded()
	if err != nil {
		fmt.Fprintln(os.Stderr, "embedded migrations invalid")
		return 1
	}
	if *action == "status" {
		ok, code := migrations.Ready(ctx, pool, known)
		fmt.Println(code)
		if !ok {
			return 1
		}
		return 0
	}
	if err = migrations.Apply(ctx, pool, known); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed: verify database access and migration integrity; credentials are omitted")
		return 1
	}
	fmt.Println("migrations applied")
	return 0
}
func main() { os.Exit(run()) }
