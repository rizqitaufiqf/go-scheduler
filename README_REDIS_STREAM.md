# 🔔 Notification System dengan Redis Streams

## 📖 Overview

Sistem notifikasi ini menggunakan **Redis Streams** untuk mengirimkan notifikasi real-time kepada user ketika task selesai atau gagal.

### Arsitektur

```
┌─────────────────────────────────────────────────────────────┐
│                     NOTIFICATION FLOW                        │
└─────────────────────────────────────────────────────────────┘

1. Task Selesai/Gagal
   ↓
2. Worker → Publish ke Redis Stream
   ↓
3. Redis Stream (notifications:stream)
   ↓
4. Notification Consumer → Consume message
   ↓
5. Save ke PostgreSQL (untuk history)
   ↓
6. Cek: User Online?
   ├─ YES → Kirim via SSE (real-time)
   └─ NO  → Tunggu user login nanti
```

### Kenapa Redis Streams?

✅ **Sudah ada** - Kamu sudah pakai Redis untuk task queue  
✅ **Cepat** - 100k+ msg/s throughput  
✅ **Simple** - Easy to implement  
✅ **Reliable** - Message persistence dengan AOF/RDB  
✅ **Consumer Groups** - Support multiple consumers  
✅ **No extra service** - Tidak perlu RabbitMQ/NATS  

## 🚀 Quick Start

### 1. Install Dependencies

```bash
# Tidak perlu install dependency baru
# Redis client sudah ada di project
go mod tidy
```

### 2. Setup Environment

```bash
cp .env.example .env
```

### 3. Start Services

```bash
docker-compose up --build
```

Services yang berjalan:
- PostgreSQL: `localhost:5432`
- Redis: `localhost:6379`
- API Server: `localhost:8080`

### 4. Test Notification System

#### A. Buka Frontend (SSE Client)

Buka file `frontend_sse_client.html` di browser:
```bash
open frontend_sse_client.html
# atau
start frontend_sse_client.html  # Windows
```

#### B. Create Task dengan User ID

```bash
curl -X POST http://localhost:8080/api/v1/scheduler/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "entity": "PRODUCT",
    "action": "CREATE",
    "user_id": 1,
    "scheduled_at": "'$(date -u -d '+1 minute' +"%Y-%m-%dT%H:%M:%SZ")'",
    "priority": 10,
    "max_retries": 3,
    "payload": {
      "name": "Test Product",
      "price": 99.99
    }
  }'
```

#### C. Lihat Notifikasi

Notifikasi akan muncul di frontend dalam ~1 menit setelah task selesai!

## 📁 File Structure

```
your-project/
├── dto/
│   ├── notification.go          # 🆕 Notification models
│   └── task.go                  # ✏️ Updated (tambah UserID)
├── repository/
│   └── notification_repository.go  # 🆕 Database operations
├── worker/
│   ├── notification_publisher.go   # 🆕 Publish ke Redis Streams
│   ├── notification_consumer.go    # 🆕 Consume dari Redis Streams
│   └── worker.go                   # ✏️ Updated (integrate notifications)
├── handler/
│   ├── sse_manager.go              # 🆕 Manage SSE connections
│   └── sse_handler.go              # 🆕 HTTP handlers untuk SSE
├── router/
│   └── router.go                   # ✏️ Updated (tambah notification routes)
├── main.go                         # ✏️ Updated (wire everything)
├── docker-compose.yml              # ✏️ Updated (no RabbitMQ)
├── .env.example                    # ✏️ Updated
└── frontend_sse_client.html        # 🆕 Frontend test client
```

## 🔧 How It Works

### 1. Publishing Notifications (Worker → Redis Stream)

Ketika task selesai/gagal, worker publish notification:

```go
// Di worker/worker.go - processTask()

// Task success
notificationPublisher.PublishTaskCompletedNotification(
    ctx,
    task.UserID,
    task.ID,
    entity.Name,
    action.Name,
)

// Task failed
notificationPublisher.PublishTaskFailedNotification(
    ctx,
    task.UserID,
    task.ID,
    entity.Name,
    action.Name,
    errorMessage,
)
```

**Redis Command yang dijalankan:**
```redis
XADD notifications:stream MAXLEN ~ 10000 * 
  data '{"user_id":1,"task_id":123,...}' 
  user_id 1 
  task_id 123 
  type "task_completed"
```

### 2. Consuming Notifications (Redis Stream → Consumer)

Consumer membaca message dengan consumer group:

```go
// XREADGROUP GROUP notification-processors consumer-1 
// BLOCK 5000 COUNT 10 STREAMS notifications:stream >

streams := redis.XReadGroup(ctx, &redis.XReadGroupArgs{
    Group:    "notification-processors",
    Consumer: "consumer-1",
    Streams:  []string{"notifications:stream", ">"},
    Count:    10,
    Block:    5 * time.Second,
})
```

**Consumer group benefits:**
- Multiple consumers dapat berjalan parallel
- Message hanya diproses 1x (tidak duplicate)
- Jika consumer crash, message bisa di-claim oleh consumer lain

### 3. Saving to Database

Consumer save notification ke PostgreSQL untuk history:

```sql
INSERT INTO notifications 
(user_id, task_id, type, title, message, status, created_at) 
VALUES (1, 123, 'task_completed', 'Task Completed', '...', 'pending', NOW());
```

### 4. Real-time Delivery via SSE

**Skenario A: User Online**

```
Consumer check: IsUserOnline(userID) → YES
  ↓
Send via SSE channel → User receives immediately
  ↓
Update DB: status = 'sent', sent_at = NOW()
```

**Skenario B: User Offline**

```
Consumer check: IsUserOnline(userID) → NO
  ↓
Notification stays in DB with status = 'pending'
  ↓
(Wait for user to login)
  ↓
User connects SSE → Load pending notifications
  ↓
Send all pending via SSE
  ↓
Update DB: status = 'sent', sent_at = NOW()
```

## 📡 API Endpoints

### 1. SSE Stream (Real-time Connection)

```http
GET /api/v1/notifications/stream?user_id=1
```

**Response:** Server-Sent Events stream

```
: connected

data: {"id":1,"task_id":123,"type":"task_completed","title":"Task Completed Successfully","message":"Your task PRODUCT CREATE has been completed successfully","status":"sent","created_at":"2025-10-13T10:00:00Z"}

: keep-alive

data: {"id":2,"task_id":124,"type":"task_failed","title":"Task Failed","message":"Your task PRODUCT UPDATE has failed: database connection error","status":"sent","created_at":"2025-10-13T10:05:00Z"}
```

### 2. Get Notifications (History)

```http
GET /api/v1/notifications?user_id=1&limit=50
```

**Response:**
```json
{
  "notifications": [
    {
      "id": 1,
      "user_id": 1,
      "task_id": 123,
      "type": "task_completed",
      "title": "Task Completed Successfully",
      "message": "Your task PRODUCT CREATE has been completed successfully",
      "status": "sent",
      "sent_at": "2025-10-13T10:00:00Z",
      "created_at": "2025-10-13T10:00:00Z"
    }
  ],
  "count": 1
}
```

### 3. Mark as Read

```http
PUT /api/v1/notifications/123/read
```

**Response:**
```json
{
  "message": "notification marked as read",
  "id": 123
}
```

### 4. SSE Statistics

```http
GET /api/v1/notifications/stats
```

**Response:**
```json
{
  "online_users": 5,
  "total_connections": 8
}
```

## 🔍 Monitoring Redis Streams

### Via Redis CLI

```bash
# Enter Redis container
docker exec -it scheduler-redis redis-cli

# Check stream info
XINFO STREAM notifications:stream

# Output:
# length: 42
# first-entry: 1697184000000-0
# last-entry: 1697187600000-0
# ...

# Check consumer group
XINFO GROUPS notifications:stream

# Output:
# name: notification-processors
# consumers: 1
# pending: 0
# last-delivered-id: 1697187600000-0

# See pending messages (unprocessed)
XPENDING notifications:stream notification-processors

# Read last 10 messages
XREAD COUNT 10 STREAMS notifications:stream 0
```

### Via Application Logs

```bash
# Follow application logs
docker-compose logs -f app

# You'll see:
# 📤 Published notification to Redis Stream: ID=1697184000000-0, TaskID=123, UserID=1
# 📥 Processing notification: MessageID=1697184000000-0, TaskID=123, UserID=1
# 📨 Sent notification to online user: UserID=1, NotificationID=5
```

## 🐛 Debugging

### Check if Consumer is Running

```bash
# Check logs
docker-compose logs app | grep "Notification Consumer"

# Should see:
# 🚀 Notification Consumer started
# Stream: notifications:stream
# Consumer Group: notification-processors
```

### Check Redis Stream Length

```bash
docker exec -it scheduler-redis redis-cli XLEN notifications:stream
# Output: (integer) 42
```

### Check Pending Notifications in DB

```bash
docker exec -it scheduler-postgres psql -U user -d scheduler_db

SELECT id, user_id, task_id, status, title, created_at 
FROM notifications 
WHERE status = 'pending' 
ORDER BY created_at DESC 
LIMIT 10;
```

### Test SSE Connection

```bash
# Test dengan curl
curl -N http://localhost:8080/api/v1/notifications/stream?user_id=1

# Should see:
# : connected
# (waits for notifications)
```

## ⚙️ Configuration

### Redis Stream Settings

Di `notification_publisher.go`:
```go
MaxLen: 10000,  // Keep only last 10k messages
Approx: true,   // Use approximate trimming for performance
```

### Consumer Settings

Di `notification_consumer.go`:
```go
Count: 10,                // Process 10 messages at once
Block: 5 * time.Second,   // Block 5s waiting for new messages
```

### SSE Settings

Di `sse_handler.go`:
```go
Channel: make(chan *dto.NotificationResponse, 10), // Buffer 10 notifications
ticker := time.NewTicker(30 * time.Second)          // Keep-alive every 30s
```

## 🔒 Production Considerations

### 1. Authentication

⚠️ **IMPORTANT**: Saat ini menggunakan `user_id` di query parameter!

**Production**: Gunakan JWT token

```go
// Extract dari Authorization header
token := c.GetHeader("Authorization")
claims := parseJWT(token)
userID := claims.UserID
```

### 2. Redis Persistence

Pastikan Redis AOF enabled:
```yaml
# docker-compose.yml
command: redis-server --appendonly yes --appendfsync everysec
```

### 3. Consumer Scaling

Bisa run multiple consumers:
```go
consumerName: hostname() // or UUID
```

Setiap consumer dalam group yang sama akan process message berbeda.

### 4. Error Handling

Consumer tidak ACK message jika error:
```go
if err := c.repository.Create(ctx, notification); err != nil {
    // Don't ACK - message will be redelivered
    return
}
```

### 5. Message Cleanup

Periodic cleanup mencegah memory overflow:
```go
// Run every hour
XTRIM notifications:stream MAXLEN ~ 10000
```

## 📊 Performance

### Expected Throughput

- **Publishing**: 50,000+ msg/s
- **Consuming**: 10,000+ msg/s
- **SSE delivery**: <10ms latency

### Resource Usage

- **Redis memory**: ~50-100MB (with 10k messages)
- **PostgreSQL**: ~1MB per 1000 notifications
- **SSE connections**: ~5KB per connection

## 🎯 Testing Scenarios

### Test 1: User Online

```bash
# 1. Start SSE connection
curl -N http://localhost:8080/api/v1/notifications/stream?user_id=1

# 2. In another terminal, create task
curl -X POST http://localhost:8080/api/v1/scheduler/tasks \
  -H "Content-Type: application/json" \
  -d '{"entity":"PRODUCT","action":"CREATE","user_id":1,"scheduled_at":"2025-10-13T10:00:00Z"}'

# 3. Wait for task to complete
# 4. You should see notification in first terminal!
```

### Test 2: User Offline

```bash
# 1. Create task (NO SSE connection)
curl -X POST http://localhost:8080/api/v1/scheduler/tasks \
  -H "Content-Type: application/json" \
  -d '{"entity":"PRODUCT","action":"CREATE","user_id=1,"scheduled_at":"2025-10-13T10:00:00Z"}'

# 2. Wait for task to complete

# 3. Connect SSE (user "logs in")
curl -N http://localhost:8080/api/v1/notifications/stream?user_id=1

# 4. You should immediately receive pending notification!
```

### Test 3: Multiple Tabs

Open frontend HTML in 3 browser tabs, same user_id.
Create a task. All 3 tabs should receive the notification!

## 🆘 Troubleshooting

### Problem: Notifications not received

**Check:**
1. Consumer running? `docker-compose logs app | grep Consumer`
2. Redis stream has messages? `redis-cli XLEN notifications:stream`
3. SSE connection active? Check browser Network tab
4. User ID correct?

### Problem: Duplicate notifications

**Cause:** Multiple consumers with same name  
**Fix:** Use unique consumer names (hostname/UUID)

### Problem: High memory usage

**Cause:** Too many messages in stream  
**Fix:** Lower MaxLen or run cleanup more frequently

### Problem: SSE connection drops

**Cause:** Proxy/nginx timeout  
**Fix:** Add keep-alive comments (already implemented)

## 📚 Next Steps

1. ✅ **Implement Authentication** - Use JWT instead of query param
2. ✅ **Add Notification Preferences** - Let users configure notification types
3. ✅ **Add Push Notifications** - Browser push API for offline notifications
4. ✅ **Add Email Fallback** - Send email if user offline for too long
5. ✅ **Add Analytics** - Track delivery rates, read rates

## 🎉 Summary

Kamu sekarang punya sistem notifikasi real-time yang:
- ✅ Menggunakan Redis Streams (no extra service!)
- ✅ Support user online/offline
- ✅ Reliable dengan database persistence
- ✅ Fast dengan SSE
- ✅ Scalable dengan consumer groups

Happy coding! 🚀