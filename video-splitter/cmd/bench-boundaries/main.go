// Command bench-boundaries chuyển JSON candidate (từ python_worker) thành danh sách
// clip cuối cùng qua chính hàm CalculateBoundaries của app, để đo số clip thực tế
// cho từng chế độ thay vì chỉ đếm candidate thô.
//
// Dùng: bench-boundaries <mode> <candidates.json> <totalDuration> [sourcePath]
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"video-splitter/internal/boundary"
	"video-splitter/internal/project"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "dùng: bench-boundaries <mode> <candidates.json> <totalDuration> [sourcePath]")
		os.Exit(2)
	}
	mode := os.Args[1]
	jsonPath := os.Args[2]
	totalDuration, _ := strconv.ParseFloat(os.Args[3], 64)
	sourcePath := ""
	if len(os.Args) >= 5 {
		sourcePath = os.Args[4]
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lỗi đọc %s: %v\n", jsonPath, err)
		os.Exit(1)
	}

	var result boundary.AnalyzerResult
	if err := json.Unmarshal(raw, &result); err != nil {
		fmt.Fprintf(os.Stderr, "lỗi parse json: %v\n", err)
		os.Exit(1)
	}

	cfg := project.DefaultConfig()
	cfg.Mode = mode

	clips := boundary.CalculateBoundaries(result.Candidates, cfg, totalDuration, sourcePath)

	// Thống kê thời lượng clip
	var minD, maxD, sumD float64
	minD = 1e9
	tierCount := map[string]int{}
	for _, c := range clips {
		d := c.Duration
		if d < minD {
			minD = d
		}
		if d > maxD {
			maxD = d
		}
		sumD += d
		tierCount[c.Tier]++
	}
	avgD := 0.0
	if len(clips) > 0 {
		avgD = sumD / float64(len(clips))
	}

	// Nếu có biến môi trường BENCH_CUTS, chỉ in danh sách điểm cắt (StartTime của
	// từng clip trừ clip đầu) mỗi dòng một số — để script ngoài so khớp tham chiếu.
	if os.Getenv("BENCH_CUTS") != "" {
		for i, c := range clips {
			if i == 0 {
				continue // clip đầu bắt đầu tại 0, không phải điểm cắt
			}
			fmt.Printf("%.3f\n", c.StartTime)
		}
		return
	}

	fmt.Printf("mode=%s candidates=%d clips=%d\n", mode, len(result.Candidates), len(clips))
	fmt.Printf("  duration min=%.1fs avg=%.1fs max=%.1fs\n", minD, avgD, maxD)
	fmt.Printf("  tiers: auto=%d review=%d\n", tierCount[project.TierAuto], tierCount[project.TierReview])
}
