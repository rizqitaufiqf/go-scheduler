# Go Scheduler dengan Arsitektur Hibrida (PostgreSQL + Asynq)

## Ringkasan Arsitektur

Proyek ini mengimplementasikan sistem penjadwalan tugas (*task scheduler*) yang andal di Go dengan menggunakan **arsitektur hibrida**. Arsitektur ini menggabungkan keunggulan dari dua dunia:

1.  **PostgreSQL sebagai *Single Source of Truth***: Semua metadata dan status tugas (seperti `scheduled`, `processing`, `completed`, `paused`) disimpan secara persisten di database. Ini memungkinkan audit trail yang lengkap, kemampuan query yang kompleks, dan jaminan integritas data.
2.  **Asynq sebagai *Robust Task Processor***: Asynq digunakan untuk menangani eksekusi tugas, termasuk antrian prioritas, mekanisme *retry* otomatis, *concurrency control*, dan pemantauan melalui UI web (Asynqmon).

### Alur Kerja

```
┌─────────────┐   1. Create Task   ┌───────────────┐   2. Enqueue Task   ┌─────────────┐
│   Client    │───────────────────▶│  PostgreSQL   │────────────────────▶│    Redis    │
│ (API Call)  │                    │ (Source of Truth) │                     │   (Broker)  │
└─────────────┘                    └───────┬───────┘                     └──────┬──────┘
                                           │ 5. Update Status                  │ 3. Process Task
                                           │                                   │
                                           └───────────────────────────────────▼
                                                                        ┌─────────────┐
                                                                        │   Worker    │
                                                                        │  (Asynq)    │
                                                                        └─────────────┘
```

### Mekanisme Rekonsiliasi

Untuk mengatasi potensi inkonsistensi antara database dan Redis (misalnya, jika server mati setelah menyimpan ke DB tetapi sebelum *enqueue* ke Redis), sistem ini dilengkapi dengan **proses rekonsiliasi**:
-   **Saat Startup**: Worker akan memindai tugas yang "seharusnya aktif" di DB (`pending`, `retrying`, `processing` yang macet) dan menjadwalkannya kembali ke Asynq jika tidak ditemukan di Redis.
-   **Secara Periodik**: Tugas rekonsiliasi berjalan secara berkala untuk memastikan konsistensi jangka panjang.

## Struktur Direktori

```plaintext
go-scheduler/
├── config/                 # Environment configuration loading
├── database/               # Database and Redis initialization
├── docs/                   # Swagger documentation files (auto-generated)
├── dto/                    # Data Transfer Objects (for API and database models)
├── handler/                # HTTP handlers for API endpoints
├── repository/             # Data access logic (e.g., scheduling a task)
├── router/                 # Gin router setup
├── sql/                    # Init Sql
├── task/                   # Task processing logic
├── worker/                 # Background worker
├── .env.example            # Example environment variables
├── docker-compose.yml      # Docker services definition
├── Dockerfile              # Docker build instructions for the Go app
├── go.mod                  # Go module dependencies
└── main.go                 # Application entrypoint
├── USAGE_EXAMPLE.md        # Contoh penggunaan API dengan cURL
```

## Fitur Utama Asynq

### 1. **Automatic Retry dengan Berbagai Strategi**
```go
// Exponential backoff
asynq.MaxRetry(10)
asynq.Timeout(5 * time.Minute)

// Custom retry delay
asynq.Retention(24 * time.Hour) // Keep completed tasks
```

### 2. **Task Prioritization**
```go
// High priority queue
client.Enqueue(task, asynq.Queue("critical"))

// Medium priority
client.Enqueue(task, asynq.Queue("default"))

// Low priority
client.Enqueue(task, asynq.Queue("low"))
```

### 3. **Scheduled & Delayed Tasks**
```go
// Process in 1 hour
client.Enqueue(task, asynq.ProcessIn(1*time.Hour))

// Process at specific time
client.Enqueue(task, asynq.ProcessAt(scheduledTime))
```

### 4. **Unique Tasks (Deduplication)**
```go
// Prevent duplicate tasks
client.Enqueue(task, 
    asynq.TaskID("unique-id"),
    asynq.Unique(24*time.Hour),
)
```

### 5. **Task Aggregation**
```go
// Group similar tasks
client.Enqueue(task, 
    asynq.Group("product:create"),
    asynq.Aggregation(10, 5*time.Minute), // 10 tasks or 5 min
)
```

### 6. **Monitoring & Observability**
- **Asynqmon**: Web UI untuk monitoring
- **Prometheus Metrics**: Built-in metrics
- **CLI Tools**: Inspect & manage tasks
- **Logging**: Structured logging built-in

### 7. **Graceful Shutdown**
```go
// Automatically handles:
// - Finishing in-progress tasks
// - Returning pending tasks to queue
// - Clean shutdown on signals
```

## Perbedaan Kunci

### Redis Usage

**Scheduler Custom:**
```
- tasks:pending (sorted set dengan score = scheduled_at)
- task:locks:{id} (string dengan TTL)
- Manual ZADD, ZRANGEBYSCORE, SETNX
```

**Asynq:**
```
- asynq:queues:{queue} (list untuk ready tasks)
- asynq:scheduled (sorted set dengan score = process_at)
- asynq:retry (sorted set untuk retry)
- asynq:archived (completed/failed tasks)
- asynq:lease (atomic lease dengan Lua scripts)
```

### Database Usage

**Scheduler Custom:**
- Database sebagai single source of truth
- Redis untuk queueing
- Perlu reconciliation logic

**Asynq:**
- Redis sebagai source of truth
- Database opsional (untuk audit/reporting)
- Tidak perlu reconciliation

### Failure Handling

**Scheduler Custom:**
```go
// Manual retry logic
if task.Attempt < task.MaxRetries {
    nextDelay := calculateBackoff(task.Attempt)
    task.ScheduledAt = time.Now().Add(nextDelay)
    task.Status = "retrying"
    // Enqueue ke Redis
}
```

**Asynq:**
```go
// Built-in retry dengan error handling
func ProcessTask(ctx context.Context, t *asynq.Task) error {
    if err := doWork(); err != nil {
        return err // Asynq handles retry automatically
    }
    return nil
}
```

## Kelebihan Asynq

### ✅ Pros
1. **Production-ready**: Battle-tested, digunakan banyak perusahaan
2. **Less code**: Tidak perlu implement worker pool, locking, retry logic
3. **Better tooling**: Web UI, CLI, metrics out of the box
4. **Advanced features**: Aggregation, rate limiting, unique tasks
5. **Active development**: Regular updates & bug fixes
6. **Good documentation**: Comprehensive docs & examples
7. **Atomic operations**: Lua scripts untuk consistency
8. **Graceful shutdown**: Handle signals properly

### ⚠️ Cons
1. **Redis-centric**: Redis adalah single source of truth
2. **Less flexibility**: Terikat dengan Asynq patterns
3. **Learning curve**: Perlu pelajari Asynq conventions
4. **Opinionated**: Struktur data & flow sudah defined

## Kelebihan Scheduler Custom Anda

### ✅ Pros
1. **Database-centric**: PostgreSQL sebagai source of truth
2. **Full control**: Bisa customize setiap aspek
3. **Flexible schema**: Bisa tambah field custom
4. **Query capability**: Bisa query tasks kompleks di DB
5. **Audit trail**: History lengkap di database
6. **Custom logic**: Bisa implement business rules spesifik

### ⚠️ Cons
1. **More maintenance**: Perlu maintain worker logic sendiri
2. **More code**: Lebih banyak boilerplate
3. **Testing complexity**: Perlu test edge cases sendiri
4. **No built-in monitoring**: Perlu build sendiri
5. **Reconciliation needed**: Perlu handle crash recovery

## Rekomendasi

### Gunakan **Scheduler Custom** jika:
- Perlu database sebagai source of truth
- Butuh query capability kompleks
- Butuh audit trail lengkap di database
- Perlu custom business logic yang kompleks
- Team sudah familiar dengan codebase

### Gunakan **Asynq** jika:
- Ingin solution yang battle-tested
- Perlu monitoring & observability out of the box
- Ingin less maintenance overhead
- Butuh advanced features (aggregation, rate limiting)
- Starting new project
- Team kecil dengan limited resources

## Migration Path

Jika ingin migrate dari custom ke Asynq:

1. **Hybrid Approach**: 
   - Keep database untuk audit
   - Use Asynq untuk processing
   - Sync status dari Asynq ke database

2. **Gradual Migration**:
   - Start dengan task types baru di Asynq
   - Legacy tasks tetap di custom scheduler
   - Migrate gradually

3. **Full Migration**:
   - Move semua task processing ke Asynq
   - Use database hanya untuk reporting
   - Implement event sourcing jika perlu audit

## Setup Instructions

Lihat file-file implementasi berikut:
- `main.go` - Entry point
- `tasks/client.go` - Asynq client setup
- `worker/server.go` - Asynq server setup
- `tasks/processor.go` - Task processors
- `handler/task_handler.go` - API handlers

## Resources

- Asynq Documentation: https://github.com/hibiken/asynq
- Asynqmon (Web UI): https://github.com/hibiken/asynqmon
- Examples: https://github.com/hibiken/asynq/tree/master/examples

## Kesimpulan

Kedua approach punya kelebihan masing-masing. Custom scheduler memberikan flexibility & control penuh, sementara Asynq memberikan production-ready solution dengan less code & better tooling.

Pilihan tergantung pada:
- Requirements spesifik project
- Team size & expertise
- Maintenance capacity
- Timeline & budget