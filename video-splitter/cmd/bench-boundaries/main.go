package main

import (
	"encoding/json"
	"fmt"
	"os"

	"video-splitter/internal/boundary"
	"video-splitter/internal/project"
)

type analyzerResult struct {
	Status     string               `json:"status"`
	Candidates []boundary.Candidate `json:"candidates"`
}

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: bench-boundaries <mode> <candidate.json> <source> <output.json>")
		os.Exit(2)
	}

	data, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	var result analyzerResult
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}

	cfg := project.DefaultConfig()
	cfg.Mode = os.Args[1]
	clips := boundary.CalculateBoundaries(result.Candidates, cfg, 2069.223039, os.Args[3])

	out, err := json.MarshalIndent(clips, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Args[4], out, 0644); err != nil {
		panic(err)
	}
	fmt.Printf("mode=%s candidates=%d clips=%d\n", cfg.Mode, len(result.Candidates), len(clips))
}
