package matchfinder

import (
	"io"
	"sync"
)

// A ParallelWriter is like a Writer, but it can compress multiple blocks in parallel.
type ParallelWriter struct {
	Dest        io.Writer
	MatchFinder func() MatchFinder // Factory function for per-goroutine MatchFinders
	Encoder     func() Encoder     // Factory function for per-goroutine Encoders

	// BlockSize is the number of bytes to compress at a time.
	BlockSize int

	// Concurrency is the number of blocks to compress in parallel.
	Concurrency int

	err    error
	inBuf  []byte
	nextID int
	
	results   chan blockResult
	pending   map[int][]byte
	nextToOut int
	wg        sync.WaitGroup
	mu        sync.Mutex
	once      sync.Once
}

type blockResult struct {
	id   int
	data []byte
	err  error
}

func (w *ParallelWriter) init() {
	w.once.Do(func() {
		if w.Concurrency <= 0 {
			w.Concurrency = 1
		}
		w.results = make(chan blockResult, w.Concurrency*2)
		w.pending = make(map[int][]byte)
		go w.drain()
	})
}

func (w *ParallelWriter) drain() {
	for res := range w.results {
		w.mu.Lock()
		if res.err != nil && w.err == nil {
			w.err = res.err
		}
		w.pending[res.id] = res.data
		
		for {
			data, ok := w.pending[w.nextToOut]
			if !ok {
				break
			}
			delete(w.pending, w.nextToOut)
			w.nextToOut++
			w.mu.Unlock()
			
			if w.err == nil {
				_, w.err = w.Dest.Write(data)
			}
			w.wg.Done()
			w.mu.Lock()
		}
		w.mu.Unlock()
	}
}

func (w *ParallelWriter) Write(p []byte) (n int, err error) {
	w.init()
	if w.err != nil {
		return 0, w.err
	}

	total := len(p)
	w.inBuf = append(w.inBuf, p...)
	for len(w.inBuf) >= w.BlockSize {
		block := make([]byte, w.BlockSize)
		copy(block, w.inBuf[:w.BlockSize])
		w.inBuf = w.inBuf[w.BlockSize:]
		
		w.wg.Add(1)
		id := w.nextID
		w.nextID++
		
		go func(id int, data []byte) {
			mf := w.MatchFinder()
			e := w.Encoder()
			matches := mf.FindMatches(nil, data)
			out := e.Encode(nil, data, matches, false)
			w.results <- blockResult{id: id, data: out}
			
			// If these interfaces implement Close, we can return them to a pool.
			if c, ok := mf.(io.Closer); ok {
				c.Close()
			}
			if c, ok := e.(io.Closer); ok {
				c.Close()
			}
		}(id, block)
	}

	return total, w.err
}

func (w *ParallelWriter) Close() error {
	w.init()
	if len(w.inBuf) > 0 {
		w.wg.Add(1)
		id := w.nextID
		w.nextID++
		go func(id int, data []byte) {
			mf := w.MatchFinder()
			e := w.Encoder()
			matches := mf.FindMatches(nil, data)
			out := e.Encode(nil, data, matches, true)
			w.results <- blockResult{id: id, data: out}
			
			if c, ok := mf.(io.Closer); ok {
				c.Close()
			}
			if c, ok := e.(io.Closer); ok {
				c.Close()
			}
		}(id, w.inBuf)
		w.inBuf = nil
	}

	w.wg.Wait()
	close(w.results)
	return w.err
}

var bargain1Pool = sync.Pool{
	New: func() any {
		return &Bargain1{MaxDistance: 1 << 20}
	},
}

func GetBargain1() *Bargain1 {
	return bargain1Pool.Get().(*Bargain1)
}

func putBargain1(z *Bargain1) {
	bargain1Pool.Put(z)
}

var encoderPool = sync.Pool{
	New: func() any {
		return nil
	},
}

func PutEncoder(e Encoder) {
	encoderPool.Put(e)
}

func GetEncoder(f func() Encoder) Encoder {
	if v := encoderPool.Get(); v != nil {
		return v.(Encoder)
	}
	return f()
}

var fastEncoderPool = sync.Pool{
	New: func() any {
		return nil
	},
}

func PutFastEncoder(e Encoder) {
	fastEncoderPool.Put(e)
}

func GetFastEncoder(f func() Encoder) Encoder {
	if v := fastEncoderPool.Get(); v != nil {
		return v.(Encoder)
	}
	return f()
}
