# Asynchronous API Development in Go

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

## Best Practices for Async APIs

1. **Always provide task status endpoints**
2. **Implement proper error handling and retries**
3. **Use context for cancellation**
4. **Set appropriate timeouts**
5. **Monitor goroutine leaks**
6. **Use worker pools to limit concurrency**
7. **Implement circuit breakers for external services**
8. **Log all async operations**
9. **Provide webhooks for completion notifications**
10. **Use distributed tracing for debugging**
