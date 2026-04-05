// Package kafka оборачивает библиотеку WBF kafkav2 для данного сервиса.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	kafkav2 "github.com/wb-go/wbf/kafka/kafka-v2"
	"github.com/wb-go/wbf/logger"

	"github.com/ponchik327/ImageProcessor/internal/config"
	"github.com/ponchik327/ImageProcessor/internal/domain"
)

// Producer оборачивает kafkav2.Producer и реализует service.TaskPublisher.
type Producer struct {
	inner *kafkav2.Producer
}

// NewProducer создаёт Producer, подключённый к брокерам из cfg.
func NewProducer(cfg *config.KafkaConfig, log logger.Logger) *Producer {
	return &Producer{
		inner: kafkav2.NewProducer(cfg.Brokers, cfg.Topic, log),
	}
}

// Publish сериализует task в JSON и отправляет его в Kafka с ID изображения в качестве ключа сообщения.
// Использование ID изображения как ключа гарантирует попадание всех сообщений одного изображения
// в одну партицию, сохраняя гарантии упорядоченности.
func (p *Producer) Publish(ctx context.Context, task *domain.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("kafka producer: marshal task: %w", err)
	}

	key := []byte(task.ImageID.String())

	if err = p.inner.Send(ctx, key, data); err != nil {
		return fmt.Errorf("kafka producer: send: %w", err)
	}

	return nil
}

// Close завершает работу нижележащего Kafka writer.
func (p *Producer) Close() error {
	return p.inner.Close()
}
