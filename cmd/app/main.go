package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	pgxdriver "github.com/wb-go/wbf/dbpg/pgx-driver"
	"github.com/wb-go/wbf/kafka/dlq"
	kafkav2 "github.com/wb-go/wbf/kafka/kafka-v2"
	"github.com/wb-go/wbf/logger"

	"github.com/ponchik327/ImageProcessor/internal/config"
	"github.com/ponchik327/ImageProcessor/internal/handler"
	"github.com/ponchik327/ImageProcessor/internal/kafka"
	"github.com/ponchik327/ImageProcessor/internal/migration"
	"github.com/ponchik327/ImageProcessor/internal/processor"
	"github.com/ponchik327/ImageProcessor/internal/repository/image_repo"
	"github.com/ponchik327/ImageProcessor/internal/repository/variant_repo"
	"github.com/ponchik327/ImageProcessor/internal/service"
	"github.com/ponchik327/ImageProcessor/internal/storage"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	runMigrate := flag.Bool("migrate", false, "run database migrations and exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	log, err := initLogger(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}

	if *runMigrate {
		if migrateErr := migration.Run(cfg.Database.DSN, log); migrateErr != nil {
			log.Error("migration failed", "error", migrateErr)
			os.Exit(1)
		}

		return
	}

	if err = run(cfg, log); err != nil {
		log.Error("application error", "error", err)
		os.Exit(1)
	}
}

func run(cfg *config.Config, log logger.Logger) error {
	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- Postgres ---
	pg, err := pgxdriver.New(
		cfg.Database.DSN,
		log,
		pgxdriver.MaxPoolSize(cfg.Database.MaxPoolSize),
		pgxdriver.MaxConnAttempts(cfg.Database.MaxAttempts),
	)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pg.Close()

	imageRepo := image_repo.New(pg, pg.Builder)
	variantRepo := variant_repo.New(pg, pg.Builder)

	// --- Хранилище файлов ---
	fileStorage, err := storage.New(cfg.Storage.BasePath)
	if err != nil {
		return fmt.Errorf("init file storage: %w", err)
	}

	// --- Kafka-продюсер ---
	kafkaProducer := kafka.NewProducer(&cfg.Kafka, log)
	defer func() {
		if closeErr := kafkaProducer.Close(); closeErr != nil {
			log.Error("kafka producer close", "error", closeErr)
		}
	}()

	// --- Сервис ---
	svc := service.New(imageRepo, variantRepo, kafkaProducer, fileStorage, log, &cfg.Processing)

	// Сбрасываем изображения, зависшие в статусе "processing" после предыдущего сбоя.
	svc.RecoverProcessing(rootCtx)

	// --- Реестр обработчиков ---
	registry := processor.Registry{
		"resize":    processor.NewResizeProcessor(),
		"thumbnail": processor.NewThumbnailProcessor(),
		"watermark": processor.NewWatermarkProcessor(cfg.Storage.WatermarkPath),
	}

	// --- Kafka-потребитель + DLQ ---
	// kafka.SilentInfoLogger подавляет внутренние INFO-сообщения kafka-go об истечении
	// таймаута опроса ("no messages received"), которые иначе засоряли бы логи.
	kafkaConsumer := kafkav2.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.Topic, cfg.Kafka.GroupID, kafka.SilentInfoLogger(log))
	defer func() {
		if closeErr := kafkaConsumer.Close(); closeErr != nil {
			log.Error("kafka consumer close", "error", closeErr)
		}
	}()

	var dlqClient *dlq.DLQ

	if cfg.Kafka.DLQTopic != "" {
		dlqProducer := kafkav2.NewProducer(cfg.Kafka.Brokers, cfg.Kafka.DLQTopic, log)
		defer func() {
			if closeErr := dlqProducer.Close(); closeErr != nil {
				log.Error("kafka dlq producer close", "error", closeErr)
			}
		}()

		dlqClient = dlq.New(dlqProducer, log)
	}

	kafkaProcessor, err := kafkav2.NewProcessor(
		kafkaConsumer,
		dlqClient,
		log,
		kafkav2.MaxAttempts(cfg.Kafka.MaxAttempts),
		kafkav2.BaseRetryDelay(cfg.Kafka.BaseRetryDelay),
		kafkav2.MaxRetryDelay(cfg.Kafka.MaxRetryDelay),
	)
	if err != nil {
		return fmt.Errorf("build kafka processor: %w", err)
	}

	consumerHandler := kafka.NewConsumerHandler(svc, registry, log)

	// Запускаем в фоне; останавливается при отмене rootCtx.
	kafkaProcessor.Start(rootCtx, consumerHandler.Handle)

	// --- HTTP-сервер ---
	httpHandler := handler.New(svc, fileStorage, log)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port),
		Handler:      httpHandler.Routes(),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}

	srvErrCh := make(chan error, 1)

	go func() {
		log.Info("http server starting", "addr", srv.Addr)

		if listenErr := srv.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			srvErrCh <- listenErr
		}

		close(srvErrCh)
	}()

	select {
	case <-rootCtx.Done():
		log.Info("shutdown signal received")
	case srvErr := <-srvErrCh:
		if srvErr != nil {
			return fmt.Errorf("http server: %w", srvErr)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer shutdownCancel()

	if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
		log.Error("http server shutdown", "error", shutdownErr)
	}

	log.Info("shutdown complete")

	return nil
}

func initLogger(cfg *config.Config) (logger.Logger, error) {
	return logger.InitLogger(
		logger.ZapEngine,
		"image-processor",
		cfg.Log.Env,
		logger.WithLevel(parseLogLevel(cfg.Log.Level)),
	)
}

func parseLogLevel(level string) logger.Level {
	switch level {
	case "debug":
		return logger.DebugLevel
	case "warn":
		return logger.WarnLevel
	case "error":
		return logger.ErrorLevel
	default:
		return logger.InfoLevel
	}
}
