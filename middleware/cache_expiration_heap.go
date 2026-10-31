package middleware

import "container/heap"

// expirationHeap has exactly one indexed node per entry in MemoryCache.entries.
// All operations run while MemoryCache.mu is held.
type expirationHeap []*cacheEntry

func (h expirationHeap) Len() int { return len(h) }

func (h expirationHeap) Less(i, j int) bool {
	return h[i].expiration.Before(h[j].expiration)
}

func (h expirationHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}

func (h *expirationHeap) Push(value any) {
	entry := value.(*cacheEntry)
	entry.heapIndex = len(*h)
	*h = append(*h, entry)
}

func (h *expirationHeap) Pop() any {
	old := *h
	last := len(old) - 1
	entry := old[last]
	old[last] = nil
	*h = old[:last]
	entry.heapIndex = -1
	return entry
}

func (h *expirationHeap) add(entry *cacheEntry) {
	heap.Push(h, entry)
}

func (h *expirationHeap) fix(entry *cacheEntry) {
	heap.Fix(h, entry.heapIndex)
}

func (h *expirationHeap) remove(entry *cacheEntry) {
	heap.Remove(h, entry.heapIndex)
}
