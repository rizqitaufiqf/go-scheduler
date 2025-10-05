# Go Scheduler with a Hybrid Architecture (PostgreSQL + Asynq)

## Architectural Overview

This project implements a reliable task scheduler in Go using a **hybrid architecture**. This architecture combines the best of two worlds:

1.  **PostgreSQL as a *Single Source of Truth***: All task metadata and statuses (such as `scheduled`, `processing`, `completed`, `paused`) are stored persistently in the database. This allows for a complete audit trail, complex query capabilities, and data integrity guarantees.
2.  **Asynq as a *Robust Task Processor***: Asynq is used to handle task execution, including priority queues, automatic retry mechanisms, concurrency control, and monitoring via a web UI (Asynqmon).

### Workflow

```
┌─────────────┐   1. Create Task   ┌───────────────┐   2. Enqueue Task   ┌─────────────┐
│   Client    │───────────────────▶│  PostgreSQL   │────────────────────▶│    Redis    │
│ (API Call)  │                    │(Source of Truth)│                   │   (Broker)  │
└─────────────┘                    └───────┬───────┘                     └──────┬──────┘
                                           │ 5. Update Status                  │ 3. Process Task
                                           │                                   │
                                           └───────────────────────────────────▼
                                                                        ┌─────────────┐
                                                                        │   Worker    │
                                                                        │  (Asynq)    │
                                                                        └─────────────┘
```

### Reconciliation Mechanism

To address potential inconsistencies between the database and Redis (e.g., if the server crashes after saving to the DB but before enqueuing to Redis), this system includes a **reconciliation process**:
-   **On Startup**: The worker scans for tasks that "should be active" in the DB (`pending`, `retrying`, stale `processing` tasks) and re-enqueues them to Asynq if they are not found in Redis.
-   **Periodically**: A reconciliation task runs at regular intervals to ensure long-term consistency.

## Directory Structure

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
├── main.go                 # Application entrypoint
└── USAGE_EXAMPLE.md        # API usage examples with cURL
```

## Key Asynq Features Used

### 1. **Automatic Retry with Various Strategies**
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
- tasks:pending (sorted set with score = scheduled_at)
- task:locks:{id} (string with TTL)
- Manual ZADD, ZRANGEBYSCORE, SETNX
```

**Asynq:**
```
- asynq:queues:{queue} (list for ready tasks)
- asynq:scheduled (sorted set with score = process_at)
- asynq:retry (sorted set for retries)
- asynq:archived (completed/failed tasks)
- asynq:lease (atomic leasing with Lua scripts)
```

### Database Usage

**Scheduler Custom:**
- Database as the single source of truth
- Redis used for queueing
- Reconciliation logic required

**Asynq:**
- Redis as the source of truth
- Database optional (for auditing/reporting)
- No reconciliation required

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

# Advantages of Asynq vs Custom Scheduler

## Advantages of Asynq

### ✅ Pros
1. **Production-ready**: Battle-tested and used by many companies  
2. **Less code**: No need to implement worker pools, locking, or retry logic  
3. **Better tooling**: Web UI, CLI, and metrics out of the box  
4. **Advanced features**: Aggregation, rate limiting, unique tasks  
5. **Active development**: Regular updates and bug fixes  
6. **Good documentation**: Comprehensive docs and examples  
7. **Atomic operations**: Lua scripts ensure consistency  
8. **Graceful shutdown**: Properly handles signals  

### ⚠️ Cons
1. **Redis-centric**: Redis is the single source of truth  
2. **Less flexibility**: Tied to Asynq’s patterns  
3. **Learning curve**: Requires learning Asynq conventions  
4. **Opinionated**: Data structures and flow are predefined  

---

## Advantages of Your Custom Scheduler

### ✅ Pros
1. **Database-centric**: PostgreSQL as the source of truth  
2. **Full control**: You can customize every aspect  
3. **Flexible schema**: Easily add custom fields  
4. **Query capability**: Perform complex task queries directly in the database  
5. **Audit trail**: Complete history stored in the database  
6. **Custom logic**: Implement specific business rules  

### ⚠️ Cons
1. **More maintenance**: You need to maintain the worker logic yourself  
2. **More code**: Involves more boilerplate  
3. **Testing complexity**: You need to test edge cases manually  
4. **No built-in monitoring**: You must build monitoring tools yourself  
5. **Reconciliation needed**: You must handle crash recovery  

---

## Recommendation

### Use **Custom Scheduler** if:
- You need the database to be the source of truth  
- You require complex querying capabilities  
- You need a complete audit trail in the database  
- You have complex custom business logic  
- Your team is already familiar with the existing codebase  

### Use **Asynq** if:
- You want a battle-tested solution  
- You need monitoring and observability out of the box  
- You prefer less maintenance overhead  
- You need advanced features (aggregation, rate limiting)  
- You’re starting a new project  
- You have a small team with limited resources  

---

## Migration Path

If you want to migrate from a custom scheduler to Asynq:

### 1. Hybrid Approach
- Keep the database for auditing  
- Use Asynq for processing  
- Sync task status from Asynq to the database  

### 2. Gradual Migration
- Start using Asynq for new task types  
- Keep legacy tasks in the custom scheduler  
- Migrate gradually over time  

### 3. Full Migration
- Move all task processing to Asynq  
- Use the database only for reporting  
- Implement event sourcing if auditing is required  

---

## Setup Instructions

Check the following implementation files:

- `main.go` – Entry point  
- `tasks/client.go` – Asynq client setup  
- `worker/server.go` – Asynq server setup  
- `tasks/processor.go` – Task processors  
- `handler/task_handler.go` – API handlers  

---

## Resources

- [Asynq Documentation](https://github.com/hibiken/asynq)  
- [Asynqmon (Web UI)](https://github.com/hibiken/asynqmon)  
- [Examples](https://github.com/hibiken/asynq/tree/master/examples)  

---

## Conclusion

Both approaches have their own advantages.  
A **custom scheduler** offers full flexibility and control, while **Asynq** provides a **production-ready solution** with less code and better tooling.

The choice depends on:
- Specific project requirements  
- Team size and expertise  
- Maintenance capacity  
- Timeline and budget