// Package filemutationqueue serializes concurrent file mutations targeting the same path.
// Aligned to pi's tools/file-mutation-queue.ts.
package filemutationqueue

import (
	"sync"
)

// mutationChain represents a serialized chain of operations for a single file path.
type mutationChain struct {
	mu   sync.Mutex
	cond *sync.Cond
	busy bool
}

// FileMutationQueue serializes concurrent file mutations per path.
// Operations on different files run in parallel; operations on the same file run serially.
type FileMutationQueue struct {
	mu    sync.Mutex
	chains map[string]*mutationChain
}

// NewFileMutationQueue creates a new file mutation queue.
func NewFileMutationQueue() *FileMutationQueue {
	return &FileMutationQueue{
		chains: make(map[string]*mutationChain),
	}
}

// WithFileMutationQueue runs fn serialized per filePath.
// Concurrent calls with the same filePath will run one at a time.
// Calls with different filePaths run in parallel.
func (q *FileMutationQueue) WithFileMutationQueue(filePath string, fn func() error) error {
	chain := q.getChain(filePath)

	chain.mu.Lock()
	for chain.busy {
		chain.cond.Wait()
	}
	chain.busy = true
	chain.mu.Unlock()

	// Run the operation
	err := fn()

	chain.mu.Lock()
	chain.busy = false
	chain.cond.Signal()
	chain.mu.Unlock()

	return err
}

func (q *FileMutationQueue) getChain(filePath string) *mutationChain {
	q.mu.Lock()
	defer q.mu.Unlock()

	chain, ok := q.chains[filePath]
	if !ok {
		chain = &mutationChain{}
		chain.cond = sync.NewCond(&chain.mu)
		q.chains[filePath] = chain
	}
	return chain
}