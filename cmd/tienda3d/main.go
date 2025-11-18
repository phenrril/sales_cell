package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/phenrril/tienda3d/internal/app"
)

func main() {
	_ = godotenv.Load()

	zerolog.TimeFieldFormat = time.RFC3339
	zlog.Logger = zlog.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.Kitchen})

	dsn := os.Getenv("DB_DSN")
	if strings.TrimSpace(dsn) == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("DB_PORT")
		if port == "" {
			port = "5432"
		}
		user := os.Getenv("DB_USER")
		if user == "" {
			user = os.Getenv("POSTGRES_USER")
		}
		if user == "" {
			user = "postgres"
		}
		pass := os.Getenv("DB_PASSWORD")
		if pass == "" {
			pass = os.Getenv("POSTGRES_PASSWORD")
		}
		if pass == "" {
			pass = "postgres"
		}
		name := os.Getenv("DB_NAME")
		if name == "" {
			name = os.Getenv("POSTGRES_DB")
		}
		if name == "" {
			name = "tienda3d"
		}
		ssl := os.Getenv("DB_SSLMODE")
		if ssl == "" {
			ssl = "disable"
		}
		dsn = "host=" + host + " user=" + user + " password=" + pass + " dbname=" + name + " port=" + port + " sslmode=" + ssl
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to connect to database")
	}

	application, err := app.NewApp(db)
	if err != nil {
		zlog.Fatal().Err(err).Msg("failed to create app")
	}
	if err := application.MigrateAndSeed(); err != nil {
		zlog.Fatal().Err(err).Msg("failed to migrate and seed database")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {

		for p := 8081; p <= 8090; p++ {
			alt := net.JoinHostPort("", fmt.Sprintf("%d", p))
			l2, err2 := net.Listen("tcp", alt)
			if err2 == nil {
				ln = l2
				port = fmt.Sprint(p)
				break
			}
		}
		if ln == nil {
		}
	}

	server := &http.Server{Handler: application.HTTPHandler()}

	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
