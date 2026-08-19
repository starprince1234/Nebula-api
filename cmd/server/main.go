package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpapi "github.com/starprince1234/Nebula-api/internal/api/http"
	"github.com/starprince1234/Nebula-api/internal/config"
	"github.com/starprince1234/Nebula-api/internal/controlplane"
	"github.com/starprince1234/Nebula-api/internal/dataplane"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/cache"
	security "github.com/starprince1234/Nebula-api/internal/infrastructure/crypto"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/mail"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	startupContext, startupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startupCancel()

	dbClient, sqlDB, err := db.Open(startupContext, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer dbClient.Close()
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	cacheStore, err := cache.New(cfg.RedisURL, cfg.SSEStreamMaxLength)
	if err != nil {
		return err
	}
	defer cacheStore.Close()
	if err := cacheStore.Ping(startupContext); err != nil {
		return err
	}

	securityManager, err := security.NewManager(
		cfg.JWTSigningKey,
		cfg.AuthPepper,
		cfg.APIKeyPepper,
		cfg.ProviderEncryptionKey,
		cfg.AccessTTL,
	)
	if err != nil {
		return err
	}
	mailer, err := mail.NewSMTP(
		cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass,
		cfg.SMTPFrom, cfg.SMTPFromName, cfg.SMTPTLSMode, cfg.SMTPTimeout,
	)
	if err != nil {
		return err
	}
	controlService := controlplane.NewService(
		dbClient, cacheStore, securityManager, mailer,
		controlplane.Config{
			AccessTTL: cfg.AccessTTL, RefreshTTL: cfg.RefreshTTL,
			VerificationTTL: cfg.VerificationTTL, SendCooldown: cfg.SendCooldown,
			MaxAttempts: cfg.MaxAttempts, Lockout: cfg.Lockout,
			InvitationTTL: cfg.InvitationTTL, PublicAppURL: cfg.PublicAppURL,
		},
	)
	if err := controlService.BootstrapTeacher(
		startupContext,
		cfg.BootstrapTeacherName,
		cfg.BootstrapTeacherEmail,
		cfg.BootstrapTeacherPassword,
	); err != nil {
		return err
	}

	gateway := dataplane.NewGateway(dbClient, cacheStore, securityManager, dataplane.Config{
		ConnectTimeout:        cfg.UpstreamConnectTimeout,
		ResponseHeaderTimeout: cfg.UpstreamResponseHeaderTimeout,
		MaxRequestBytes:       cfg.GatewayMaxRequestBytes,
		VideoTaskRouteTTL:     cfg.VideoTaskRouteTTL,
	})
	api := httpapi.NewServer(
		controlService, securityManager, cacheStore, gateway,
		httpapi.HealthDependencies{Database: sqlDB, Cache: cacheStore},
		httpapi.Config{
			CookieSecure: cfg.CookieSecure(),
			RefreshTTL:   cfg.RefreshTTL,
			SSEHeartbeat: cfg.SSEHeartbeat,
		},
	)
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("Nebula server listening", "address", cfg.HTTPAddress)
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case <-signalContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownContext)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
