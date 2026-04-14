//go:build integration

package kafka

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kafkacontainer "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/wb-go/wbf/logger"

	"github.com/ponchik327/ImageProcessor/internal/config"
	"github.com/ponchik327/ImageProcessor/internal/domain"
)

var testBroker string

// createTopic создаёт Kafka-топик напрямую через внешний адрес брокера
// (без перехода на внутренний адрес контроллера) и ждёт готовности лидера.
func createTopic(t *testing.T, topic string) {
	t.Helper()

	// Создаём топик через прямое подключение к внешнему адресу брокера.
	// conn.Controller() вернул бы внутренний адрес контейнера, поэтому
	// используем тот же conn для CreateTopics.
	conn, err := kafkago.Dial("tcp", testBroker)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.CreateTopics(kafkago.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	require.NoError(t, err)

	// Ждём, пока лидер партиции станет доступен для записи.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		leaderConn, dialErr := kafkago.DialLeader(context.Background(), "tcp", testBroker, topic, 0)
		if dialErr == nil {
			_ = leaderConn.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for topic %q leader to become available", topic)
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	kContainer, err := kafkacontainer.Run(ctx, "confluentinc/confluent-local:7.5.0")
	if err != nil {
		panic("start kafka container: " + err.Error())
	}
	defer func() { _ = kContainer.Terminate(ctx) }()

	brokers, err := kContainer.Brokers(ctx)
	if err != nil {
		panic("get kafka brokers: " + err.Error())
	}
	testBroker = brokers[0]

	os.Exit(m.Run())
}

func publishWithRetry(t *testing.T, ctx context.Context, p *Producer, task *domain.Task, attempts int, delay time.Duration) {
	t.Helper()
	var lastErr error
	for i := range attempts {
		lastErr = p.Publish(ctx, task)
		if lastErr == nil {
			return
		}
		if i < attempts-1 {
			time.Sleep(delay)
		}
	}
	require.NoError(t, lastErr)
}

func newTestLogger(t *testing.T) logger.Logger {
	t.Helper()
	log, err := logger.InitLogger(logger.ZapEngine, "test", "test", logger.WithLevel(logger.ErrorLevel))
	require.NoError(t, err)
	return log
}

// ─── Publish ──────────────────────────────────────────────────────────────────

func TestPublish_HappyPath(t *testing.T) {
	ctx := context.Background()
	topic := "test-publish-" + uuid.NewString()

	log := newTestLogger(t)
	cfg := &config.KafkaConfig{
		Brokers: []string{testBroker},
		Topic:   topic,
	}

	createTopic(t, topic)

	producer := NewProducer(cfg, log)
	t.Cleanup(func() { _ = producer.Close() })

	task := &domain.Task{
		ImageID:    uuid.New(),
		SourcePath: "/storage/original.jpg",
		MIMEType:   "image/jpeg",
		Operations: []domain.TaskOperation{
			{Type: "resize", Params: map[string]any{"width": float64(800), "height": float64(600)}},
		},
	}

	require.NoError(t, producer.Publish(ctx, task))

	// Читаем сообщение напрямую через kafka-go reader
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:   []string{testBroker},
		Topic:     topic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	t.Cleanup(func() { _ = reader.Close() })

	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	msg, err := reader.ReadMessage(readCtx)
	require.NoError(t, err)

	// Ключ — строковое представление ImageID
	assert.Equal(t, task.ImageID.String(), string(msg.Key))

	// Тело — валидный JSON, десериализуется обратно в domain.Task
	var decoded domain.Task
	require.NoError(t, json.Unmarshal(msg.Value, &decoded))
	assert.Equal(t, task.ImageID, decoded.ImageID)
	assert.Equal(t, task.SourcePath, decoded.SourcePath)
	assert.Equal(t, task.MIMEType, decoded.MIMEType)
	assert.Len(t, decoded.Operations, 1)
	assert.Equal(t, "resize", decoded.Operations[0].Type)
}

func TestPublish_CancelledContext(t *testing.T) {
	topic := "test-cancel-" + uuid.NewString()
	log := newTestLogger(t)

	cfg := &config.KafkaConfig{
		Brokers: []string{testBroker},
		Topic:   topic,
	}

	producer := NewProducer(cfg, log)
	t.Cleanup(func() { _ = producer.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // сразу отменяем

	task := &domain.Task{
		ImageID:    uuid.New(),
		SourcePath: "/path",
		MIMEType:   "image/jpeg",
	}

	err := producer.Publish(ctx, task)
	assert.Error(t, err)
}

func TestPublish_MultipleMessages_OrderedByKey(t *testing.T) {
	ctx := context.Background()
	topic := "test-multi-" + uuid.NewString()
	imageID := uuid.New()

	log := newTestLogger(t)
	cfg := &config.KafkaConfig{
		Brokers: []string{testBroker},
		Topic:   topic,
	}

	createTopic(t, topic)

	producer := NewProducer(cfg, log)
	t.Cleanup(func() { _ = producer.Close() })

	task := &domain.Task{
		ImageID:    imageID,
		SourcePath: "/path",
		MIMEType:   "image/jpeg",
	}

	// Первая публикация может упасть с "Unknown Topic Or Partition" пока
	// внутренний кэш метаданных Writer-а не обновится. Повторяем до готовности.
	publishWithRetry(t, ctx, producer, task, 10, 500*time.Millisecond)

	// Ещё 2 сообщения — метаданные уже есть, ошибок быть не должно.
	for range 2 {
		require.NoError(t, producer.Publish(ctx, task))
	}

	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:   []string{testBroker},
		Topic:     topic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	t.Cleanup(func() { _ = reader.Close() })

	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for range 3 {
		msg, err := reader.ReadMessage(readCtx)
		require.NoError(t, err)
		assert.Equal(t, imageID.String(), string(msg.Key))
	}
}
