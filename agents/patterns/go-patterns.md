# Go Patterns Reference

> Load this file when working on the database layer, concurrency, worker pools, or idempotency.

---

## Database Guidelines

House standard for the data layer, whatever the driver or query builder.

### DO
- Write properly indexed queries with parameterized values
- Use single, composite, and partial indexes strategically
- Use `EXPLAIN ANALYZE` on slow queries
- Keep logic in application code (testable, version controlled)

### DON'T
- No stored procedures, views, or triggers
- No ORM-generated queries for complex operations — write explicit queries

```sql
-- Index examples
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_transactions_user_date ON transactions(user_id, created_at);
CREATE INDEX idx_active_users ON users(id) WHERE deleted_at IS NULL;
```

---

## Concurrency & Race Conditions

### Understanding Race Conditions

A race condition occurs when multiple goroutines access shared data concurrently, and at least one modifies it.

**Common scenarios:**
- Counter increments without synchronization
- Read-modify-write operations
- Check-then-act patterns
- Shared map access

```go
// ❌ RACE CONDITION: Multiple goroutines modify counter
var counter int

func increment() {
    counter++ // Read, increment, write - NOT atomic!
}

// ✅ FIX 1: Use mutex
var (
    counter int
    mu      sync.Mutex
)

func increment() {
    mu.Lock()
    defer mu.Unlock()
    counter++
}

// ✅ FIX 2: Use atomic operations
var counter int64

func increment() {
    atomic.AddInt64(&counter, 1)
}
```

### Mutex Patterns

**Basic Mutex:**
```go
type SafeCounter struct {
    mu    sync.Mutex
    value int
}

func (c *SafeCounter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.value++
}

func (c *SafeCounter) Value() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.value
}
```

**RWMutex (for read-heavy workloads):**
```go
type SafeCache struct {
    mu    sync.RWMutex
    items map[string]interface{}
}

func (c *SafeCache) Get(key string) (interface{}, bool) {
    c.mu.RLock()         // Multiple readers allowed
    defer c.mu.RUnlock()
    val, ok := c.items[key]
    return val, ok
}

func (c *SafeCache) Set(key string, val interface{}) {
    c.mu.Lock()          // Exclusive write access
    defer c.mu.Unlock()
    c.items[key] = val
}
```

**Mutex best practices:**
- Always use `defer mu.Unlock()` to prevent deadlocks
- Keep critical sections small
- Use RWMutex when reads >> writes
- Never copy a mutex (pass by pointer)

### Semaphore Pattern (Limit Concurrency)

```go
// Semaphore limits concurrent operations
type Semaphore struct {
    sem chan struct{}
}

func NewSemaphore(max int) *Semaphore {
    return &Semaphore{
        sem: make(chan struct{}, max),
    }
}

func (s *Semaphore) Acquire() {
    s.sem <- struct{}{}
}

func (s *Semaphore) Release() {
    <-s.sem
}

// Usage: Limit to 10 concurrent DB connections
func processItems(items []Item) {
    sem := NewSemaphore(10)
    var wg sync.WaitGroup

    for _, item := range items {
        wg.Add(1)
        sem.Acquire()

        go func(item Item) {
            defer wg.Done()
            defer sem.Release()
            processItem(item)
        }(item)
    }

    wg.Wait()
}
```

**Using golang.org/x/sync/semaphore:**
```go
import "golang.org/x/sync/semaphore"

var sem = semaphore.NewWeighted(10)

func processWithLimit(ctx context.Context, item Item) error {
    if err := sem.Acquire(ctx, 1); err != nil {
        return err
    }
    defer sem.Release(1)

    return process(item)
}
```

---

## Worker Pool Pattern

### Basic Worker Pool

```go
type Job struct {
    ID   int
    Data interface{}
}

type Result struct {
    JobID int
    Value interface{}
    Err   error
}

func WorkerPool(numWorkers int, jobs <-chan Job, results chan<- Result) {
    var wg sync.WaitGroup

    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            for job := range jobs {
                result := processJob(job)
                results <- result
            }
        }(i)
    }

    wg.Wait()
    close(results)
}

// Usage
func main() {
    jobs := make(chan Job, 100)
    results := make(chan Result, 100)

    // Start 5 workers
    go WorkerPool(5, jobs, results)

    // Send jobs
    go func() {
        for i := 0; i < 50; i++ {
            jobs <- Job{ID: i, Data: i * 2}
        }
        close(jobs)
    }()

    // Collect results
    for result := range results {
        fmt.Printf("Job %d: %v\n", result.JobID, result.Value)
    }
}
```

### Production Worker Pool with Context

```go
type WorkerPool struct {
    numWorkers int
    jobs       chan Job
    results    chan Result
    ctx        context.Context
    cancel     context.CancelFunc
    wg         sync.WaitGroup
}

func NewWorkerPool(ctx context.Context, numWorkers, bufferSize int) *WorkerPool {
    ctx, cancel := context.WithCancel(ctx)
    return &WorkerPool{
        numWorkers: numWorkers,
        jobs:       make(chan Job, bufferSize),
        results:    make(chan Result, bufferSize),
        ctx:        ctx,
        cancel:     cancel,
    }
}

func (wp *WorkerPool) Start() {
    for i := 0; i < wp.numWorkers; i++ {
        wp.wg.Add(1)
        go wp.worker(i)
    }
}

func (wp *WorkerPool) worker(id int) {
    defer wp.wg.Done()

    for {
        select {
        case <-wp.ctx.Done():
            return
        case job, ok := <-wp.jobs:
            if !ok {
                return
            }
            result := wp.processJob(job)

            select {
            case wp.results <- result:
            case <-wp.ctx.Done():
                return
            }
        }
    }
}

func (wp *WorkerPool) Submit(job Job) error {
    select {
    case wp.jobs <- job:
        return nil
    case <-wp.ctx.Done():
        return wp.ctx.Err()
    }
}

func (wp *WorkerPool) Shutdown() {
    close(wp.jobs)
    wp.wg.Wait()
    close(wp.results)
}

func (wp *WorkerPool) Stop() {
    wp.cancel()
    wp.wg.Wait()
}
```

---

## Idempotency (Prevent Double Payment/Booking)

Idempotency ensures that multiple identical requests produce the same result.

### Choosing a strategy

Every mutating endpoint takes an idempotency key — that decision is not conditional. What varies is
the locking underneath it: optimistic (version column) when conflicts are rare and a retry is cheap,
pessimistic (`FOR UPDATE`) when losing the race corrupts money or state. A distributed lock only once
more than one instance runs; it is the expensive option, not the default.

### Idempotency Key Pattern

```go
// dto/payment.go
type PaymentRequest struct {
    IdempotencyKey string  `json:"idempotency_key" validate:"required,uuid"`
    Amount         int64   `json:"amount" validate:"required,min=1"`
    Currency       string  `json:"currency" validate:"required"`
    CustomerID     string  `json:"customer_id" validate:"required"`
}

// datamodel/idempotency.go
type IdempotencyRecord struct {
    Key        string          `db:"idempotency_key"`
    RequestHash string         `db:"request_hash"`
    Response   json.RawMessage `db:"response"`
    StatusCode int             `db:"status_code"`
    CreatedAt  time.Time       `db:"created_at"`
    ExpiresAt  time.Time       `db:"expires_at"`
}

// postgresql/idempotency.go
func (r *IdempotencyRepo) GetOrCreate(ctx context.Context, key, requestHash string) (*IdempotencyRecord, bool, error) {
    // Try to insert first (optimistic)
    query := `
        INSERT INTO idempotency_records (idempotency_key, request_hash, created_at, expires_at)
        VALUES ($1, $2, NOW(), NOW() + INTERVAL '24 hours')
        ON CONFLICT (idempotency_key) DO NOTHING
        RETURNING *
    `

    var record IdempotencyRecord
    err := r.db.QueryRowContext(ctx, query, key, requestHash).Scan(...)

    if err == sql.ErrNoRows {
        // Record exists, fetch it
        existing, err := r.Get(ctx, key)
        if err != nil {
            return nil, false, err
        }

        // Verify request hash matches (same request, not different request with same key)
        if existing.RequestHash != requestHash {
            return nil, false, ErrIdempotencyKeyReused
        }

        return existing, true, nil // isExisting = true
    }

    return &record, false, err // isExisting = false
}

func (r *IdempotencyRepo) SaveResponse(ctx context.Context, key string, statusCode int, response []byte) error {
    query := `
        UPDATE idempotency_records
        SET response = $2, status_code = $3
        WHERE idempotency_key = $1
    `
    _, err := r.db.ExecContext(ctx, query, key, response, statusCode)
    return err
}
```

### Service Layer with Idempotency

```go
// service.go
func (s *PaymentService) ProcessPayment(ctx context.Context, req dto.PaymentRequest) (*dto.PaymentResponse, error) {
    // 1. Hash the request to detect same key with different payload
    requestHash := hashRequest(req)

    // 2. Check idempotency
    record, isExisting, err := s.idempotencyRepo.GetOrCreate(ctx, req.IdempotencyKey, requestHash)
    if err != nil {
        if errors.Is(err, ErrIdempotencyKeyReused) {
            return nil, fmt.Errorf("idempotency key already used with different request")
        }
        return nil, err
    }

    // 3. If existing, return cached response
    if isExisting && record.Response != nil {
        var response dto.PaymentResponse
        if err := json.Unmarshal(record.Response, &response); err != nil {
            return nil, err
        }
        return &response, nil
    }

    // 4. Process payment (new request)
    response, err := s.executePayment(ctx, req)
    if err != nil {
        return nil, err
    }

    // 5. Save response for future idempotent requests
    responseBytes, _ := json.Marshal(response)
    _ = s.idempotencyRepo.SaveResponse(ctx, req.IdempotencyKey, 200, responseBytes)

    return response, nil
}

func hashRequest(req dto.PaymentRequest) string {
    h := sha256.New()
    h.Write([]byte(fmt.Sprintf("%d:%s:%s", req.Amount, req.Currency, req.CustomerID)))
    return hex.EncodeToString(h.Sum(nil))
}
```

### Database-Level Idempotency (Double Booking Prevention)

```sql
-- Unique constraint prevents double booking
CREATE TABLE bookings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    resource_id UUID NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    idempotency_key UUID UNIQUE NOT NULL,  -- Prevents duplicate submissions
    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- Prevents overlapping bookings for same resource
    EXCLUDE USING gist (
        resource_id WITH =,
        tstzrange(start_time, end_time) WITH &&
    )
);

-- Idempotency table with TTL
CREATE TABLE idempotency_records (
    idempotency_key UUID PRIMARY KEY,
    request_hash VARCHAR(64) NOT NULL,
    response JSONB,
    status_code INT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_idempotency_expires ON idempotency_records(expires_at);
```

### Transaction-Level Locking (Pessimistic)

```go
// Prevent double payment with row-level locking
func (r *PaymentRepo) ProcessPaymentWithLock(ctx context.Context, paymentID string) error {
    tx, err := r.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Lock the payment row - prevents concurrent processing
    var payment Payment
    err = tx.QueryRowContext(ctx, `
        SELECT id, status, amount
        FROM payments
        WHERE id = $1
        FOR UPDATE NOWAIT  -- Fail immediately if locked
    `, paymentID).Scan(&payment.ID, &payment.Status, &payment.Amount)

    if err != nil {
        if strings.Contains(err.Error(), "could not obtain lock") {
            return ErrPaymentBeingProcessed
        }
        return err
    }

    // Check status after acquiring lock
    if payment.Status != StatusPending {
        return ErrPaymentAlreadyProcessed
    }

    // Process payment...
    _, err = tx.ExecContext(ctx, `
        UPDATE payments SET status = $1 WHERE id = $2
    `, StatusCompleted, paymentID)
    if err != nil {
        return err
    }

    return tx.Commit()
}
```

### Optimistic Locking with Version

```go
// datamodel/payment.go
type Payment struct {
    ID        string
    Amount    int64
    Status    string
    Version   int  // Optimistic lock version
    UpdatedAt time.Time
}

// postgresql/payment.go
func (r *PaymentRepo) UpdateWithOptimisticLock(ctx context.Context, payment *Payment) error {
    result, err := r.db.ExecContext(ctx, `
        UPDATE payments
        SET status = $1, version = version + 1, updated_at = NOW()
        WHERE id = $2 AND version = $3
    `, payment.Status, payment.ID, payment.Version)

    if err != nil {
        return err
    }

    rows, _ := result.RowsAffected()
    if rows == 0 {
        return ErrConcurrentModification // Someone else modified it
    }

    payment.Version++
    return nil
}

// service.go - Retry with optimistic locking
func (s *PaymentService) UpdatePaymentWithRetry(ctx context.Context, paymentID string, update func(*Payment) error) error {
    maxRetries := 3

    for i := 0; i < maxRetries; i++ {
        payment, err := s.repo.GetByID(ctx, paymentID)
        if err != nil {
            return err
        }

        if err := update(payment); err != nil {
            return err
        }

        err = s.repo.UpdateWithOptimisticLock(ctx, payment)
        if err == nil {
            return nil
        }

        if !errors.Is(err, ErrConcurrentModification) {
            return err
        }

        // Retry on concurrent modification
        time.Sleep(time.Duration(i*10) * time.Millisecond)
    }

    return ErrMaxRetriesExceeded
}
```

### Distributed Locking (Redis)

```go
import "github.com/go-redsync/redsync/v4"

type DistributedLock struct {
    rs *redsync.Redsync
}

func (l *DistributedLock) WithLock(ctx context.Context, key string, ttl time.Duration, fn func() error) error {
    mutex := l.rs.NewMutex(
        "lock:"+key,
        redsync.WithExpiry(ttl),
        redsync.WithTries(3),
    )

    if err := mutex.LockContext(ctx); err != nil {
        return fmt.Errorf("failed to acquire lock: %w", err)
    }
    defer mutex.Unlock()

    return fn()
}

// Usage: Prevent double payment processing
func (s *PaymentService) ProcessPayment(ctx context.Context, paymentID string) error {
    return s.lock.WithLock(ctx, "payment:"+paymentID, 30*time.Second, func() error {
        // Only one instance can process this payment at a time
        return s.doProcessPayment(ctx, paymentID)
    })
}
```

---

## Concurrency Checklist

- [ ] Shared data protected by mutex or channels?
- [ ] Race condition tested with `-race` flag?
- [ ] Deadlock scenarios considered?
- [ ] Context cancellation propagated?
- [ ] Goroutine leaks prevented (proper cleanup)?
- [ ] Worker pool sized appropriately?

## Idempotency Checklist

- [ ] Idempotency key required for mutating operations?
- [ ] Request hash validates same payload?
- [ ] Response cached for replay?
- [ ] TTL set for idempotency records?
- [ ] Database constraints prevent duplicates?
- [ ] Optimistic or pessimistic locking chosen?
- [ ] Distributed lock needed for multi-instance?
