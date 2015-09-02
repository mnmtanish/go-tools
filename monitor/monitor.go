package monitor

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/kadirahq/go-tools/logger"
)

// Type is the metric type
type Type uint8

// Metric types
const (
	Gauge Type = iota
	Counter
	Rate
)

var (
	// default metric store
	store = newStore("app")
)

// New creates a sub collection using the default metric store
func New(head string) (s *Store) {
	return store.New(head)
}

// Register registers a metric using the default metric store
func Register(k string, kind Type) {
	store.Register(k, kind)
}

// Track tracks a metric using the default metric store
func Track(k string, n int64) {
	store.Track(k, n)
}

// Values returns values stored in the default metric store
func Values() (res map[string]int64) {
	return store.Values()
}

// Print logs using the default metric store
func Print(dur time.Duration) (ch chan bool) {
	return store.Print(dur)
}

//   Store
// ---------

// Store is a collection of application metrics
type Store struct {
	head string
	vals map[string]metric
	subs map[string]*Store
}

func newStore(head string) *Store {
	return &Store{
		head: head,
		vals: map[string]metric{},
		subs: map[string]*Store{},
	}
}

// New returns a child store by extending the header
func (s *Store) New(head string) (sub *Store) {
	if sub, ok := s.subs[head]; ok {
		return sub
	}

	key := s.head + "." + head
	return newStore(key)
}

// Register a new metric to measure later
func (s *Store) Register(k string, t Type) {
	k = s.head + ":" + k
	if _, ok := s.vals[k]; !ok {
		switch t {
		case Gauge:
			s.vals[k] = &gauge{}
		case Counter:
			s.vals[k] = &counter{}
		case Rate:
			s.vals[k] = &rate{}
		}
	}
}

// Track records a new value for a metric. Metric should be
// registered before tracking values.
func (s *Store) Track(k string, n int64) {
	k = s.head + ":" + k
	if m, ok := s.vals[k]; ok {
		m.Track(n)
	} else {
		logger.Debug("unregistered monitor key", k)
		s.vals[k] = &counter{}
	}
}

// Values returns all values as a map
func (s *Store) Values() (res map[string]int64) {
	res = map[string]int64{}
	for k, m := range s.vals {
		res[k] = m.Value()
	}

	return res
}

// Print logs application metrics to stdout with given interval
// The "metrics" log level should be enabled for this to work.
// It will also log all children metric stores recursively.
func (s *Store) Print(dur time.Duration) (ch chan bool) {
	ch = make(chan bool)

	go func() {
		for {
			select {
			case <-ch:
				break
			case <-time.After(dur):
				s.log()
			}
		}
	}()

	return ch
}

func (s *Store) log() {
	logger.Print("metrics", s.head, s.Values())
	for _, sub := range s.subs {
		sub.log()
	}
}

//   metric
// ----------

type metric interface {
	Value() (val int64)
	Track(n int64)
}

//   gauge
// ---------

type gauge struct {
	val int64
}

func (c *gauge) Value() (val int64) {
	val = atomic.LoadInt64(&c.val)
	for !atomic.CompareAndSwapInt64(&c.val, val, 0) {
		val = atomic.LoadInt64(&c.val)
	}

	return val
}

func (c *gauge) Track(n int64) {
	atomic.StoreInt64(&c.val, n)
}

//   counter
// -----------

type counter struct {
	val int64
}

func (c *counter) Value() (val int64) {
	val = atomic.LoadInt64(&c.val)
	for !atomic.CompareAndSwapInt64(&c.val, val, 0) {
		val = atomic.LoadInt64(&c.val)
	}

	return val
}

func (c *counter) Track(n int64) {
	atomic.AddInt64(&c.val, n)
}

//   rate
// --------

type rate struct {
	mtx sync.Mutex
	val int64
	ts0 int64
}

func (c *rate) Value() (val int64) {
	c.mtx.Lock()

	if now := time.Now().Unix(); now > c.ts0 {
		val = c.val / (now - c.ts0)
		c.ts0 = now
		c.val = 0
	}

	c.mtx.Unlock()
	return val
}

func (c *rate) Track(n int64) {
	c.mtx.Lock()

	c.val += n
	if c.ts0 == 0 {
		c.ts0 = time.Now().Unix()
	}

	c.mtx.Unlock()
}
