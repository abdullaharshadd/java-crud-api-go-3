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

	candidates := candidateDSNs(cfg.DatabaseURL)
	if len(candidates) == 0 {
		log.Fatal().Msg("DATABASE_URL is not set")
	}

	var db *sql.DB
	var pingErr error
	for i := 0; i < 30 && db == nil; i++ {
		for _, dsn := range candidates {
			cand, err := sql.Open("mysql", dsn)
			if err != nil {
				pingErr = err
				continue
			}
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			pingErr = cand.PingContext(pctx)
			cancel()
			if pingErr == nil {
				db = cand
				break
			}
			_ = cand.Close()
		}
		if db == nil {
			log.Warn().Err(pingErr).Msg("database not ready, retrying")
			time.Sleep(2 * time.Second)
		}
	}
	if db == nil {
		log.Fatal().Err(pingErr).Msg("connect database")
	}
	defer db.Close()

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

// candidateDSNs returns the distinct MySQL DSNs derivable from the
// environment, in priority order.
func candidateDSNs(primary string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = normalizeDSN(strings.TrimSpace(s))
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(primary)
	add(os.Getenv("DATABASE_URL"))
	add(os.Getenv("DB_URL"))
	add(buildDSNFromEnv())
	return out
}

// normalizeDSN converts URL-style DSNs (mysql://user:pass@host:port/db)
// into the go-sql-driver format; other values are returned unchanged.
func normalizeDSN(s string) string {
	if !strings.Contains(s, "://") {
		return s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return s
	}
	host := u.Host
	if u.Port() == "" {
		host += ":3306"
	}
	creds := ""
	if u.User != nil {
		creds = u.User.Username()
		if p, ok := u.User.Password(); ok {
			creds += ":" + p
		}
		creds += "@"
	}
	q := u.Query()
	if q.Get("parseTime") == "" {
		q.Set("parseTime", "true")
	}
	return creds + "tcp(" + host + ")/" + strings.TrimPrefix(u.Path, "/") + "?" + q.Encode()
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
