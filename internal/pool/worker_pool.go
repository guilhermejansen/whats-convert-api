package pool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Task represents a unit of work to be executed
type Task func() error

// TaskWithContext represents a task that accepts context
type TaskWithContext func(context.Context) error

// WorkerPool manages a pool of goroutines for concurrent task execution
type WorkerPool struct {
	maxWorkers   int
	taskQueue    chan Task
	contextQueue chan contextTask
	workerWg     sync.WaitGroup
	quit         chan struct{}
	activeCount  int32
	totalTasks   int64
	failedTasks  int64
	avgExecTime  int64 // nanoseconds
	started      bool
	mu           sync.RWMutex
}

type contextTask struct {
	ctx  context.Context
	task TaskWithContext
	done chan error
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(maxWorkers int) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}

	return &WorkerPool{
		maxWorkers:   maxWorkers,
		taskQueue:    make(chan Task, maxWorkers*10), // Buffered queue
		contextQueue: make(chan contextTask, maxWorkers*10),
		quit:         make(chan struct{}),
	}
}

// Start initializes and starts all workers
func (p *WorkerPool) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("worker pool already started")
	}

	for i := 0; i < p.maxWorkers; i++ {
		p.workerWg.Add(1)
		go p.worker(i)
	}

	p.started = true
	return nil
}

// worker is the main goroutine that processes tasks
func (p *WorkerPool) worker(id int) {
	defer p.workerWg.Done()

	for {
		select {
		case task := <-p.taskQueue:
			if task == nil {
				continue
			}

			start := time.Now()
			atomic.AddInt32(&p.activeCount, 1)
			atomic.AddInt64(&p.totalTasks, 1)

			if err := task(); err != nil {
				atomic.AddInt64(&p.failedTasks, 1)
			}

			elapsed := time.Since(start).Nanoseconds()
			// Update average execution time (simple moving average)
			oldAvg := atomic.LoadInt64(&p.avgExecTime)
			newAvg := (oldAvg*9 + elapsed) / 10
			atomic.StoreInt64(&p.avgExecTime, newAvg)

			atomic.AddInt32(&p.activeCount, -1)

		case ctxTask := <-p.contextQueue:
			if ctxTask.task == nil {
				continue
			}

			start := time.Now()
			atomic.AddInt32(&p.activeCount, 1)
			atomic.AddInt64(&p.totalTasks, 1)

			err := ctxTask.task(ctxTask.ctx)
			if err != nil {
				atomic.AddInt64(&p.failedTasks, 1)
			}

			elapsed := time.Since(start).Nanoseconds()
			oldAvg := atomic.LoadInt64(&p.avgExecTime)
			newAvg := (oldAvg*9 + elapsed) / 10
			atomic.StoreInt64(&p.avgExecTime, newAvg)

			atomic.AddInt32(&p.activeCount, -1)

			// Send result back if channel provided
			if ctxTask.done != nil {
				select {
				case ctxTask.done <- err:
				case <-ctxTask.ctx.Done():
				}
			}

		case <-p.quit:
			return
		}
	}
}

// Submit adds a task to the queue
func (p *WorkerPool) Submit(task Task) error {
	p.mu.RLock()
	if !p.started {
		p.mu.RUnlock()
		return fmt.Errorf("worker pool not started")
	}
	p.mu.RUnlock()

	select {
	case p.taskQueue <- task:
		return nil
	default:
		// Queue is full, execute in new goroutine as fallback
		go func() {
			atomic.AddInt32(&p.activeCount, 1)
			atomic.AddInt64(&p.totalTasks, 1)

			if err := task(); err != nil {
				atomic.AddInt64(&p.failedTasks, 1)
			}

			atomic.AddInt32(&p.activeCount, -1)
		}()
		return nil
	}
}

// SubmitWithContext submits a task with context and returns error channel
func (p *WorkerPool) SubmitWithContext(ctx context.Context, task TaskWithContext) (<-chan error, error) {
	p.mu.RLock()
	if !p.started {
		p.mu.RUnlock()
		return nil, fmt.Errorf("worker pool not started")
	}
	p.mu.RUnlock()

	done := make(chan error, 1)
	ctxTask := contextTask{
		ctx:  ctx,
		task: task,
		done: done,
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case p.contextQueue <- ctxTask:
		return done, nil
	default:
		// Queue is full, execute in new goroutine
		go func() {
			atomic.AddInt32(&p.activeCount, 1)
			atomic.AddInt64(&p.totalTasks, 1)

			err := task(ctx)
			if err != nil {
				atomic.AddInt64(&p.failedTasks, 1)
			}

			atomic.AddInt32(&p.activeCount, -1)

			select {
			case done <- err:
			case <-ctx.Done():
			}
		}()
		return done, nil
	}
}

// SubmitBatch submits multiple tasks and waits for all to complete
func (p *WorkerPool) SubmitBatch(ctx context.Context, tasks []Task) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(tasks))

	for _, task := range tasks {
		wg.Add(1)
		taskCopy := task // Capture loop variable

		err := p.Submit(func() error {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return taskCopy()
			}
		})

		if err != nil {
			wg.Done()
			return err
		}
	}

	// Wait for all tasks in goroutine
	go func() {
		wg.Wait()
		close(errChan)
	}()

	// Collect any errors
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		if err != nil {
			return err
		}
	}

	return nil
}

// Stop gracefully shuts down the worker pool
func (p *WorkerPool) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return
	}

	close(p.quit)
	p.workerWg.Wait()
	p.started = false
}

// ActiveWorkers returns the number of currently active workers
func (p *WorkerPool) ActiveWorkers() int32 {
	return atomic.LoadInt32(&p.activeCount)
}

// QueueSize returns the current number of tasks in queue
func (p *WorkerPool) QueueSize() int {
	return len(p.taskQueue) + len(p.contextQueue)
}

// WorkerPoolStats holds statistics about the worker pool
type WorkerPoolStats struct {
	MaxWorkers    int
	ActiveWorkers int32
	QueueSize     int
	TotalTasks    int64
	FailedTasks   int64
	SuccessRate   float64
	AvgExecTimeMs float64
}

// Stats returns current pool statistics
func (p *WorkerPool) Stats() WorkerPoolStats {
	total := atomic.LoadInt64(&p.totalTasks)
	failed := atomic.LoadInt64(&p.failedTasks)
	avgNs := atomic.LoadInt64(&p.avgExecTime)

	successRate := float64(0)
	if total > 0 {
		successRate = float64(total-failed) / float64(total)
	}

	return WorkerPoolStats{
		MaxWorkers:    p.maxWorkers,
		ActiveWorkers: atomic.LoadInt32(&p.activeCount),
		QueueSize:     p.QueueSize(),
		TotalTasks:    total,
		FailedTasks:   failed,
		SuccessRate:   successRate,
		AvgExecTimeMs: float64(avgNs) / 1e6,
	}
}
