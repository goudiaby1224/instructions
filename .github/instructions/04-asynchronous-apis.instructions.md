# Asynchronous API Development in Go

## Incoming Asynchronous Traffic Patterns

### Kafka Consumer Implementation

```go
// internal/adapters/incoming/messaging/kafka/consumer.go
package kafka

import (
    "context"
    "github.com/segmentio/kafka-go"
    "myapi/internal/core/ports/incoming"
)

type MessageHandler interface {
    Handle(ctx context.Context, message []byte) error
}

type Consumer struct {
    reader   *kafka.Reader
    handlers map[string]MessageHandler
    logger   Logger
}

func NewConsumer(brokers []string, groupID string, logger Logger) *Consumer {
    return &Consumer{
        reader: kafka.NewReader(kafka.ReaderConfig{
            Brokers:        brokers,
            GroupID:        groupID,
            CommitInterval: time.Second,
            StartOffset:    kafka.LastOffset,
        }),
        handlers: make(map[string]MessageHandler),
        logger:   logger,
    }
}

func (c *Consumer) Subscribe(topic string, handler MessageHandler) {
    c.handlers[topic] = handler
    c.reader.SetOffset(kafka.LastOffset)
}

func (c *Consumer) Start(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return c.reader.Close()
        default:
            message, err := c.reader.ReadMessage(ctx)
            if err != nil {
                c.logger.Error("failed to read message", "error", err)
                continue
            }
            
            if err := c.processMessage(ctx, message); err != nil {
                c.logger.Error("failed to process message", 
                    "topic", message.Topic,
                    "partition", message.Partition,
                    "offset", message.Offset,
                    "error", err)
            }
        }
    }
}

func (c *Consumer) processMessage(ctx context.Context, message kafka.Message) error {
    handler, exists := c.handlers[message.Topic]
    if !exists {
        return fmt.Errorf("no handler for topic: %s", message.Topic)
    }
    
    // Add correlation ID to context
    ctx = context.WithValue(ctx, "correlation_id", 
        string(message.Headers["correlation_id"]))
    
    return handler.Handle(ctx, message.Value)
}
```

### Event Handler Implementation

```go
// internal/adapters/incoming/messaging/kafka/order_events_handler.go
package kafka

type OrderEventsHandler struct {
    orderService incoming.OrderService
    logger       Logger
}

func NewOrderEventsHandler(orderService incoming.OrderService, logger Logger) *OrderEventsHandler {
    return &OrderEventsHandler{
        orderService: orderService,
        logger:       logger,
    }
}

func (h *OrderEventsHandler) HandleOrderCreated(ctx context.Context, message []byte) error {
    var event OrderCreatedEvent
    if err := json.Unmarshal(message, &event); err != nil {
        return fmt.Errorf("failed to unmarshal order created event: %w", err)
    }
    
    // Validate event
    if err := event.Validate(); err != nil {
        return fmt.Errorf("invalid order created event: %w", err)
    }
    
    // Convert to domain model
    order := &domain.Order{
        ID:         event.OrderID,
        UserID:     event.UserID,
        Items:      convertItems(event.Items),
        Total:      event.Total,
        Status:     domain.OrderStatusPending,
        CreatedAt:  event.Timestamp,
    }
    
    // Process through service
    return h.orderService.ProcessNewOrder(ctx, order)
}

func (h *OrderEventsHandler) HandlePaymentCompleted(ctx context.Context, message []byte) error {
    var event PaymentCompletedEvent
    if err := json.Unmarshal(message, &event); err != nil {
        return fmt.Errorf("failed to unmarshal payment completed event: %w", err)
    }
    
    return h.orderService.CompletePayment(ctx, event.OrderID, event.PaymentID)
}
```

### RabbitMQ Consumer

```go
// internal/adapters/incoming/messaging/rabbitmq/consumer.go
package rabbitmq

import (
    "github.com/streadway/amqp"
)

type Consumer struct {
    conn     *amqp.Connection
    channel  *amqp.Channel
    handlers map[string]MessageHandler
    logger   Logger
}

func NewConsumer(url string, logger Logger) (*Consumer, error) {
    conn, err := amqp.Dial(url)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
    }
    
    ch, err := conn.Channel()
    if err != nil {
        return nil, fmt.Errorf("failed to open channel: %w", err)
    }
    
    return &Consumer{
        conn:     conn,
        channel:  ch,
        handlers: make(map[string]MessageHandler),
        logger:   logger,
    }, nil
}

func (c *Consumer) Subscribe(queueName string, handler MessageHandler) error {
    msgs, err := c.channel.Consume(
        queueName,
        "",    // consumer
        false, // auto-ack
        false, // exclusive
        false, // no-local
        false, // no-wait
        nil,   // args
    )
    if err != nil {
        return fmt.Errorf("failed to register consumer: %w", err)
    }
    
    go func() {
        for msg := range msgs {
            ctx := context.Background()
            
            if err := handler.Handle(ctx, msg.Body); err != nil {
                c.logger.Error("message processing failed", 
                    "queue", queueName, 
                    "error", err)
                msg.Nack(false, true) // requeue
            } else {
                msg.Ack(false)
            }
        }
    }()
    
    return nil
}
```

### NATS Subscriber

```go
// internal/adapters/incoming/messaging/nats/subscriber.go
package nats

import (
    "github.com/nats-io/nats.go"
)

type Subscriber struct {
    conn     *nats.Conn
    handlers map[string]MessageHandler
    logger   Logger
}

func NewSubscriber(url string, logger Logger) (*Subscriber, error) {
    nc, err := nats.Connect(url)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to NATS: %w", err)
    }
    
    return &Subscriber{
        conn:     nc,
        handlers: make(map[string]MessageHandler),
        logger:   logger,
    }, nil
}

func (s *Subscriber) Subscribe(subject string, handler MessageHandler) error {
    _, err := s.conn.Subscribe(subject, func(msg *nats.Msg) {
        ctx := context.Background()
        
        if err := handler.Handle(ctx, msg.Data); err != nil {
            s.logger.Error("message processing failed", 
                "subject", subject, 
                "error", err)
        }
    })
    
    return err
}

func (s *Subscriber) QueueSubscribe(subject, queue string, handler MessageHandler) error {
    _, err := s.conn.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
        ctx := context.Background()
        
        if err := handler.Handle(ctx, msg.Data); err != nil {
            s.logger.Error("message processing failed", 
                "subject", subject, 
                "queue", queue,
                "error", err)
        }
    })
    
    return err
}
```

## Goroutines and Channels

### Basic Async Pattern
```go
func (h *Handler) ProcessAsync(w http.ResponseWriter, r *http.Request) {
    taskID := uuid.New().String()
    
    // Start async processing
    go func() {
        if err := h.service.ProcessTask(taskID); err != nil {
            log.Error("task failed", "taskID", taskID, "error", err)
        }
    }()
    
    // Return immediately
    respondWithJSON(w, http.StatusAccepted, map[string]string{
        "taskId": taskID,
        "status": "processing",
    })
}
```

## Job Queue Pattern

```go
// internal/queue/worker.go
package queue

type Job struct {
    ID      string
    Type    string
    Payload interface{}
}

type Worker struct {
    ID         int
    JobQueue   chan Job
    WorkerPool chan chan Job
    QuitChan   chan bool
}

func NewWorker(id int, workerPool chan chan Job) Worker {
    return Worker{
        ID:         id,
        JobQueue:   make(chan Job),
        WorkerPool: workerPool,
        QuitChan:   make(chan bool),
    }
}

func (w Worker) Start() {
    go func() {
        for {
            w.WorkerPool <- w.JobQueue
            
            select {
            case job := <-w.JobQueue:
                if err := w.processJob(job); err != nil {
                    log.Error("job processing failed", "jobID", job.ID, "error", err)
                }
                
            case <-w.QuitChan:
                return
            }
        }
    }()
}

func (w Worker) processJob(job Job) error {
    log.Info("processing job", "jobID", job.ID, "type", job.Type)
    
    switch job.Type {
    case "email":
        return w.sendEmail(job.Payload)
    case "report":
        return w.generateReport(job.Payload)
    default:
        return fmt.Errorf("unknown job type: %s", job.Type)
    }
}
```

## Message Queue Integration

### RabbitMQ Example
```go
// internal/messaging/rabbitmq.go
package messaging

import "github.com/streadway/amqp"

type RabbitMQ struct {
    conn    *amqp.Connection
    channel *amqp.Channel
}

func (r *RabbitMQ) PublishTask(task *Task) error {
    body, err := json.Marshal(task)
    if err != nil {
        return err
    }
    
    return r.channel.Publish(
        "tasks",     // exchange
        task.Type,   // routing key
        false,       // mandatory
        false,       // immediate
        amqp.Publishing{
            ContentType: "application/json",
            Body:        body,
        },
    )
}

func (r *RabbitMQ) ConsumeTask(queueName string, handler func(*Task) error) error {
    msgs, err := r.channel.Consume(
        queueName,
        "",    // consumer
        false, // auto-ack
        false, // exclusive
        false, // no-local
        false, // no-wait
        nil,   // args
    )
    if err != nil {
        return err
    }
    
    forever := make(chan bool)
    
    go func() {
        for msg := range msgs {
            var task Task
            if err := json.Unmarshal(msg.Body, &task); err != nil {
                log.Error("failed to unmarshal task", "error", err)
                msg.Nack(false, false)
                continue
            }
            
            if err := handler(&task); err != nil {
                log.Error("task handler failed", "error", err)
                msg.Nack(false, true) // requeue
            } else {
                msg.Ack(false)
            }
        }
    }()
    
    <-forever
    return nil
}
```

### Redis Queue Example
```go
// internal/queue/redis.go
package queue

import "github.com/go-redis/redis/v8"

type RedisQueue struct {
    client *redis.Client
}

func (q *RedisQueue) Enqueue(ctx context.Context, queueName string, task *Task) error {
    data, err := json.Marshal(task)
    if err != nil {
        return err
    }
    
    return q.client.LPush(ctx, queueName, data).Err()
}

func (q *RedisQueue) Dequeue(ctx context.Context, queueName string) (*Task, error) {
    result, err := q.client.BRPop(ctx, 0, queueName).Result()
    if err != nil {
        return nil, err
    }
    
    var task Task
    if err := json.Unmarshal([]byte(result[1]), &task); err != nil {
        return nil, err
    }
    
    return &task, nil
}
```

## WebSocket for Real-time Updates

```go
// internal/websocket/hub.go
package websocket

type Hub struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
}

func NewHub() *Hub {
    return &Hub{
        broadcast:  make(chan []byte),
        register:   make(chan *Client),
        unregister: make(chan *Client),
        clients:    make(map[*Client]bool),
    }
}

func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.clients[client] = true
            
        case client := <-h.unregister:
            if _, ok := h.clients[client]; ok {
                delete(h.clients, client)
                close(client.send)
            }
            
        case message := <-h.broadcast:
            for client := range h.clients {
                select {
                case client.send <- message:
                default:
                    close(client.send)
                    delete(h.clients, client)
                }
            }
        }
    }
}
```

## Server-Sent Events (SSE)

```go
// internal/handlers/sse.go
func (h *Handler) SSEHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
        return
    }
    
    eventChan := make(chan Event)
    h.eventBus.Subscribe(r.Context(), eventChan)
    
    for {
        select {
        case event := <-eventChan:
            fmt.Fprintf(w, "event: %s\n", event.Type)
            fmt.Fprintf(w, "data: %s\n\n", event.Data)
            flusher.Flush()
            
        case <-r.Context().Done():
            h.eventBus.Unsubscribe(eventChan)
            return
        }
    }
}
```

## Error Handling for Async Processing

```go
// internal/adapters/incoming/messaging/error_handler.go
package messaging

type ErrorHandler interface {
    Handle(ctx context.Context, message Message, err error) error
}

type RetryErrorHandler struct {
    maxRetries int
    retryDelay time.Duration
    dlq        DeadLetterQueue
    logger     Logger
}

func (h *RetryErrorHandler) Handle(ctx context.Context, message Message, err error) error {
    retryCount := message.GetRetryCount()
    
    if retryCount >= h.maxRetries {
        h.logger.Error("max retries exceeded, sending to DLQ", 
            "message_id", message.ID,
            "retry_count", retryCount,
            "error", err)
        
        return h.dlq.Send(ctx, message, err)
    }
    
    // Increment retry count and delay
    time.Sleep(h.retryDelay * time.Duration(retryCount+1))
    message.IncrementRetryCount()
    
    h.logger.Warn("retrying message processing", 
        "message_id", message.ID,
        "retry_count", retryCount+1,
        "error", err)
    
    return nil // Message will be retried
}
```

## Message Deduplication

```go
// internal/adapters/incoming/messaging/deduplicator.go
package messaging

type Deduplicator struct {
    store   DeduplicationStore
    ttl     time.Duration
    logger  Logger
}

func (d *Deduplicator) IsDuplicate(ctx context.Context, messageID string) (bool, error) {
    exists, err := d.store.Exists(ctx, messageID)
    if err != nil {
        return false, fmt.Errorf("failed to check message existence: %w", err)
    }
    
    if exists {
        d.logger.Info("duplicate message detected", "message_id", messageID)
        return true, nil
    }
    
    // Mark as processed
    if err := d.store.Set(ctx, messageID, d.ttl); err != nil {
        return false, fmt.Errorf("failed to mark message as processed: %w", err)
    }
    
    return false, nil
}

// Redis implementation
type RedisDeduplicationStore struct {
    client *redis.Client
}

func (r *RedisDeduplicationStore) Exists(ctx context.Context, key string) (bool, error) {
    result, err := r.client.Exists(ctx, "msg:"+key).Result()
    return result > 0, err
}

func (r *RedisDeduplicationStore) Set(ctx context.Context, key string, ttl time.Duration) error {
    return r.client.Set(ctx, "msg:"+key, "processed", ttl).Err()
}
```

## Complete Application Setup

```go
// cmd/api/main.go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // Initialize dependencies
    db := initDatabase()
    redis := initRedis()
    logger := initLogger()
    
    // Initialize outgoing adapters
    userRepo := postgres.NewUserRepository(db)
    orderRepo := postgres.NewOrderRepository(db)
    
    // Initialize services
    userService := services.NewUserService(userRepo, logger)
    orderService := services.NewOrderService(orderRepo, logger)
    
    // Initialize synchronous incoming adapters
    httpServer := http.NewServer(userService, orderService, logger)
    
    // Initialize asynchronous incoming adapters
    kafkaConsumer := kafka.NewConsumer(kafkaConfig.Brokers, kafkaConfig.GroupID, logger)
    orderEventsHandler := kafka.NewOrderEventsHandler(orderService, logger)
    
    // Subscribe to topics
    kafkaConsumer.Subscribe("order.created", orderEventsHandler.HandleOrderCreated)
    kafkaConsumer.Subscribe("payment.completed", orderEventsHandler.HandlePaymentCompleted)
    
    // Start all adapters
    errGroup, ctx := errgroup.WithContext(ctx)
    
    errGroup.Go(func() error {
        return httpServer.Start()
    })
    
    errGroup.Go(func() error {
        return kafkaConsumer.Start(ctx)
    })
    
    // Graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    
    select {
    case <-sigChan:
        logger.Info("shutting down...")
        cancel()
    case <-ctx.Done():
    }
    
    // Wait for all goroutines to finish
    if err := errGroup.Wait(); err != nil {
        logger.Error("error during shutdown", "error", err)
    }
}
```

## Best Practices for Async Incoming Traffic

1. **Implement idempotency** - Handle duplicate messages gracefully
2. **Use correlation IDs** - Track requests across services
3. **Implement proper error handling** - Retries, dead letter queues
4. **Monitor message processing** - Metrics and logging
5. **Handle backpressure** - Rate limiting and circuit breakers
6. **Use structured logging** - Include context and correlation IDs
7. **Implement health checks** - Monitor consumer health
8. **Handle graceful shutdown** - Stop processing cleanly
9. **Use message versioning** - Handle schema evolution
10. **Test async flows thoroughly** - Integration and end-to-end tests
