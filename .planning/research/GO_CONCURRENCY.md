# Go Concurrency Patterns for Shared State

**Researched:** 2026-03-25  
**Sources:** pkg.go.dev (go1.26.1), official Go docs  
**Confidence:** HIGH — all patterns from official stdlib documentation

---

## 1. `sync.RWMutex` Pattern

Write lock on update, read lock on serve. Good when you need to protect a struct field holding a pointer to the current value.

```go
type Cache struct {
    mu   sync.RWMutex
    data *[]Record // pointer to immutable snapshot
}

// Background sync goroutine — called infrequently
func (c *Cache) Update(newData []Record) {
    c.mu.Lock()
    c.data = &newData // replace pointer atomically under write lock
    c.mu.Unlock()
}

// HTTP handler — called concurrently, many readers
func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    c.mu.RLock()
    data := c.data // grab pointer; safe to read after RUnlock
    c.mu.RUnlock()

    if data == nil {
        http.Error(w, "data not ready", http.StatusServiceUnavailable)
        return
    }
    // use *data — it's an immutable snapshot, no lock needed
    render(w, *data)
}
```

**Key rule:** Only hold the lock long enough to swap the pointer. Never hold it while doing I/O or expensive work.

---

## 2. `sync/atomic.Value` Pattern

Lock-free pointer swap. Store/Load are sequentially consistent — no mutex required.

```go
type Cache struct {
    value atomic.Value // holds *[]Record
}

// Background sync goroutine
func (c *Cache) Update(newData []Record) {
    c.value.Store(&newData) // atomic store; safe from any goroutine
}

// HTTP handler
func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    v := c.value.Load() // atomic load; never blocks
    if v == nil {
        http.Error(w, "data not ready", http.StatusServiceUnavailable)
        return
    }
    data := v.(*[]Record) // type-assert; safe, Store enforces consistent type
    render(w, *data)
}
```

**Constraints (official docs):**
- Once you `Store` a value of type `T`, all subsequent stores **must** also be type `T`. Mixing types panics.
- `atomic.Value` holds `any`; the type-assert overhead is negligible.
- Go 1.19+ `atomic.Pointer[T]` is the type-safe generic alternative — **prefer it** when targeting Go 1.19+:

```go
type Cache struct {
    ptr atomic.Pointer[[]Record]
}

func (c *Cache) Update(newData []Record) { c.ptr.Store(&newData) }

func (c *Cache) Get() *[]Record { return c.ptr.Load() } // returns nil if not yet set
```

---

## 3. Which Pattern to Use: Recommendation

**Use `atomic.Pointer[T]` (or `atomic.Value`) for "replace-entire-value-on-update".**

| Criterion | `sync.RWMutex` | `atomic.Value` / `atomic.Pointer[T]` |
|-----------|---------------|--------------------------------------|
| Complexity | Slightly more (lock/unlock ceremony) | Minimal — Store/Load only |
| Performance | Excellent for read-heavy; RLock is nearly free with no writers | Lock-free; marginally faster under high read concurrency |
| Composability | Easy to protect multiple fields together | One value per atomic; use a wrapper struct if needed |
| Clarity | Universally understood; intent obvious | Idiomatic for single-value swap; slightly less familiar |
| Go recommendation | General shared state | "Read-mostly" pointer swap (official example in pkg.go.dev) |

**Decision rule:**
- Single pointer swap, read-heavy, no other shared fields → **`atomic.Pointer[T]`**
- Multiple related fields that must be updated together → **`sync.RWMutex`** (update a struct pointer under write lock)
- When in doubt, `sync.RWMutex` is more readable and harder to misuse

The official `sync/atomic` docs explicitly provide a `Value (ReadMostly)` example for exactly this pattern. For a background-sync + HTTP-serve scenario with a single snapshot, `atomic.Pointer[T]` is the more idiomatic choice in Go 1.19+.

---

## 4. HTTP Client Timeouts

### Client-level `Timeout` field

```go
client := &http.Client{
    Timeout: 10 * time.Second, // covers full request: dial + TLS + send + read body
}
resp, err := client.Get(url)
```

`Timeout` is a **hard wall-clock deadline** on the entire round-trip. It cancels the request regardless of progress. Use this as the **baseline default** — always set it. The default `http.DefaultClient` has no timeout (hangs forever).

### Context-based timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
resp, err := client.Do(req)
```

Context propagates cancellation through the call stack and across service boundaries. Use this when:
- The caller has a context (e.g., from an incoming HTTP request)
- You need per-request timeout that may differ from the client default
- You need to cancel the request early (user disconnects, etc.)

### Recommendation

**Use both, layered:**

```go
// Package-level: shared client with a sane default ceiling
var httpClient = &http.Client{
    Timeout: 30 * time.Second, // absolute max; prevents goroutine leaks
    Transport: &http.Transport{
        DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
        TLSHandshakeTimeout:   5 * time.Second,
        ResponseHeaderTimeout: 10 * time.Second,
        IdleConnTimeout:       90 * time.Second,
        MaxIdleConns:          100,
    },
}

// Per-call: inherit parent context for propagation + tighter deadline
func fetchData(ctx context.Context, url string) (*http.Response, error) {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, err
    }
    return httpClient.Do(req)
}
```

**Never use `http.DefaultClient`** in production — it has no timeout.

**Transport-level timeouts** (`DialContext`, `TLSHandshakeTimeout`, `ResponseHeaderTimeout`) are finer-grained than `Client.Timeout` and prevent stalls at specific phases. Use them for background sync goroutines where a slow upstream should not block indefinitely.

---

## 5. Guard Patterns for Empty Slice Access

### Do: Explicit length check + idiomatic error return

```go
func first(items []string) (string, error) {
    if len(items) == 0 {
        return "", fmt.Errorf("items is empty")
    }
    return items[0], nil
}
```

**This is the Go idiom.** Errors are values; callers decide how to handle them.

### Do NOT: Panic recovery as a guard

```go
// Anti-pattern — do not do this
func firstUnsafe(items []string) (result string, err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("recovered: %v", r)
        }
    }()
    return items[0], nil // panics on empty
}
```

**Why panic recovery is wrong here:**
- Panic is for unrecoverable programming errors, not expected edge cases
- Recovery hides the bug instead of preventing it
- It's ~100x slower than a length check
- Suppresses the stack trace that would identify the actual problem
- Idiomatic Go: "Don't use panic for normal error handling" (Effective Go)

### Nil vs empty slice

In Go, `nil` and `[]T{}` are both length-zero slices and behave identically in `len()` and `range`. Guard against both with the same check:

```go
if len(items) == 0 { // handles both nil and empty
    return "", ErrEmpty
}
```

### When to define sentinel errors

```go
var ErrNoData = errors.New("no data available") // caller can errors.Is() check

func (c *Cache) First() (Record, error) {
    data := c.ptr.Load()
    if data == nil || len(*data) == 0 {
        return Record{}, ErrNoData
    }
    return (*data)[0], nil
}
```

Use `errors.New` sentinel when callers need to distinguish this error from others. Use inline `fmt.Errorf` for one-off internal errors.

---

## Summary: Decision Guide

| Situation | Use |
|-----------|-----|
| Single pointer swap, read-heavy | `atomic.Pointer[T]` |
| Multiple fields updated atomically | `sync.RWMutex` + struct pointer |
| HTTP client in production | `http.Client{Timeout: N}` + `context.WithTimeout` |
| Empty slice guard | `len(items) == 0` + error return |
| Unexpected nil in critical path | `panic(...)` (not recovery — let it crash and fix the bug) |
