package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wallissonmarinho/GoAnimes/internal/adapters/storage"
	persistmigrate "github.com/wallissonmarinho/GoAnimes/internal/persistence/migrate"
)

func runMigrateCLI(args []string) int {
	lg := log.New(os.Stderr, "goanimes ", log.LstdFlags|log.Lmicroseconds)
	if len(args) < 1 {
		lg.Println("usage: goanimes migrate <up|down|version|status|goto> [args]")
		return 2
	}

	dataDir := os.Getenv("GOANIMES_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	_ = os.MkdirAll(dataDir, 0o755)
	dbPath := filepath.Join(dataDir, "goanimes.db")
	sqliteDSN := os.Getenv("GOANIMES_SQLITE_DSN")
	if sqliteDSN == "" {
		sqliteDSN = "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = sqliteDSN
	}

	db, pg, err := storage.OpenDB(dsn)
	if err != nil {
		lg.Println(err)
		return 1
	}
	defer db.Close()

	p, err := persistmigrate.NewMigrateProvider(db, pg)
	if err != nil {
		lg.Println(err)
		return 1
	}

	ctx := context.Background()
	cmd := args[0]
	switch cmd {
	case "up":
		_, err = p.Up(ctx)
	case "down":
		n := 1
		if len(args) > 1 {
			n, err = strconv.Atoi(args[1])
			if err != nil || n < 1 {
				lg.Println("down: optional argument must be a positive integer")
				return 2
			}
		}
		err = persistmigrate.MigrateDownSteps(ctx, p, n)
	case "version":
		var v int64
		v, err = p.GetDBVersion(ctx)
		if err != nil {
			break
		}
		_, _ = fmt.Fprintf(os.Stdout, "%d\n", v)
	case "status":
		err = persistmigrate.PrintMigrateStatus(ctx, p, os.Stdout)
	case "goto":
		if len(args) < 2 {
			lg.Println("goto: missing VERSION")
			return 2
		}
		target, convErr := strconv.ParseInt(args[1], 10, 64)
		if convErr != nil || target < 0 {
			lg.Println("goto: VERSION must be a non-negative integer")
			return 2
		}
		var cur int64
		cur, err = p.GetDBVersion(ctx)
		if err != nil {
			break
		}
		if target > cur {
			_, err = p.UpTo(ctx, target)
		} else if target < cur {
			_, err = p.DownTo(ctx, target)
		}
	default:
		lg.Printf("unknown migrate subcommand %q", cmd)
		return 2
	}

	if err != nil {
		lg.Println(err)
		return 1
	}
	return 0
}
