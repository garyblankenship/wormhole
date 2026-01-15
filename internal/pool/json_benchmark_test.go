package pool

import (
	"encoding/json"
	"testing"
)

type testStruct struct {
	Name     string            `json:"name"`
	Age      int               `json:"age"`
	Tags     []string          `json:"tags"`
	Metadata map[string]string `json:"metadata"`
	Nested   *nestedStruct     `json:"nested"`
}

type nestedStruct struct {
	Value float64 `json:"value"`
	Flag  bool    `json:"flag"`
}

func BenchmarkJSONMarshalStandard(b *testing.B) {
	data := testStruct{
		Name: "Test User",
		Age:  30,
		Tags: []string{"go", "programming", "performance"},
		Metadata: map[string]string{
			"role":    "developer",
			"team":    "backend",
			"project": "wormhole",
		},
		Nested: &nestedStruct{
			Value: 3.14159,
			Flag:  true,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONMarshalPooled(b *testing.B) {
	data := testStruct{
		Name: "Test User",
		Age:  30,
		Tags: []string{"go", "programming", "performance"},
		Metadata: map[string]string{
			"role":    "developer",
			"team":    "backend",
			"project": "wormhole",
		},
		Nested: &nestedStruct{
			Value: 3.14159,
			Flag:  true,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf, err := Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
		Return(buf)
	}
}

func BenchmarkJSONMarshalStandardSmall(b *testing.B) {
	data := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONMarshalPooledSmall(b *testing.B) {
	data := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf, err := Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
		Return(buf)
	}
}

func BenchmarkJSONMarshalToString(b *testing.B) {
	data := testStruct{
		Name: "Test User",
		Age:  30,
		Tags: []string{"go", "programming", "performance"},
		Metadata: map[string]string{
			"role":    "developer",
			"team":    "backend",
			"project": "wormhole",
		},
		Nested: &nestedStruct{
			Value: 3.14159,
			Flag:  true,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := MarshalToString(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkJSONMarshalPooledReuse tests repeated marshaling where pool reuse matters
func BenchmarkJSONMarshalPooledReuse(b *testing.B) {
	// Test data of varying sizes to simulate real workload
	testCases := []testStruct{
		{
			Name: "Small",
			Age:  25,
			Tags: []string{"test"},
		},
		{
			Name: "Medium",
			Age:  30,
			Tags: []string{"go", "programming"},
			Metadata: map[string]string{
				"role": "developer",
			},
		},
		{
			Name: "Large",
			Age:  35,
			Tags: []string{"go", "programming", "performance", "optimization"},
			Metadata: map[string]string{
				"role":    "developer",
				"team":    "backend",
				"project": "wormhole",
				"goal":    "optimization",
			},
			Nested: &nestedStruct{
				Value: 3.14159,
				Flag:  true,
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Cycle through test cases
		data := testCases[i%len(testCases)]
		buf, err := Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
		Return(buf)
	}
}

func BenchmarkJSONMarshalStandardReuse(b *testing.B) {
	// Test data of varying sizes to simulate real workload
	testCases := []testStruct{
		{
			Name: "Small",
			Age:  25,
			Tags: []string{"test"},
		},
		{
			Name: "Medium",
			Age:  30,
			Tags: []string{"go", "programming"},
			Metadata: map[string]string{
				"role": "developer",
			},
		},
		{
			Name: "Large",
			Age:  35,
			Tags: []string{"go", "programming", "performance", "optimization"},
			Metadata: map[string]string{
				"role":    "developer",
				"team":    "backend",
				"project": "wormhole",
				"goal":    "optimization",
			},
			Nested: &nestedStruct{
				Value: 3.14159,
				Flag:  true,
			},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Cycle through test cases
		data := testCases[i%len(testCases)]
		_, err := json.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}