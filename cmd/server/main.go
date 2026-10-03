package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog/log"

	"migrated-app/internal/config"
	"migrated-app/internal/model"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("load config")
	}

	dsn := cfg.DatabaseURL
	if dsn == "" {
		dsn = buildDSNFromEnv()
	}
	if dsn == "" {
		log.Fatal().Msg("DATABASE_URL is not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("open database")
	}
	defer db.Close()

	var pingErr error
	for i := 0; i < 30; i++ {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		pingErr = db.PingContext(pctx)
		cancel()
		if pingErr == nil {
			break
		}
		log.Warn().Err(pingErr).Msg("database not ready, retrying")
		time.Sleep(2 * time.Second)
	}
	if pingErr != nil {
		log.Fatal().Err(pingErr).Msg("connect database")
	}

	if err := model.EnsureUserSchema(ctx, db); err != nil {
		log.Fatal().Err(err).Msg("ensure schema")
	}

	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: buildRouter(db),
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	log.Info().Msg("server started on :" + port)
	<-ctx.Done()

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}
}

// buildDSNFromEnv assembles a MySQL DSN from discrete DB_* variables.
func buildDSNFromEnv() string {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				return v
			}
		}
		return ""
	}
	host := get("DB_HOST", "MYSQL_HOST")
	if host == "" {
		return ""
	}
	port := get("DB_PORT", "MYSQL_PORT")
	if port == "" {
		port = "3306"
	}
	user := get("DB_USER", "DB_USERNAME", "MYSQL_USER")
	pass := get("DB_PASSWORD", "MYSQL_PASSWORD")
	name := get("DB_NAME", "DB_DATABASE", "MYSQL_DATABASE")
	return user + ":" + pass + "@tcp(" + host + ":" + port + ")/" + name + "?parseTime=true"
}
