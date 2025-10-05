# API Usage Examples - Go Scheduler (Hybrid)

## Setup

```bash
# Clone repository
git clone https://github.com/rizqitaufiqf/go-scheduler.git
cd go-scheduler

# Copy environment file
cp .env.example .env

# Start services
docker-compose up -d

# Check logs
docker-compose logs -f app
```

## Akses Services

- **API Server**: http://localhost:8080/api/v1
- **Swagger UI**: http://localhost:8080/swagger/index.html
- **Asynqmon (Web UI)**: http://localhost:8081
- **Health Check**: http://localhost:8080/health

## 1. Schedule a Task (for a specific time)

### Example: Product Create Task
```bash
curl -X POST http://localhost:8080/api/v1/scheduler/tasks -v \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "PRODUCT",
    "action": "CREATE",
    "scheduled_at": "2025-10-20T15:30:00+07:00",
    "priority": "high",
    "max_retries": 5,
    "payload": {
      "name": "Laptop Gaming ROG",
      "price": 25000000,
      "stock": 10,
      "description": "High-end gaming laptop"
    }
  }'
```

### Email Send Task (Scheduled)
```bash
curl -X POST http://localhost:8080/api/v1/scheduler/tasks -v \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "email",
    "action": "send",
    "scheduled_at": "2025-10-20T09:00:00+07:00",
    "priority": "default",
    "max_retries": 3,
    "payload": {
      "to": ["user@example.com"],
      "subject": "Welcome Email",
      "body": "Thank you for signing up!",
      "template": "welcome"
    }
  }'
```

## 2. Enqueue Task (eksekusi segera)

### Product Update Task
```bash
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/enqueue -v \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "PRODUCT",
    "action": "UPDATE",
    "priority": "default",
    "max_retries": 5,
    "payload": {
      "id": 123,
      "name": "Updated Product Name",
      "price": 199000
    }
  }'
```

### Order Processing Task
```bash
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/enqueue -v \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "order",
    "action": "process",
    "priority": "critical",
    "max_retries": 3,
    "payload": {
      "order_id": "ORD-12345",
      "amount": 500000,
      "customer_id": 789
    }
  }'
```

### Report Generation Task
```bash
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/enqueue -v \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "report",
    "action": "generate",
    "priority": "low",
    "max_retries": 2,
    "payload": {
      "type": "sales",
      "format": "pdf",
      "date_from": "2025-01-01",
      "date_to": "2025-01-31",
      "recipients": ["manager@company.com"]
    }
  }'
```

## 3. List Tasks

### List Pending Tasks
Catatan: Endpoint ini mengambil data dari Redis (via Asynq) untuk status real-time.
```bash
curl "http://localhost:8080/api/v1/scheduler/tasks?queue=default&status=pending"
```

### List Scheduled Tasks
```bash
curl "http://localhost:8080/api/v1/scheduler/tasks?queue=high&status=scheduled&page=1&page_size=20"
```

### List Retry Tasks
```bash
curl "http://localhost:8080/api/v1/scheduler/tasks?queue=default&status=retry"
```

### List Archived Tasks (Completed/Failed)
```bash
curl "http://localhost:8080/api/v1/scheduler/tasks?queue=default&status=archived"
```

## 4. Get Task Info

```bash
# Format: /tasks/{queue}/{task_id}
curl http://localhost:8080/api/v1/scheduler/tasks/default/01HQX7Z8K9M3N4P5Q6R7S8T9UV
```

## 5. Cancel Task

```bash
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/default/01HQX7Z8K9M3N4P5Q6R7S8T9UV/cancel
```

## 6. Get Queue Statistics

```bash
curl http://localhost:8080/api/v1/scheduler/stats/queues
```

Response:
```json
{
  "critical": {
    "size": 5,
    "pending": 3,
    "active": 2,
    "scheduled": 10,
    "retry": 1,
    "archived": 50,
    "processed": 1000,
    "failed": 15,
    "paused": false
  },
  "default": {
    "size": 20,
    "pending": 15,
    "active": 5,
    ...
  }
}
```

## 7. Priority Queues

Asynq mendukung multiple queues dengan priority berbeda:

```bash
# Critical priority (highest)
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/enqueue \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "order",
    "action": "process",
    "priority": "critical",
    "payload": {...}
  }'

# High priority
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/enqueue \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "email",
    "action": "send",
    "priority": "high",
    "payload": {...}
  }'

# Default priority
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/enqueue \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "PRODUCT",
    "action": "CREATE",
    "priority": "default",
    "payload": {...}
  }'

# Low priority
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/enqueue \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "data",
    "action": "export",
    "priority": "low",
    "payload": {...}
  }'

# Batch priority (lowest)
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/enqueue \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "email",
    "action": "batch",
    "priority": "batch",
    "payload": {...}
  }'
```

## 9. Monitoring with Asynqmon

Open your browser and go to: [http://localhost:8081](http://localhost:8081)

Features in Asynqmon:
- Real-time task monitoring  
- Task statistics per queue  
- Task details and payload inspection  
- Retry, archive, or delete tasks


## 10. Advanced Features (via Asynq)

### Unique Tasks (Preventing Duplicates)
To create a unique task, use the following code in your application:

```go
client.EnqueueUniqueTask(
    "product:create",
    payload,
    "product-123", // unique key
    24*time.Hour,  // TTL
)
```

### Task Aggregation
```go
client.EnqueueTask(
    "email:batch",
    payload,
    asynq.Group("daily-newsletter"),
    asynq.Aggregation(100, 5*time.Minute), // Group 100 tasks or 5 min
)
```

### Task Timeout
```go
client.EnqueueTask(
    "report:generate",
    payload,
    asynq.Timeout(10*time.Minute),
)
```

## 11. Testing the Retry Mechanism

To test the retry mechanism, create a task that is designed to fail:

```bash
# Misal processor diatur untuk fail 2x pertama
curl -X POST http://localhost:8080/api/v1/scheduler/tasks/enqueue \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "test",
    "action": "fail",
    "priority": "default",
    "max_retries": 5,
    "payload": {
      "test": "retry mechanism"
    }
  }'
```

## Troubleshooting

### Task Not Being Processed
1. Check if the worker is running: `docker-compose logs app`  
2. Check the Redis connection  
3. Ensure the queue name matches between enqueue and processor

### Task Keeps Failing
1. Check the logs for detailed error messages  
2. Verify if `max_retries` has been exhausted  
3. Monitor retries in Asynqmon

### Performance Issues
1. Increase `WORKER_CONCURRENCY`  
2. Add more worker instances  
3. Optimize the processor logic

## Conclusion

This hybrid architecture provides:

✅ **Data Resilience**: Task states are stored in PostgreSQL.  
✅ **Audit Capability**: Complete task history is kept in the database.  
✅ **Reliable Processing**: Leverages Asynq’s advanced features (retry, priority, etc.).  
✅ **Observability**: Real-time monitoring via Asynqmon and historical data via SQL.  
✅ **Self-Healing**: Automatic reconciliation mechanisms ensure consistency.

This approach is well-suited for production-grade applications that require high data reliability and complex task processing.
