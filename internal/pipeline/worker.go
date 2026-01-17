// Package pipeline provides worker pool and stream processing utilities.
package pipeline

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Message represents a generic message in the pipeline.
type Message struct {
	ID        string
	Content   string
	Source    string
	Timestamp time.Time
	Metadata  map[string]string
}

// Result represents the result of processing a message.
type Result struct {
	MessageID string
	Success   bool
	Data      interface{}
	Error     error
}

// Handler is a function that processes a message.
type Handler func(ctx context.Context, msg *Message) (*Result, error)

// WorkerPool manages a pool of workers for parallel processing.
type WorkerPool struct {
	tasks       chan *Message
	results     chan *Result
	workers     int
	handler     Handler
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *zap.Logger
	metrics     *PoolMetrics
	bufferSize  int
}

// PoolMetrics tracks worker pool statistics.
type PoolMetrics struct {
	mu             sync.Mutex
	Processed      int64
	Errors         int64
	Dropped        int64
	AvgProcessTime time.Duration
	totalTime      time.Duration
}

// PoolConfig configures the worker pool.
type PoolConfig struct {
	Workers    int
	BufferSize int
	Logger     *zap.Logger
}

// DefaultPoolConfig returns sensible defaults.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		Workers:    100,
		BufferSize: 10000,
	}
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(ctx context.Context, config PoolConfig) *WorkerPool {
	if config.Workers <= 0 {
		config.Workers = 100
	}
	if config.BufferSize <= 0 {
		config.BufferSize = 10000
	}

	ctx, cancel := context.WithCancel(ctx)

	return &WorkerPool{
		tasks:      make(chan *Message, config.BufferSize),
		results:    make(chan *Result, config.BufferSize),
		workers:    config.Workers,
		ctx:        ctx,
		cancel:     cancel,
		logger:     config.Logger,
		metrics:    &PoolMetrics{},
		bufferSize: config.BufferSize,
	}
}

// Start begins processing with the given handler.
func (wp *WorkerPool) Start(handler Handler) {
	wp.handler = handler

	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker(i)
	}

	if wp.logger != nil {
		wp.logger.Info("Worker pool started", zap.Int("workers", wp.workers))
	}
}

// worker is the main worker goroutine.
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	for {
		select {
		case msg := <-wp.tasks:
			if msg == nil {
				continue
			}

			start := time.Now()

			result, err := wp.handler(wp.ctx, msg)
			if err != nil {
				wp.metrics.mu.Lock()
				wp.metrics.Errors++
				wp.metrics.mu.Unlock()

				if wp.logger != nil {
					wp.logger.Error("Worker error",
						zap.Int("worker_id", id),
						zap.Error(err),
					)
				}

				result = &Result{
					MessageID: msg.ID,
					Success:   false,
					Error:     err,
				}
			} else {
				wp.metrics.mu.Lock()
				wp.metrics.Processed++
				elapsed := time.Since(start)
				wp.metrics.totalTime += elapsed
				wp.metrics.AvgProcessTime = wp.metrics.totalTime / time.Duration(wp.metrics.Processed)
				wp.metrics.mu.Unlock()
			}

			// Non-blocking send to results
			select {
			case wp.results <- result:
			default:
				// Results buffer full, drop result
			}

		case <-wp.ctx.Done():
			return
		}
	}
}

// Submit adds a message to the processing queue.
func (wp *WorkerPool) Submit(msg *Message) bool {
	select {
	case wp.tasks <- msg:
		return true
	case <-wp.ctx.Done():
		return false
	default:
		// Buffer full
		wp.metrics.mu.Lock()
		wp.metrics.Dropped++
		wp.metrics.mu.Unlock()

		if wp.logger != nil {
			wp.logger.Warn("Message dropped - buffer full")
		}
		return false
	}
}

// SubmitBlocking adds a message to the queue, blocking if full.
func (wp *WorkerPool) SubmitBlocking(msg *Message) bool {
	select {
	case wp.tasks <- msg:
		return true
	case <-wp.ctx.Done():
		return false
	}
}

// Results returns the results channel.
func (wp *WorkerPool) Results() <-chan *Result {
	return wp.results
}

// Stop gracefully shuts down the worker pool.
func (wp *WorkerPool) Stop() {
	wp.cancel()
	wp.wg.Wait()
	close(wp.tasks)
	close(wp.results)

	if wp.logger != nil {
		wp.logger.Info("Worker pool stopped",
			zap.Int64("processed", wp.metrics.Processed),
			zap.Int64("errors", wp.metrics.Errors),
			zap.Int64("dropped", wp.metrics.Dropped),
		)
	}
}

// GetMetrics returns current pool metrics.
func (wp *WorkerPool) GetMetrics() PoolMetrics {
	wp.metrics.mu.Lock()
	defer wp.metrics.mu.Unlock()

	return PoolMetrics{
		Processed:      wp.metrics.Processed,
		Errors:         wp.metrics.Errors,
		Dropped:        wp.metrics.Dropped,
		AvgProcessTime: wp.metrics.AvgProcessTime,
	}
}

// QueueSize returns the current number of pending tasks.
func (wp *WorkerPool) QueueSize() int {
	return len(wp.tasks)
}

// IsHealthy checks if the worker pool is functioning properly.
func (wp *WorkerPool) IsHealthy() bool {
	select {
	case <-wp.ctx.Done():
		return false
	default:
		// Check if queue is not critically full (>90%)
		return len(wp.tasks) < int(float64(wp.bufferSize)*0.9)
	}
}

// Batch processes a batch of messages and waits for all results.
func (wp *WorkerPool) Batch(ctx context.Context, messages []*Message) []*Result {
	results := make([]*Result, 0, len(messages))
	resultChan := make(chan *Result, len(messages))

	// Submit all messages
	for _, msg := range messages {
		wp.SubmitBlocking(msg)
	}

	// Collect results with timeout
	timeout := time.After(30 * time.Second)
	for i := 0; i < len(messages); i++ {
		select {
		case result := <-wp.results:
			results = append(results, result)
		case <-timeout:
			return results
		case <-ctx.Done():
			return results
		}
	}

	close(resultChan)
	return results
}
