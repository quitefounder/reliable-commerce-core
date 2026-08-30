package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/cors"

	"github.com/quitefounder/reliable-commerce-core/api/graph"
	"github.com/quitefounder/reliable-commerce-core/api/internal/commerce"
	"github.com/quitefounder/reliable-commerce-core/api/internal/migrate"
	"github.com/quitefounder/reliable-commerce-core/api/internal/seed"
	"github.com/quitefounder/reliable-commerce-core/api/migrations"
)

func main() {
	dsn := env("DATABASE_URL", "postgres://commerce:commerce@127.0.0.1:5432/commerce?sslmode=disable")
	addr := env("ADDR", ":8080")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("postgres: %v", err)
	}
	if err := migrate.Up(db, migrations.Files); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := seed.IfEmpty(context.Background(), db); err != nil {
		log.Fatalf("seed: %v", err)
	}

	svc := commerce.New(db)
	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{Commerce: svc},
	}))

	mux := http.NewServeMux()
	mux.Handle("/graphql", srv)
	mux.Handle("/playground", playground.Handler("Commerce GraphQL", "/graphql"))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := cors.New(cors.Options{
		AllowedOrigins: allowedOrigins(),
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders: []string{"Content-Type"},
	}).Handler(mux)

	log.Printf("api listening on %s (graphql /graphql, playground /playground)", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func allowedOrigins() []string {
	raw := env("CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
