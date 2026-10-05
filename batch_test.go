package db2

import (
	"math"
	"testing"
)

func TestDetectAndExtractBatch_HomogeneousSlices(t *testing.T) {
	ids := []int{1, 2, 3}
	names := []string{"Alpha", "Beta", "Gamma"}
	prices := []float64{10.5, 20.0, 30.25}

	rawArgs := []any{ids, names, prices}
	isBatch, batchRows, err := detectAndExtractBatch(rawArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isBatch {
		t.Fatalf("expected isBatch = true")
	}
	if len(batchRows) != 3 {
		t.Fatalf("expected 3 batch rows, got %d", len(batchRows))
	}

	// Verify row 0: [1, "Alpha", 10.5]
	if batchRows[0][0] != 1 || batchRows[0][1] != "Alpha" || batchRows[0][2] != 10.5 {
		t.Fatalf("row 0 mismatch: %v", batchRows[0])
	}
	// Verify row 2: [3, "Gamma", 30.25]
	if batchRows[2][0] != 3 || batchRows[2][1] != "Gamma" || batchRows[2][2] != 30.25 {
		t.Fatalf("row 2 mismatch: %v", batchRows[2])
	}
}

func TestDetectAndExtractBatch_MixedScalarAndSlice(t *testing.T) {
	ids := []int{1, 2, 3, 4}
	tenantID := "tenant-xyz" // scalar
	names := []string{"A", "B", "C", "D"}

	rawArgs := []any{ids, tenantID, names}
	isBatch, batchRows, err := detectAndExtractBatch(rawArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isBatch {
		t.Fatalf("expected isBatch = true")
	}
	if len(batchRows) != 4 {
		t.Fatalf("expected 4 batch rows, got %d", len(batchRows))
	}

	for i, row := range batchRows {
		if row[0] != i+1 || row[1] != "tenant-xyz" {
			t.Fatalf("row %d mismatch: %v", i, row)
		}
	}
}

func TestDetectAndExtractBatch_MismatchedLengths(t *testing.T) {
	ids := []int{1, 2, 3}
	names := []string{"A", "B"} // length 2 vs 3

	rawArgs := []any{ids, names}
	_, _, err := detectAndExtractBatch(rawArgs)
	if err == nil {
		t.Fatalf("expected error for mismatched slice lengths, got nil")
	}
}

func TestDetectAndExtractBatch_BytesNotTreatedAsBatch(t *testing.T) {
	blob := []byte{0x01, 0x02, 0x03, 0x04}
	name := "Sample"

	rawArgs := []any{blob, name}
	isBatch, _, err := detectAndExtractBatch(rawArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isBatch {
		t.Fatalf("expected []byte to be treated as scalar BLOB, not batch slice")
	}
}

func TestDetectAndExtractBatch_OverflowProtection(t *testing.T) {
	// [math.MaxInt/2 + 100]struct{} has 0-size elements so it requires 0 memory allocation,
	// but its reflect.Len() will trigger the integer overflow check when numCols >= 2.
	var hugeArray [math.MaxInt/2 + 100]struct{}
	rawArgs := []any{hugeArray, hugeArray}

	_, _, err := detectAndExtractBatch(rawArgs)
	if err == nil {
		t.Fatalf("expected error due to batchSize * numCols integer overflow, got nil")
	}
}

func BenchmarkDetectAndExtractBatch_100Rows(b *testing.B) {
	ids := make([]int, 100)
	names := make([]string, 100)
	prices := make([]float64, 100)
	for i := 0; i < 100; i++ {
		ids[i] = i
		names[i] = "item"
		prices[i] = float64(i) * 1.5
	}
	tenantID := "tenant-123"
	rawArgs := []any{ids, tenantID, names, prices}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := detectAndExtractBatch(rawArgs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDetectAndExtractBatch_1000Rows(b *testing.B) {
	ids := make([]int, 1000)
	names := make([]string, 1000)
	prices := make([]float64, 1000)
	for i := 0; i < 1000; i++ {
		ids[i] = i
		names[i] = "item"
		prices[i] = float64(i) * 1.5
	}
	tenantID := "tenant-123"
	rawArgs := []any{ids, tenantID, names, prices}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := detectAndExtractBatch(rawArgs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDetectAndExtractBatch_Scalar(b *testing.B) {
	blob := []byte{0x01, 0x02, 0x03, 0x04}
	rawArgs := []any{101, "sample-name", 99.9, blob}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := detectAndExtractBatch(rawArgs)
		if err != nil {
			b.Fatal(err)
		}
	}
}
