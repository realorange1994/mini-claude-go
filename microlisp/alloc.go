package microlisp

// -------- Batch Cell Allocation --------
// Pre-allocates Value cells to reduce GC overhead for cons-heavy operations.
// Inspired by golisp's batch cell allocation pattern.
//
// Simple approach: a large pre-allocated pool. After GC, we advance the
// allocation cursor past surviving cells to avoid overwriting them.
// Freed cells are not reused (trade-off: simplicity over reuse).

const batchCellSize = 262144 // 256K cells per pool

type cellPool struct {
	cells  []Value
	cursor int // next free index
}

var currentPool *cellPool

func newCellPool() *cellPool {
	return &cellPool{
		cells:  make([]Value, batchCellSize),
		cursor: 0,
	}
}

// batchAlloc returns a fresh *Value from the pre-allocated pool.
// Falls back to gcv() if all pools are exhausted.
func batchAlloc() *Value {
	if currentPool == nil {
		currentPool = newCellPool()
	}
	if currentPool.cursor >= len(currentPool.cells) {
		// Current pool full, create a new one
		currentPool = newCellPool()
	}

	v := &currentPool.cells[currentPool.cursor]
	// Zero all fields to ensure clean state
	*v = Value{}
	currentPool.cursor++

	allValues = append(allValues, v)
	if len(allValues) >= youngThreshold {
		gcollect()
	}
	return v
}

// batchGCollect is called after GC. It ensures the current pool's cursor
// is advanced past any surviving cells to avoid overwriting them.
// This is a conservative approach: we never reuse freed batch cells.
func batchGCollect() {
	// No action needed — we simply don't reuse freed cells.
	// The cursor already advances monotonically, and survivors
	// in allValues are compacted but their pool positions remain.
}
