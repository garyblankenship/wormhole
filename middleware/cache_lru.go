package middleware

import (
	"sync"
	"time"
)

// LRUCache implements a Least Recently Used cache
type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	cache    map[string]*lruNode
	head     *lruNode
	tail     *lruNode
}

type lruNode struct {
	key   string
	value any
	prev  *lruNode
	next  *lruNode
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(capacity int) *LRUCache {
	lru := &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*lruNode),
	}

	// Create sentinel nodes
	lru.head = &lruNode{}
	lru.tail = &lruNode{}
	lru.head.next = lru.tail
	lru.tail.prev = lru.head

	return lru
}

// Get retrieves a value from the LRU cache
func (lru *LRUCache) Get(key string) (any, bool) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	node, exists := lru.cache[key]
	if !exists {
		return nil, false
	}

	// Move to front
	lru.moveToFront(node)

	return node.value, true
}

// Set stores a value in the LRU cache
func (lru *LRUCache) Set(key string, value any, ttl time.Duration) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if node, exists := lru.cache[key]; exists {
		// Update existing node
		node.value = value
		lru.moveToFront(node)
		return
	}

	// Add new node
	node := &lruNode{
		key:   key,
		value: value,
	}

	lru.cache[key] = node
	lru.addToFront(node)

	// Evict if over capacity
	if len(lru.cache) > lru.capacity {
		lru.evictLRU()
	}
}

// Delete removes a value from the LRU cache
func (lru *LRUCache) Delete(key string) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	if node, exists := lru.cache[key]; exists {
		lru.removeNode(node)
		delete(lru.cache, key)
	}
}

// Clear removes all entries from the LRU cache
func (lru *LRUCache) Clear() {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	lru.cache = make(map[string]*lruNode)
	lru.head.next = lru.tail
	lru.tail.prev = lru.head
}

func (lru *LRUCache) moveToFront(node *lruNode) {
	lru.removeNode(node)
	lru.addToFront(node)
}

func (lru *LRUCache) addToFront(node *lruNode) {
	node.prev = lru.head
	node.next = lru.head.next
	lru.head.next.prev = node
	lru.head.next = node
}

func (lru *LRUCache) removeNode(node *lruNode) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (lru *LRUCache) evictLRU() {
	node := lru.tail.prev
	lru.removeNode(node)
	delete(lru.cache, node.key)
}

// Close implements Cache interface for LRUCache
func (lru *LRUCache) Close() error {
	// LRUCache has no cleanup goroutines to stop.
	return nil
}
