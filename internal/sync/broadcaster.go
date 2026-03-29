package sync

import "sync"

// 一次性广播器.
type Broadcaster[T any] struct {
	ready bool
	cond  *sync.Cond
	data  T
}

func NewBroadcaster[T any]() *Broadcaster[T] {
	return &Broadcaster[T]{
		cond: sync.NewCond(&sync.Mutex{}),
	}
}

// 发送广播 (重复调用会 panic).
func (b *Broadcaster[T]) Store(data T) {
	if b.ready {
		panic("重复发送广播")
	}

	b.cond.L.Lock()
	b.data = data
	b.ready = true
	b.cond.Broadcast()
	b.cond.L.Unlock()
}

// 阻塞接受广播.
func (b *Broadcaster[T]) Load() T {
	b.cond.L.Lock()
	for !b.ready {
		b.cond.Wait()
	}
	b.cond.L.Unlock()
	return b.data
}
