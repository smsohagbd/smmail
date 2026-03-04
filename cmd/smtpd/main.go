package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"learn/smtp-platform/internal/config"
	"learn/smtp-platform/internal/db"
	"learn/smtp-platform/internal/httpapi"
	"learn/smtp-platform/internal/repo"
	"learn/smtp-platform/internal/service"
	smtpsrv "learn/smtp-platform/internal/smtp"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	if cfg.UpstreamSMTPHost == "" {
		log.Printf("upstream relay: disabled (local dev mode)")
	} else {
		log.Printf("upstream relay: %s:%s user=%s", cfg.UpstreamSMTPHost, cfg.UpstreamSMTPPort, mask(cfg.UpstreamSMTPUser))
	}

	database, err := db.OpenAndMigrate(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("db init failed: %v", err)
	}
	defer database.Close()

	userRepo := repo.NewUserRepo(database)
	domainRepo := repo.NewDomainThrottleRepo(database)
	eventRepo := repo.NewMailEventRepo(database)
	queueRepo := repo.NewQueueRepo(database)
	analyticsRepo := repo.NewAnalyticsRepo(database)
	packageRepo := repo.NewPackageRepo(database)
	smtpRepo := repo.NewSMTPRepo(database)

	limiter := service.NewLimiter()
	authSvc := service.NewAuthService(userRepo)
	sendSvc := service.NewSendService(userRepo, domainRepo, eventRepo, queueRepo, limiter)
	deliverySvc := service.NewDeliveryService(queueRepo, eventRepo, userRepo, domainRepo, smtpRepo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go deliverySvc.Run(ctx)

	handler := httpapi.NewHandler(cfg, userRepo, domainRepo, eventRepo, analyticsRepo, packageRepo, smtpRepo, queueRepo, authSvc)
	httpServer := &http.Server{
		Addr:         cfg.HTTPListenAddr,
		Handler:      handler.Router(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
	}

	smtpServer := smtpsrv.NewServer(cfg, authSvc, sendSvc)

	go func() {
		log.Printf("admin API listening on %s", cfg.HTTPListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	go func() {
		log.Printf("smtp server listening on %s", cfg.SMTPListenAddr)
		if err := smtpServer.ListenAndServe(); err != nil {
			log.Fatalf("smtp server failed: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Printf("shutdown requested")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = smtpServer.Close()
	_ = httpServer.Shutdown(shutdownCtx)
}

func mask(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 3 {
		return "***"
	}
	return v[:2] + "***"
}
