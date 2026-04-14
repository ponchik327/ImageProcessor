# ImageProcessor

Сервис загрузки и асинхронной обработки изображений. Принимает файлы через HTTP, публикует задачи в Kafka, воркер создаёт варианты: resize, thumbnail, watermark.

## Запуск

```bash
cp config.example.yaml config.yaml
docker compose up --build
```

Сервис доступен на `http://localhost:8080`.

Все параметры конфигурации описаны и прокомментированы в [config.example.yaml](config.example.yaml) — для запуска достаточно его скопировать и переименовать.

Переменные окружения переопределяют значения из файла:

| Переменная             | Параметр конфига          |
|------------------------|---------------------------|
| `DATABASE_DSN`         | `database.dsn`            |
| `KAFKA_BROKERS`        | `kafka.brokers`           |
| `STORAGE_BASE_PATH`    | `storage.base_path`       |
| `STORAGE_WATERMARK_PATH` | `storage.watermark_path` |

---

## API

### POST /upload

Загрузить изображение. Возвращает объект сразу после сохранения; обработка происходит асинхронно.

**Request** — `multipart/form-data`, поле `file`, максимум 32 MB.

```bash
curl -F "file=@photo.jpg" http://localhost:8080/upload
```

**Response 201**
```json
{
  "id": "d290f1ee-6c54-4b01-90e6-d701748f0851",
  "original_name": "photo.jpg",
  "mime_type": "image/jpeg",
  "status": "pending",
  "created_at": "2024-04-14T10:00:00Z"
}
```

---

### GET /images

Список всех изображений.

**Query params**

| Параметр | Тип | По умолчанию | Максимум |
|----------|-----|:---:|:---:|
| `limit`  | int | 50  | 200 |
| `offset` | int | 0   | —   |

**Response 200** — массив объектов изображений (см. формат ниже).

---

### GET /image/{id}

Информация об изображении и его вариантах.

**Response 200**
```json
{
  "id": "d290f1ee-6c54-4b01-90e6-d701748f0851",
  "original_name": "photo.jpg",
  "mime_type": "image/jpeg",
  "status": "done",
  "error_message": null,
  "created_at": "2024-04-14T10:00:00Z",
  "updated_at": "2024-04-14T10:00:05Z",
  "variants": [
    { "type": "resize",    "url": "/image/{id}/file?variant=resize",    "width": 800, "height": 600 },
    { "type": "thumbnail", "url": "/image/{id}/file?variant=thumbnail", "width": 150, "height": 150 },
    { "type": "watermark", "url": "/image/{id}/file?variant=watermark", "width": 800, "height": 600 }
  ]
}
```

**Статусы**

| Статус       | Описание                              |
|-------------|---------------------------------------|
| `pending`    | В очереди                            |
| `processing` | Обрабатывается                       |
| `done`       | Готово, варианты доступны            |
| `failed`     | Ошибка обработки (см. `error_message`) |

---

### GET /image/{id}/file?variant={type}

Скачать обработанный вариант файла.

**Query params** — `variant`: `resize` | `thumbnail` | `watermark`

**Response 200** — бинарный файл.

**Коды ошибок**: `400` неверный variant, `404` не найдено, `409` обработка ещё не завершена.

---

### DELETE /image/{id}

Удалить изображение и все его варианты.

**Response 204** — No Content.
