package config

import (
	"time"

	cleanenvport "github.com/wb-go/wbf/config/cleanenv-port"
)

// Config содержит всю конфигурацию приложения.
type Config struct {
	HTTP       HTTPConfig       `yaml:"http"`
	Database   DatabaseConfig   `yaml:"database"`
	Kafka      KafkaConfig      `yaml:"kafka"`
	Storage    StorageConfig    `yaml:"storage"`
	Processing ProcessingConfig `yaml:"processing"`
	Log        LogConfig        `yaml:"log"`
}

// HTTPConfig содержит настройки HTTP-сервера.
type HTTPConfig struct {
	Host            string        `yaml:"host"             env:"HTTP_HOST"`
	Port            int           `yaml:"port"             env:"HTTP_PORT"             validate:"required,min=1"`
	ReadTimeout     time.Duration `yaml:"read_timeout"     env:"HTTP_READ_TIMEOUT"     validate:"required"`
	WriteTimeout    time.Duration `yaml:"write_timeout"    env:"HTTP_WRITE_TIMEOUT"    validate:"required"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"     env:"HTTP_IDLE_TIMEOUT"     validate:"required"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"HTTP_SHUTDOWN_TIMEOUT" validate:"required"`
}

// DatabaseConfig содержит настройки подключения к PostgreSQL.
type DatabaseConfig struct {
	DSN         string `yaml:"dsn"          env:"DATABASE_DSN"          validate:"required"`
	MaxPoolSize int32  `yaml:"max_pool_size" env:"DATABASE_MAX_POOL_SIZE" validate:"required,min=1"`
	MaxAttempts int    `yaml:"max_attempts"  env:"DATABASE_MAX_ATTEMPTS"  validate:"required,min=1"`
}

// KafkaConfig содержит настройки брокеров и топиков Kafka.
type KafkaConfig struct {
	Brokers        []string      `yaml:"brokers"          env:"KAFKA_BROKERS"          validate:"required,min=1"`
	Topic          string        `yaml:"topic"            env:"KAFKA_TOPIC"            validate:"required"`
	GroupID        string        `yaml:"group_id"         env:"KAFKA_GROUP_ID"         validate:"required"`
	DLQTopic       string        `yaml:"dlq_topic"        env:"KAFKA_DLQ_TOPIC"`
	MaxAttempts    int           `yaml:"max_attempts"     env:"KAFKA_MAX_ATTEMPTS"     validate:"required,min=1"`
	BaseRetryDelay time.Duration `yaml:"base_retry_delay" env:"KAFKA_BASE_RETRY_DELAY" validate:"required"`
	MaxRetryDelay  time.Duration `yaml:"max_retry_delay"  env:"KAFKA_MAX_RETRY_DELAY"  validate:"required"`
}

// StorageConfig содержит пути файлового хранилища.
type StorageConfig struct {
	BasePath      string `yaml:"base_path"      env:"STORAGE_BASE_PATH"      validate:"required"`
	WatermarkPath string `yaml:"watermark_path" env:"STORAGE_WATERMARK_PATH" validate:"required"`
}

// ProcessingConfig содержит размеры по умолчанию для обработки изображений.
type ProcessingConfig struct {
	Resize    ResizeCfg    `yaml:"resize"`
	Thumbnail ThumbnailCfg `yaml:"thumbnail"`
}

// ResizeCfg содержит размеры по умолчанию для операции resize.
type ResizeCfg struct {
	Width  int `yaml:"width"  env:"RESIZE_WIDTH"  validate:"required,min=1"`
	Height int `yaml:"height" env:"RESIZE_HEIGHT" validate:"required,min=1"`
}

// ThumbnailCfg содержит размеры по умолчанию для миниатюры.
type ThumbnailCfg struct {
	Width  int `yaml:"width"  env:"THUMBNAIL_WIDTH"  validate:"required,min=1"`
	Height int `yaml:"height" env:"THUMBNAIL_HEIGHT" validate:"required,min=1"`
}

// LogConfig содержит настройки логирования.
type LogConfig struct {
	Level string `yaml:"level" env:"LOG_LEVEL" validate:"required"`
	Env   string `yaml:"env"   env:"APP_ENV"   validate:"required"`
}

// Load читает и валидирует конфигурацию из указанного YAML-файла.
func Load(configPath string) (*Config, error) {
	var cfg Config

	if err := cleanenvport.LoadPath(configPath, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
