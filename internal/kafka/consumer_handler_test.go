package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/wb-go/wbf/logger"

	"github.com/ponchik327/ImageProcessor/internal/domain"
	"github.com/ponchik327/ImageProcessor/internal/processor"
)

// ─── Мок ──────────────────────────────────────────────────────────────────────

type MockTaskProcessor struct{ mock.Mock }

func (m *MockTaskProcessor) ProcessTask(ctx context.Context, task *domain.Task, registry processor.Registry) error {
	return m.Called(ctx, task, registry).Error(0)
}

// ─── Хелпер ───────────────────────────────────────────────────────────────────

func newTestConsumerHandler(svc *MockTaskProcessor) *ConsumerHandler {
	log, _ := logger.InitLogger(logger.ZapEngine, "test", "test", logger.WithLevel(logger.ErrorLevel))
	return &ConsumerHandler{svc: svc, registry: processor.Registry{}, log: log}
}

// ─── Тесты ────────────────────────────────────────────────────────────────────

func TestHandle_HappyPath(t *testing.T) {
	svc := &MockTaskProcessor{}
	h := newTestConsumerHandler(svc)

	// Params используют float64, как при JSON-декодировании
	task := &domain.Task{
		ImageID:    uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		SourcePath: "/storage/original.jpg",
		MIMEType:   "image/jpeg",
		Operations: []domain.TaskOperation{
			{Type: "resize", Params: map[string]any{"width": float64(800)}},
		},
	}
	svc.On("ProcessTask", mock.Anything, task, h.registry).Return(nil)

	data, err := json.Marshal(task)
	require.NoError(t, err)

	require.NoError(t, h.Handle(context.Background(), kafkago.Message{Value: data}))
	svc.AssertExpectations(t)
}

func TestHandle_InvalidJSON(t *testing.T) {
	svc := &MockTaskProcessor{}
	h := newTestConsumerHandler(svc)

	err := h.Handle(context.Background(), kafkago.Message{Value: []byte("not-json{{{")})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal task")
	svc.AssertNotCalled(t, "ProcessTask")
}

func TestHandle_ProcessTaskError(t *testing.T) {
	svc := &MockTaskProcessor{}
	h := newTestConsumerHandler(svc)

	imageID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	task := &domain.Task{ImageID: imageID, SourcePath: "/path", MIMEType: "image/jpeg"}
	svc.On("ProcessTask", mock.Anything, task, h.registry).Return(errors.New("processing failed"))

	data, err := json.Marshal(task)
	require.NoError(t, err)

	handleErr := h.Handle(context.Background(), kafkago.Message{Value: data})

	require.Error(t, handleErr)
	assert.Contains(t, handleErr.Error(), imageID.String())
}
