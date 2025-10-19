# Real-time Notification System

Sistem notifikasi real-time yang terintegrasi dengan Go Scheduler menggunakan Redis Pub/Sub dan WebSocket.

## 🎯 Features

- ✅ **Real-time notifications** via WebSocket
- ✅ **Redis Pub/Sub** untuk distributed messaging
- ✅ **Hybrid session management** (Redis + PostgreSQL)
- ✅ **JWT authentication**
- ✅ **Multi-device support** - satu user bisa login di banyak device
- ✅ **Pending notifications** - notifikasi tersimpan untuk user yang offline
- ✅ **Per-session read status** - setiap device punya status read sendiri
- ✅ **Auto cleanup** expired notifications
- ✅ **Fully configurable** via environment variables
- ✅ **Production-ready** dengan error handling dan reconnection logic

## 📁 Architecture

```
┌─────────────┐        ┌─────────────┐         ┌─────────────┐
│   Worker    │───────▶│   Redis     │───────▶│  WebSocket  │
│  (Producer) │ Publish│  Pub/Sub    │Subscribe│    Hub      │
└─────────────┘        └─────────────┘         └─────────────┘
                              │                       │
                              │                       │
                              ▼                       ▼
                       ┌─────────────┐        ┌─────────────┐
                       │ PostgreSQL  │        │   Clients   │
                       │  (Storage)  │        │ (Browsers)  │
                       └─────────────┘        └─────────────┘
```

## 🚀 Quick Start

### 1. Environment Setup

Update your `.env` file:

```env
# Notification Configuration
NOTIFICATION_ENABLED=true
NOTIFICATION_REDIS_CHANNEL=notification-dev
NOTIFICATION_RETENTION_DAYS=30
NOTIFICATION_USE_POSTGRES_SESSION=false

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-change-this-in-production
JWT_EXPIRY_HOURS=24

# WebSocket Configuration
WS_READ_BUFFER_SIZE=1024
WS_WRITE_BUFFER_SIZE=1024
WS_PING_INTERVAL_SECONDS=30
WS_PONG_TIMEOUT_SECONDS=60
WS_MAX_MESSAGE_SIZE=512
```

### 2. Run Migrations

```bash
make migrate
```

Or manually:

```sql
-- Run the migration SQL from migrations/001_create_notifications.sql
```

### 3. Start Services

```bash
make up
```

### 4. Generate JWT Token (for testing)

```bash
# You'll need to create an endpoint or use existing auth
# Example token structure:
{
  "user_id": "user-123",
  "session_id": "session-abc",
  "exp": 1234567890
}
```

### 5. Connect via WebSocket

Open `frontend-client.html` in your browser and:
1. Enter WebSocket URL: `ws://localhost:8080/ws/notifications`
2. Enter your JWT token
3. Click "Connect"

## 📡 API Endpoints

### Authentication

All notification endpoints require JWT authentication via `Authorization: Bearer <token>` header.

### Get Notifications

```http
GET /api/v1/notifications?limit=20&offset=0&unread=false
Authorization: Bearer <jwt_token>
```

Response:
```json
{
  "notifications": [
    {
      "notification": {
        "id": "uuid",
        "user_id": "user-123",
        "task_id": "task-uuid",
        "title": "Task completed",
        "message": "Task PRODUCT - CREATE is now completed",
        "type": "task_update",
        "metadata": {
          "status": "completed",
          "entity": "PRODUCT",
          "action": "CREATE"
        },
        "created_at": "2025-10-18T10:00:00Z"
      },
      "is_read": false
    }
  ],
  "total": 100,
  "unread": 5
}
```

### Mark as Read

```http
POST /api/v1/notifications/read
Authorization: Bearer <jwt_token>
Content-Type: application/json

{
  "notification_ids": ["uuid1", "uuid2"]
}
```

### Get Unread Count

```http
GET /api/v1/notifications/unread-count
Authorization: Bearer <jwt_token>
```

Response:
```json
{
  "unread_count": 5
}
```

## 🔌 WebSocket Protocol

### Connection

```javascript
const ws = new WebSocket('ws://localhost:8080/ws/notifications?token=JWT_TOKEN');
```

### Message Types

#### 1. Auth Success (Server → Client)

```json
{
  "type": "auth_success",
  "timestamp": 1697654400,
  "session_id": "session-abc",
  "data": {
    "user_id": "user-123",
    "session_id": "session-abc"
  }
}
```

#### 2. Notification (Server → Client)

```json
{
  "type": "notification",
  "timestamp": 1697654400,
  "data": {
    "notification": {
      "notification": {
        "id": "uuid",
        "user_id": "user-123",
        "task_id": "task-uuid",
        "title": "Task completed",
        "message": "Task PRODUCT - CREATE is now completed",
        "type": "task_update",
        "metadata": {},
        "created_at": "2025-10-18T10:00:00Z"
      },
      "is_read": false
    }
  }
}
```

#### 3. Ping/Pong (Bi-directional)

```json
// Client → Server
{
  "type": "ping",
  "timestamp": 1697654400
}

// Server → Client
{
  "type": "pong",
  "timestamp": 1697654400
}
```

#### 4. Error (Server → Client)

```json
{
  "type": "error",
  "timestamp": 1697654400,
  "data": {
    "code": "INVALID_TOKEN",
    "message": "Authentication failed"
  }
}
```

## 🔧 Configuration Options

### Redis Channel Naming

Untuk environment berbeda:

```env
# Development
NOTIFICATION_REDIS_CHANNEL=notification-dev

# Staging
NOTIFICATION_REDIS_CHANNEL=notification-staging

# Production
NOTIFICATION_REDIS_CHANNEL=notification-prod
```

### Session Storage Strategy

**Option 1: Redis Only (Default - Recommended)**
```env
NOTIFICATION_USE_POSTGRES_SESSION=false
```
- ✅ Faster
- ✅ Automatic TTL
- ❌ Session hilang saat Redis restart

**Option 2: PostgreSQL Only**
```env
NOTIFICATION_USE_POSTGRES_SESSION=true
```
- ✅ Persistent
- ✅ Queryable
- ❌ Slightly slower

**Option 3: Hybrid (Best)**
- Gunakan Redis untuk active sessions
- PostgreSQL untuk audit log
- Implement custom logic di `session_service.go`

### Retention Policy

```env
# Keep notifications for 30 days
NOTIFICATION_RETENTION_DAYS=30

# Keep forever
NOTIFICATION_RETENTION_DAYS=0
```

## 🧪 Testing

### Unit Tests

```bash
make test
```

### Integration Test

```bash
# Terminal 1: Start services
make up

# Terminal 2: Run integration test
go test -v ./tests/integration/notification_test.go
```

### Load Test

```bash
make load-test
```

## 🐛 Troubleshooting

### WebSocket Connection Failed

1. Check JWT token validity
2. Verify CORS settings in `router.go`
3. Check firewall/proxy settings

### Notifications Not Received

1. Check if notification system is enabled:
   ```bash
   curl http://localhost:8080/ws/health
   ```

2. Verify Redis Pub/Sub:
   ```bash
   make redis-cli
   > SUBSCRIBE notification-dev
   # In another terminal, trigger a task to create notification
   ```

3. Check logs:
   ```bash
   make logs-app
   ```

### User Not Receiving Pending Notifications

1. Ensure user is authenticated correctly
2. Check session storage:
   ```bash
   # Redis
   make redis-cli
   > KEYS session:*
   
   # PostgreSQL
   make db-shell
   SELECT * FROM sessions WHERE user_id = 'your-user-id';
   ```

### High Memory Usage

1. Reduce retention days
2. Implement pagination for notification history
3. Enable PostgreSQL session storage only

### Notifications Sent Multiple Times

Check if you have multiple worker instances without proper Redis locking. This is already handled by the scheduler's distributed locking mechanism.

## 📊 Monitoring

### Health Checks

```bash
# App health
curl http://localhost:8080/health

# WebSocket health
curl http://localhost:8080/ws/health
```

### Metrics to Monitor

1. **Active WebSocket Connections**
   - Implement counter in `WebSocketHub`
   
2. **Notification Delivery Rate**
   - Log successful deliveries
   
3. **Redis Pub/Sub Lag**
   - Monitor message queue depth
   
4. **Database Size**
   - Monitor `notifications` table growth

### Prometheus Metrics (Optional)

Add to your code:

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    activeConnections = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "websocket_active_connections",
        Help: "Number of active WebSocket connections",
    })
    
    notificationsSent = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "notifications_sent_total",
        Help: "Total number of notifications sent",
    })
)
```

## 🔐 Security Best Practices

### 1. JWT Secret

```bash
# Generate strong secret
openssl rand -base64 32
```

### 2. CORS Configuration

Update `router.go`:

```go
// Only allow specific origins in production
allowedOrigins := []string{
    "https://yourdomain.com",
    "https://app.yourdomain.com",
}
```

### 3. Rate Limiting

Implement rate limiting for notification endpoints:

```go
import "github.com/ulule/limiter/v3"

// Limit to 100 requests per minute
rate := limiter.Rate{
    Period: 1 * time.Minute,
    Limit:  100,
}
```

### 4. Input Validation

Always validate notification content:

```go
func (s *NotificationService) CreateAndSend(ctx context.Context, notification *dto.Notification) error {
    // Validate
    if len(notification.Title) > 500 {
        return errors.New("title too long")
    }
    if len(notification.Message) > 5000 {
        return errors.New("message too long")
    }
    
    // ... rest of the code
}
```

## 🚀 Production Deployment

### 1. Update Environment Variables

```env
# Production values
NOTIFICATION_REDIS_CHANNEL=notification-prod
JWT_SECRET=<strong-random-secret>
NOTIFICATION_RETENTION_DAYS=90
NOTIFICATION_USE_POSTGRES_SESSION=true
```

### 2. Database Indexes

Ensure these indexes exist:

```sql
-- Check existing indexes
\di notifications*

-- Create if missing
CREATE INDEX CONCURRENTLY idx_notifications_user_created 
ON notifications(user_id, created_at DESC);

CREATE INDEX CONCURRENTLY idx_notifications_expires 
ON notifications(expires_at) WHERE expires_at IS NOT NULL;
```

### 3. Redis Configuration

For production, consider using Redis Sentinel or Cluster:

```env
# Redis Sentinel
REDIS_ADDR=sentinel1:26379,sentinel2:26379,sentinel3:26379
REDIS_MASTER_NAME=mymaster

# Redis Cluster
REDIS_CLUSTER_ADDRS=node1:6379,node2:6379,node3:6379
```

### 4. Load Balancer Configuration

If using multiple app instances behind a load balancer:

```nginx
# Nginx configuration for WebSocket
upstream websocket_backend {
    ip_hash;  # Important: sticky sessions for WebSocket
    server app1:8080;
    server app2:8080;
    server app3:8080;
}

server {
    location /ws/ {
        proxy_pass http://websocket_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_read_timeout 86400;
    }
}
```

### 5. Horizontal Scaling

The notification system supports horizontal scaling:

- ✅ Multiple worker instances (already supported by scheduler)
- ✅ Multiple app instances (WebSocket hub per instance)
- ✅ Redis Pub/Sub broadcasts to all instances
- ✅ Each instance manages its own WebSocket connections

## 📈 Performance Optimization

### 1. Connection Pooling

Already configured in `database/database.go`:

```go
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

### 2. Redis Pipeline

For bulk operations:

```go
pipe := r.redis.Pipeline()
for _, notif := range notifications {
    pipe.Publish(ctx, r.channel, notif)
}
_, err := pipe.Exec(ctx)
```

### 3. Notification Batching

For high-volume scenarios:

```go
// Batch notifications every 100ms
ticker := time.NewTicker(100 * time.Millisecond)
buffer := make([]*dto.Notification, 0, 100)

for {
    select {
    case notif := <-notifChan:
        buffer = append(buffer, notif)
        if len(buffer) >= 100 {
            s.sendBatch(buffer)
            buffer = buffer[:0]
        }
    case <-ticker.C:
        if len(buffer) > 0 {
            s.sendBatch(buffer)
            buffer = buffer[:0]
        }
    }
}
```

### 4. Database Partitioning

For large datasets:

```sql
-- Partition by month
CREATE TABLE notifications_2025_10 PARTITION OF notifications
FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
```

## 🔄 Maintenance

### Daily Tasks

```bash
# Check system health
make logs | grep ERROR

# Monitor connections
make redis-cli
> CLIENT LIST | grep notification
```

### Weekly Tasks

```bash
# Database vacuum
make db-shell
VACUUM ANALYZE notifications;

# Check table sizes
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename))
FROM pg_tables 
WHERE tablename LIKE 'notification%';
```

### Monthly Tasks

```bash
# Backup database
make db-backup

# Review retention policy
# Analyze notification patterns
```

## 📚 Additional Resources

### API Documentation

Interactive Swagger documentation available at:
```
http://localhost:8080/swagger/index.html
```

### Frontend Integration Examples

#### React Example

```jsx
import { useState, useEffect } from 'react';

function useNotifications(token) {
    const [notifications, setNotifications] = useState([]);
    const [ws, setWs] = useState(null);
    
    useEffect(() => {
        const websocket = new WebSocket(
            `ws://localhost:8080/ws/notifications?token=${token}`
        );
        
        websocket.onmessage = (event) => {
            const message = JSON.parse(event.data);
            if (message.type === 'notification') {
                setNotifications(prev => [
                    message.data.notification,
                    ...prev
                ]);
            }
        };
        
        setWs(websocket);
        
        return () => websocket.close();
    }, [token]);
    
    return { notifications, ws };
}
```

#### Vue.js Example

```javascript
export default {
    data() {
        return {
            notifications: [],
            ws: null
        }
    },
    mounted() {
        this.connectWebSocket();
    },
    methods: {
        connectWebSocket() {
            this.ws = new WebSocket(
                `ws://localhost:8080/ws/notifications?token=${this.token}`
            );
            
            this.ws.onmessage = (event) => {
                const message = JSON.parse(event.data);
                if (message.type === 'notification') {
                    this.notifications.unshift(message.data.notification);
                }
            };
        }
    },
    beforeUnmount() {
        if (this.ws) this.ws.close();
    }
}
```

## 🎓 Development Tips

### 1. Generate Test Notifications

Create a test endpoint:

```go
// handler/test_handler.go
func (h *TestHandler) SendTestNotification(c *gin.Context) {
    userID, _ := c.Get("user_id")
    
    notification := &dto.Notification{
        UserID:  userID.(string),
        TaskID:  uuid.New().String(),
        Title:   "Test Notification",
        Message: "This is a test notification at " + time.Now().Format(time.RFC3339),
        Type:    "test",
    }
    
    err := h.notificationService.CreateAndSend(c.Request.Context(), notification)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{"message": "Test notification sent"})
}
```

### 2. Debug WebSocket Messages

```javascript
ws.onmessage = (event) => {
    console.log('Raw message:', event.data);
    const message = JSON.parse(event.data);
    console.log('Parsed message:', message);
    // Handle message...
};
```

### 3. Monitor Redis Pub/Sub

```bash
# Terminal 1: Subscribe to channel
redis-cli
> SUBSCRIBE notification-dev

# Terminal 2: Trigger notifications
curl -X POST http://localhost:8080/api/v1/test/notification \
  -H "Authorization: Bearer YOUR_TOKEN"
```
