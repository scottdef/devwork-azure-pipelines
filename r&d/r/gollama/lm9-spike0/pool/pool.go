// Package pool is the compute thread pool: a fixed set of goroutines, each
// locked to its own OS thread and optionally pinned to a core, with work
// split statically by row.
//
// The static split is what makes results independent of thread count. Row r
// is always computed by one worker, start to finish, with the same kernel, so
// one thread and sixteen threads produce the same bits.
package pool

import (
	"runtime"
	"sync"
)

// Pool runs row-parallel work on pinned threads.
type Pool struct {
	jobs []chan job
	wg   sync.WaitGroup
}

type job struct {
	lo, hi int
	fn     func(lo, hi int)
	done   *sync.WaitGroup
}

// New starts n workers. If pin is true each worker is bound to one of the
// CPUs this process is allowed to use, where the OS supports it.
func New(n int, pin bool) *Pool {
	if n < 1 {
		n = 1
	}
	cpus := allowedCPUs()
	p := &Pool{jobs: make([]chan job, n)}
	for w := range p.jobs {
		ch := make(chan job)
		p.jobs[w] = ch
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			// Kernels keep state in vector registers and must not migrate
			// mid-row; a locked thread is also the unit the OS can pin.
			runtime.LockOSThread()
			if pin && len(cpus) > 0 {
				_ = pinTo(cpus[w%len(cpus)]) // best effort
			}
			for j := range ch {
				j.fn(j.lo, j.hi)
				j.done.Done()
			}
		}()
	}
	return p
}

// Rows calls fn(lo, hi) once per worker with a contiguous range of rows, and
// returns when all have finished. Worker i always gets the same range for the
// same (rows, workers), so nothing depends on scheduling.
func (p *Pool) Rows(rows int, fn func(lo, hi int)) {
	n := len(p.jobs)
	var done sync.WaitGroup
	for w := 0; w < n; w++ {
		lo, hi := w*rows/n, (w+1)*rows/n
		if lo == hi {
			continue
		}
		done.Add(1)
		p.jobs[w] <- job{lo, hi, fn, &done}
	}
	done.Wait()
}

// Close stops the workers.
func (p *Pool) Close() {
	for _, ch := range p.jobs {
		close(ch)
	}
	p.wg.Wait()
}
