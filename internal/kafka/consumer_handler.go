package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/wb-go/wbf/logger"

	"github.com/ponchik327/ImageProcessor/internal/domain"
	"github.com/ponchik327/ImageProcessor/internal/processor"
)

// imageService — минимальный контракт сервиса, необходимый ConsumerHandler.
type imageService interface {
	ProcessTask(ctx context.Context, task *domain.Task, registry processor.Registry) error
}

// ConsumerHandler декодирует Kafka-сообщения и передаёт их сервису для обработки.
type ConsumerHandler struct {
	svc      imageService
	registry processor.Registry
	log      logger.Logger
}

// NewConsumerHandler создаёт ConsumerHandler с заданным сервисом и реестром обработчиков.
func NewConsumerHandler(svc imageService, registry processor.Registry, log logger.Logger) *ConsumerHandler {
	return &ConsumerHandler{svc: svc, registry: registry, log: log}
}

// Handle реализует сигнатуру обработчика kafkav2.
// Десериализует тело сообщения в domain.Task и вызывает service.ProcessTask.
// Возврат ненулевой ошибки сигнализирует kafkav2.Processor о необходимости повтора или отправки в DLQ.
func (h *ConsumerHandler) Handle(ctx context.Context, msg kafkago.Message) error {
	var task domain.Task

	if err := json.Unmarshal(msg.Value, &task); err != nil {
		return fmt.Errorf("consumer handler: unmarshal task: %w", err)
	}

	h.log.Info("consumer handler: processing task", "image_id", task.ImageID)

	if err := h.svc.ProcessTask(ctx, &task, h.registry); err != nil {
		return fmt.Errorf("consumer handler: process task %s: %w", task.ImageID, err)
	}

	return nil
}
