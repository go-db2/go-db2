package suite

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ConformanceResult represents the outcome of a semantic equality check.
type ConformanceResult struct {
	Name       string
	PureGoVal  string
	CgoVal     string
	Passed     bool
	Difference string
}

// PerfMetric records execution and memory metrics for a single driver run.
type PerfMetric struct {
	Duration   time.Duration
	Ops        int
	BytesAlloc uint64
	Allocs     uint64
}

func (m PerfMetric) OpsPerSec() float64 {
	if m.Duration == 0 {
		return 0
	}
	return float64(m.Ops) / m.Duration.Seconds()
}

func (m PerfMetric) AvgLatency() time.Duration {
	if m.Ops == 0 {
		return 0
	}
	return m.Duration / time.Duration(m.Ops)
}

func (m PerfMetric) BytesPerOp() uint64 {
	if m.Ops == 0 {
		return 0
	}
	return m.BytesAlloc / uint64(m.Ops)
}

func (m PerfMetric) AllocsPerOp() uint64 {
	if m.Ops == 0 {
		return 0
	}
	return m.Allocs / uint64(m.Ops)
}

// BenchmarkResult stores the comparison between Pure Go and CGO for a test.
type BenchmarkResult struct {
	Name    string
	PureGo  PerfMetric
	CgoIBM  PerfMetric
	Summary string
}

// ReportGenerator renders and saves benchmark and conformance results.
type ReportGenerator struct {
	Conformance []ConformanceResult
	Benchmarks  []BenchmarkResult
}

// PrintConsole outputs a clean formatted summary to stdout.
func (r *ReportGenerator) PrintConsole() {
	fmt.Println("\n==========================================================================================================")
	fmt.Println("                            📊 DB2 DRIVER BENCHMARK & CONFORMANCE REPORT")
	fmt.Println("                            go-db2 (Pure Go / DRDA) vs go_ibm_db (CGO / CLI)")
	fmt.Println("==========================================================================================================")

	fmt.Println("\n--- 1. SEMANTIC CONFORMANCE (RESULT EQUALITY) ---")
	passedCount := 0
	for _, c := range r.Conformance {
		status := "✅ PASS"
		if !c.Passed {
			status = "❌ FAIL"
		} else {
			passedCount++
		}
		fmt.Printf(" %s | %-32s | Pure-Go: %-20s | CGO: %-20s\n", status, c.Name, truncate(c.PureGoVal, 20), truncate(c.CgoVal, 20))
		if !c.Passed && c.Difference != "" {
			fmt.Printf("      ⚠️ Diff: %s\n", c.Difference)
		}
	}
	fmt.Printf("\nConformance Summary: %d/%d tests passed (%.1f%%)\n", passedCount, len(r.Conformance), float64(passedCount)/float64(len(r.Conformance))*100)

	fmt.Println("\n--- 2. PERFORMANCE & RESOURCE ALLOCATION ---")
	fmt.Printf("%-30s | %-16s | %-16s | %-14s | %-12s | %-12s\n",
		"Benchmark Scenario", "Pure-Go Latency", "CGO-IBM Latency", "Speedup / Ratio", "Pure-Go B/op", "CGO-IBM B/op")
	fmt.Println(strings.Repeat("-", 110))

	for _, b := range r.Benchmarks {
		pgLat := b.PureGo.AvgLatency()
		cgLat := b.CgoIBM.AvgLatency()

		ratio := float64(cgLat) / float64(pgLat)
		speedup := fmt.Sprintf("%.2fx faster", ratio)
		if ratio < 1.0 {
			speedup = fmt.Sprintf("%.2fx slower", 1.0/ratio)
		}

		fmt.Printf("%-30s | %-16s | %-16s | %-14s | %-12d | %-12d\n",
			b.Name, pgLat.String(), cgLat.String(), speedup, b.PureGo.BytesPerOp(), b.CgoIBM.BytesPerOp())
	}
	fmt.Println("==========================================================================================================\n")
}

// SaveMarkdown writes a detailed report to a Markdown file.
func (r *ReportGenerator) SaveMarkdown(filePath string) error {
	var sb strings.Builder
	sb.WriteString("# IBM Db2 Driver Benchmark & Conformance Report\n\n")
	sb.WriteString(fmt.Sprintf("> Generated at: %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST")))
	sb.WriteString("Comparison between **`go-db2`** (Pure Go DRDA implementation) and **`go_ibm_db`** (IBM Official CGO / clidriver wrapper).\n\n")

	sb.WriteString("## 1. Semantic Conformance (Result Parity)\n\n")
	sb.WriteString("| Status | Test Scenario | `go-db2` (Pure Go) Output | `go_ibm_db` (CGO) Output |\n")
	sb.WriteString("| :---: | :--- | :--- | :--- |\n")

	for _, c := range r.Conformance {
		status := "✅ PASS"
		if !c.Passed {
			status = "❌ FAIL"
		}
		sb.WriteString(fmt.Sprintf("| %s | **%s** | `%s` | `%s` |\n",
			status, c.Name, sanitizeMD(c.PureGoVal), sanitizeMD(c.CgoVal)))
	}

	sb.WriteString("\n## 2. Performance & Resource Allocation\n\n")
	sb.WriteString("| Scenario | `go-db2` Avg Latency | `go_ibm_db` Avg Latency | Latency Ratio | `go-db2` Mem/Op | `go_ibm_db` Mem/Op | `go-db2` Allocs/Op | `go_ibm_db` Allocs/Op |\n")
	sb.WriteString("| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |\n")

	for _, b := range r.Benchmarks {
		pgLat := b.PureGo.AvgLatency()
		cgLat := b.CgoIBM.AvgLatency()
		ratio := float64(cgLat) / float64(pgLat)
		speedup := fmt.Sprintf("**%.2fx faster** 🚀", ratio)
		if ratio < 1.0 {
			speedup = fmt.Sprintf("%.2fx slower", 1.0/ratio)
		}

		sb.WriteString(fmt.Sprintf("| **%s** | %s | %s | %s | %d B | %d B | %d | %d |\n",
			b.Name, pgLat, cgLat, speedup,
			b.PureGo.BytesPerOp(), b.CgoIBM.BytesPerOp(),
			b.PureGo.AllocsPerOp(), b.CgoIBM.AllocsPerOp(),
		))
	}

	sb.WriteString("\n## 3. Key Observations\n\n")
	sb.WriteString("- **CGO Bridge Overhead**: `go_ibm_db` incurs a context switch overhead across the CGO boundary for every row fetch (`rows.Next()` / `rows.Scan()`).\n")
	sb.WriteString("- **Pure Go Memory Management**: `go-db2` uses stack buffers and pre-allocated slices, significantly lowering heap memory allocations.\n")
	sb.WriteString("- **Zero External C Dependencies**: `go-db2` compiles statically (`CGO_ENABLED=0`) across all operating systems and architectures.\n")

	return os.WriteFile(filePath, []byte(sb.String()), 0644)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}

func sanitizeMD(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	if len(s) > 40 {
		return s[:37] + "..."
	}
	return s
}
