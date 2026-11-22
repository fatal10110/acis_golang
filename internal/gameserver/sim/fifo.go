package sim

import "slices"

// fifo is a FIFO over one reusable buffer: items are read from head, so
// emptying it keeps the backing array for the next push instead of slicing
// its capacity away. Callers synchronize it.
type fifo[T any] struct {
	buf  []T
	head int // index of the oldest item in buf
}

func (f *fifo[T]) len() int { return len(f.buf) - f.head }

// push appends v. A full buffer whose consumed prefix is at least half of it
// is compacted in place instead of grown, so a queue that never empties
// still reuses its buffer at amortized O(1) per item.
func (f *fifo[T]) push(v T) {
	if len(f.buf) == cap(f.buf) && f.head > 0 && f.head >= len(f.buf)/2 {
		n := copy(f.buf, f.buf[f.head:])
		clear(f.buf[n:])
		f.buf, f.head = f.buf[:n], 0
	}
	f.buf = append(f.buf, v)
}

// pop removes and returns the oldest item. f must not be empty.
func (f *fifo[T]) pop() T {
	v := f.buf[f.head]
	var zero T
	f.buf[f.head] = zero
	f.head++
	f.rewind()
	return v
}

// take moves up to len(dst) of the oldest items into dst and returns how
// many it moved.
func (f *fifo[T]) take(dst []T) int {
	n := copy(dst, f.buf[f.head:])
	clear(f.buf[f.head : f.head+n])
	f.head += n
	f.rewind()
	return n
}

// unshift puts items back in front of the oldest item, in order.
func (f *fifo[T]) unshift(items []T) {
	if f.head >= len(items) {
		f.head -= len(items)
		copy(f.buf[f.head:], items)
		return
	}
	f.buf, f.head = slices.Insert(f.buf[f.head:], 0, items...), 0
}

// release drops the buffer of an empty f when its capacity is above keep, so
// an idle owner does not pin the array one burst grew.
func (f *fifo[T]) release(keep int) {
	if f.len() == 0 && cap(f.buf) > keep {
		f.buf, f.head = nil, 0
	}
}

// rewind restarts an emptied f at the front of its buffer.
func (f *fifo[T]) rewind() {
	if f.head == len(f.buf) {
		f.buf, f.head = f.buf[:0], 0
	}
}
