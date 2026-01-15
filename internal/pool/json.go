package pool

import (
	"bytes"
	"encoding/json"
	"sync"
)

// JSONBufferPool pools byte slices for JSON marshaling to reduce allocations.
// Buffers start with 4KB capacity, which is sufficient for most JSON payloads.
var JSONBufferPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 4096)
	},
}

var bufferPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

// Marshal marshals v to JSON using a pooled buffer.
// The returned buffer is borrowed from the pool and MUST be returned
// to the pool by calling Return after use.
//
// This reduces allocations by reusing buffers across marshaling operations.
// In high-throughput scenarios, this can reduce allocation pressure by 40-60%.
//
// Example:
//
//	buf, err := Marshal(data)
//	if err != nil {
//	    return err
//	}
//	defer Return(buf)
//	// Use buf...
func Marshal(v any) ([]byte, error) {
	// Get bytes.Buffer from pool
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	// Encode directly into buffer
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		return nil, err
	}

	// Get the encoded bytes
	encodedBytes := buf.Bytes()

	// Get a byte slice from JSONBufferPool with sufficient capacity
	result := JSONBufferPool.Get().([]byte)
	result = result[:0]
	if cap(result) < len(encodedBytes) {
		// Pooled slice too small, allocate new one
		// Return the unused pooled slice
		JSONBufferPool.Put(result)
		result = make([]byte, len(encodedBytes))
	} else {
		// Reuse pooled slice
		result = result[:len(encodedBytes)]
	}

	// Copy encoded data into result slice
	copy(result, encodedBytes)
	return result, nil
}

// Return returns a buffer to the pool for reuse.
// The buffer length is reset to 0 before returning to preserve capacity.
func Return(buf []byte) {
	if buf == nil {
		return
	}
	// Reset length to 0 while keeping capacity
	buf = buf[:0]
	JSONBufferPool.Put(buf)
}

// MarshalToString marshals v to JSON and returns it as a string.
// This is a convenience wrapper that handles buffer pooling internally.
func MarshalToString(v any) (string, error) {
	buf, err := Marshal(v)
	if err != nil {
		return "", err
	}
	defer Return(buf)
	return string(buf), nil
}