package drain

import (
	"testing"
	"time"
)

func TestDrainTree_Parse(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		logs     []string
		wantNew  []bool
		wantVars int
	}{
		{
			name: "Similar error logs should group together",
			logs: []string{
				"Error connecting to database at 192.168.1.1:5432",
				"Error connecting to database at 192.168.1.2:5432",
				"Error connecting to database at 10.0.0.1:5432",
			},
			wantNew:  []bool{true, false, false},
			wantVars: 2, // IP and port
		},
		{
			name: "Different log patterns should create different clusters",
			logs: []string{
				"User john logged in from 192.168.1.1",
				"User jane logged in from 192.168.1.2",
				"Server started on port 8080",
			},
			wantNew:  []bool{true, false, true},
			wantVars: 2, // username and IP for first pattern
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := NewDrainTree(config)
			timestamp := time.Now().UnixNano()

			for i, log := range tt.logs {
				result, err := tree.Parse(log, timestamp)
				if err != nil {
					t.Fatalf("Parse failed: %v", err)
				}

				if result.IsNew != tt.wantNew[i] {
					t.Errorf("Log %d: expected IsNew=%v, got %v", i, tt.wantNew[i], result.IsNew)
				}

				if result.TemplateID == "" {
					t.Errorf("Log %d: TemplateID should not be empty", i)
				}
			}
		})
	}
}

func TestDrainTree_ExtractVariables(t *testing.T) {
	config := DefaultConfig()
	dt := NewDrainTree(config)
	timestamp := time.Now().UnixNano()

	// First log creates template
	_, err := dt.Parse("Error code 500 at 192.168.1.1", timestamp)
	if err != nil {
		t.Fatalf("First parse failed: %v", err)
	}

	// Second log should extract variables
	result, err := dt.Parse("Error code 404 at 10.0.0.1", timestamp)
	if err != nil {
		t.Fatalf("Second parse failed: %v", err)
	}

	if len(result.Variables) == 0 {
		t.Error("Expected variables to be extracted")
	}
}

func TestDrainTree_ClusterCount(t *testing.T) {
	config := DefaultConfig()
	dt := NewDrainTree(config)
	timestamp := time.Now().UnixNano()

	logs := []string{
		"Pattern A with value 1",
		"Pattern A with value 2",
		"Pattern B with id 100",
		"Pattern B with id 200",
		"Pattern C started",
	}

	for _, log := range logs {
		_, err := dt.Parse(log, timestamp)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
	}

	count := dt.ClusterCount()
	if count < 2 || count > 5 {
		t.Errorf("Expected 2-5 clusters, got %d", count)
	}
}

func TestDrainTree_Stats(t *testing.T) {
	config := DefaultConfig()
	dt := NewDrainTree(config)
	timestamp := time.Now().UnixNano()

	// Add some logs
	for i := 0; i < 10; i++ {
		dt.Parse("Request processed in 100ms", timestamp)
	}

	stats := dt.GetStats()
	if stats.TotalLogs != 10 {
		t.Errorf("Expected 10 total logs, got %d", stats.TotalLogs)
	}

	if stats.TotalClusters != 1 {
		t.Errorf("Expected 1 cluster, got %d", stats.TotalClusters)
	}

	if stats.AverageSize != 10.0 {
		t.Errorf("Expected average size 10.0, got %f", stats.AverageSize)
	}
}

func BenchmarkDrainTree_Parse(b *testing.B) {
	config := DefaultConfig()
	dt := NewDrainTree(config)
	timestamp := time.Now().UnixNano()

	logs := []string{
		"Error connecting to database at 192.168.1.1:5432",
		"Request processed in 150ms for user abc123",
		"Memory usage at 75% on node server-01",
		"Connection timeout after 30s from 10.0.0.5",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dt.Parse(logs[i%len(logs)], timestamp)
	}
}

func BenchmarkDrainTree_ParseParallel(b *testing.B) {
	config := DefaultConfig()
	dt := NewDrainTree(config)
	timestamp := time.Now().UnixNano()

	logs := []string{
		"Error connecting to database at 192.168.1.1:5432",
		"Request processed in 150ms for user abc123",
		"Memory usage at 75% on node server-01",
		"Connection timeout after 30s from 10.0.0.5",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			dt.Parse(logs[i%len(logs)], timestamp)
			i++
		}
	})
}
