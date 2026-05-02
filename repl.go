package main

import (
	"sync"
)

// replState manages the async REPL state machine.
// Matching Claude Code's QueryGuard + CommandQueue architecture:
// - User input submitted while agent runs goes into a priority queue
// - Background agent notifications go in as lower priority
// - Ctrl+C is highest priority interrupt
// - REPL remains responsive: accepts input, displays notifications
//   while the agent is processing
type replState struct {
	mu           sync.Mutex
	guard        *queryGuard
	queue        *commandQueue
	agentRunning bool
	resultCh     chan string
	doneCh       chan struct{} // closed when agent finishes
	interruptCh  chan struct{}
}

type queryGuard struct {
	mu    sync.Mutex
	state int    // 0=idle, 1=running
	gen   uint64 // generation counter
}

func newQueryGuard() *queryGuard {
	return &queryGuard{state: 0}
}

func (g *queryGuard) tryStart() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state != 0 {
		return 0
	}
	g.gen++
	g.state = 1
	return g.gen
}

func (g *queryGuard) end(gen uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gen == gen {
		g.state = 0
	}
}

func (g *queryGuard) forceEnd() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gen++
	g.state = 0
}

func (g *queryGuard) isActive() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state != 0
}

type commandQueue struct {
	mu      sync.Mutex
	entries []queuedCommand
}

type queuedCommand struct {
	value    string
	priority int // 0=now (interrupt), 1=next (user input), 2=later (notifications)
}

func newCommandQueue() *commandQueue {
	return &commandQueue{}
}

func (q *commandQueue) enqueue(value string, priority int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries = append(q.entries, queuedCommand{value: value, priority: priority})
}

func (q *commandQueue) enqueueInput(value string) { q.enqueue(value, 1) }
func (q *commandQueue) enqueueNotification(value string) { q.enqueue(value, 2) }
func (q *commandQueue) enqueueInterrupt(value string) { q.enqueue(value, 0) }

func (q *commandQueue) dequeue() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.entries) == 0 {
		return "", false
	}
	best := 0
	for i := 1; i < len(q.entries); i++ {
		if q.entries[i].priority < q.entries[best].priority {
			best = i
		}
	}
	cmd := q.entries[best]
	q.entries = append(q.entries[:best], q.entries[best+1:]...)
	return cmd.value, true
}

func (q *commandQueue) hasInput() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, e := range q.entries {
		if e.priority == 1 {
			return true
		}
	}
	return false
}

func newReplState() *replState {
	return &replState{
		guard:       newQueryGuard(),
		queue:       newCommandQueue(),
		resultCh:    make(chan string, 1),
		doneCh:      make(chan struct{}),
		interruptCh: make(chan struct{}, 1),
	}
}

// startAgent launches the agent in a goroutine. Returns true if started,
// false if already running (input was queued instead).
func (s *replState) startAgent(input string, runAgent func(string) string) bool {
	s.mu.Lock()
	if s.agentRunning {
		s.mu.Unlock()
		s.queue.enqueueInput(input)
		return false
	}
	s.agentRunning = true
	// Reset channels
	s.resultCh = make(chan string, 1)
	s.doneCh = make(chan struct{})
	s.mu.Unlock()

	gen := s.guard.tryStart()
	if gen == 0 {
		s.mu.Lock()
		s.agentRunning = false
		s.mu.Unlock()
		return false
	}

	go func() {
		defer close(s.doneCh)
		result := runAgent(input)
		s.guard.end(gen)
		select {
		case s.resultCh <- result:
		default:
		}
	}()
	return true
}

// submitInput handles user input: if agent is running, enqueue; otherwise start.
func (s *replState) submitInput(input string, runAgent func(string) string) bool {
	s.mu.Lock()
	running := s.agentRunning
	s.mu.Unlock()

	if running {
		s.queue.enqueueInput(input)
		return false
	}
	return s.startAgent(input, runAgent)
}

// submitNotification queues a background agent completion notification.
func (s *replState) submitNotification(notification string) {
	s.queue.enqueueNotification(notification)
}

// isAgentRunning returns whether the agent is currently processing.
func (s *replState) isAgentRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agentRunning
}

// hasQueuedInput returns true if user input is waiting.
func (s *replState) hasQueuedInput() bool {
	return s.queue.hasInput()
}

// waitForDone returns the done channel. Non-blocking callers can select on it.
func (s *replState) waitForDone() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.doneCh
}

// getResult returns the agent result if available, blocking until done.
func (s *replState) getResult() string {
	return <-s.resultCh
}

// markFinished marks the agent as finished (used when result was consumed).
func (s *replState) markFinished() {
	s.mu.Lock()
	s.agentRunning = false
	s.mu.Unlock()
}

// processDrainedNotifications handles drained notifications from the agent's
// notification channel while it's running. These get enqueued as PriorityLater.
func (s *replState) processDrainedNotifications(notifications []string) {
	for _, n := range notifications {
		s.queue.enqueueNotification(n)
	}
}

// isRunning returns whether the agent is currently processing.
func (s *replState) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agentRunning
}
