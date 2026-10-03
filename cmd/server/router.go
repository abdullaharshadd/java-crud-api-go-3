package main

import (
	"database/sql"
	"net/http"

	"github.com/rs/zerolog/log"

	"migrated-app/internal/httpapi"
	"migrated-app/internal/service"
	"migrated-app/internal/store"
)

func buildRouter(db *sql.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	repo := store.NewUserRepository(db)
	svc := service.NewUserService(repo)
	httpapi.NewHandler(svc, log.Logger).Register(mux)

	return httpapi.Middleware(mux)
}
