package kafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/wb-go/wbf/logger"
)

// silentInfoLogger оборачивает logger.Logger и подавляет вызовы LogAttrs на уровне
// InfoLevel и ниже. Используется для заглушения внутренних сообщений kafka-go об истечении
// таймаута опроса ("no messages received …"), которые wbf-потребитель отправляет через
// LogAttrs на уровне Info, при этом продолжая пересылать записи Warn/Error.
//
// Кроме того, транзитные ошибки запуска Kafka (код ошибки [15] "Group
// Coordinator Not Available") понижаются с ERROR до WARN, поскольку
// kafka-go повторяет их автоматически и они всегда разрешаются после того,
// как брокер завершает выбор лидера партиции для __consumer_offsets.
type silentInfoLogger struct {
	logger.Logger
}

// SilentInfoLogger оборачивает l и отбрасывает все вызовы LogAttrs на уровне InfoLevel и ниже.
func SilentInfoLogger(l logger.Logger) logger.Logger {
	return silentInfoLogger{l}
}

func (l silentInfoLogger) LogAttrs(ctx context.Context, level logger.Level, msg string, attrs ...logger.Attr) {
	if level <= logger.InfoLevel {
		return
	}
	if level == logger.ErrorLevel && isTransientCoordinatorError(attrs) {
		level = logger.WarnLevel
	}
	l.Logger.LogAttrs(ctx, level, msg, attrs...)
}

// isTransientCoordinatorError сообщает, содержат ли attrs транзитную ошибку запуска Kafka,
// которую kafka-go повторяет автоматически:
//   - [15] Group Coordinator Not Available — __consumer_offsets ещё не создан
//   - [5]  Leader Not Available            — выбор лидера партиции в процессе
//
// Оба случая ожидаемы при холодном старте брокера и разрешаются без вмешательства.
func isTransientCoordinatorError(attrs []logger.Attr) bool {
	for _, a := range attrs {
		if a.Key == "error" {
			v := fmt.Sprint(a.Value)
			return strings.Contains(v, "[15]") ||
				strings.Contains(v, "Group Coordinator Not Available") ||
				strings.Contains(v, "[5]") ||
				strings.Contains(v, "Leader Not Available")
		}
	}
	return false
}
