package decoder

import "testing"

func TestSetMaxWindowBitsReleasesOversizedRing(t *testing.T) {
	r := &Reader{
		ringbuffer:     make([]byte, 1<<20),
		ringbufferSize: 1 << 20,
		ringbufferMask: (1 << 20) - 1,
	}
	r.ringbufferEnd = r.ringbuffer[len(r.ringbuffer):]

	r.SetMaxWindowBits(10)
	if r.ringbuffer != nil {
		t.Fatalf("ring buffer retained %d bytes after lowering the window limit", cap(r.ringbuffer))
	}
	if r.ringbufferSize != 0 || r.ringbufferMask != 0 || r.pos != 0 {
		t.Fatalf("ring buffer state not cleared: size=%d mask=%d pos=%d", r.ringbufferSize, r.ringbufferMask, r.pos)
	}

	r.ringbuffer = make([]byte, 1<<10)
	r.SetMaxWindowBits(24)
	if r.ringbuffer == nil {
		t.Fatal("ring buffer was released while still within the limit")
	}
}
