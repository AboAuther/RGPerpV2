package main

import (
	"testing"
	"time"
)

func TestParseStages(t *testing.T) {
	got, err := parseStages("1, 5,10")
	if err != nil {
		t.Fatalf("parseStages returned error: %v", err)
	}
	want := []int{1, 5, 10}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected stage at %d: got=%d want=%d", i, got[i], want[i])
		}
	}
}

func TestParseStagesRejectsInvalidValue(t *testing.T) {
	if _, err := parseStages("1,0,3"); err == nil {
		t.Fatalf("expected parseStages to reject zero concurrency")
	}
}

func TestPercentiles(t *testing.T) {
	values := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}
	p50, p95, p99, max := percentiles(values)
	if p50 != 30*time.Millisecond {
		t.Fatalf("unexpected p50: %s", p50)
	}
	if p95 != 40*time.Millisecond {
		t.Fatalf("unexpected p95: %s", p95)
	}
	if p99 != 40*time.Millisecond {
		t.Fatalf("unexpected p99: %s", p99)
	}
	if max != 50*time.Millisecond {
		t.Fatalf("unexpected max: %s", max)
	}
}

func TestSummarizeStageMarksSLAFailure(t *testing.T) {
	cfg := config{
		warmupDuration:   time.Second,
		stageDuration:    2 * time.Second,
		successThreshold: 0.99,
		orderP95SLA:      20 * time.Millisecond,
		orderP99SLA:      50 * time.Millisecond,
		cancelP99SLA:     30 * time.Millisecond,
		targetTPS:        5000,
	}
	result := workerResult{
		Cycles: 2,
		Samples: []sample{
			{Operation: "limit_create", OK: true, Duration: 60 * time.Millisecond},
			{Operation: "limit_cancel", OK: true, Duration: 20 * time.Millisecond},
			{Operation: "limit_create", OK: true, Duration: 70 * time.Millisecond},
			{Operation: "limit_cancel", OK: true, Duration: 25 * time.Millisecond},
		},
	}

	stage := summarizeStage(cfg, "limit_cycle", 2, result, cfg.stageDuration)
	if stage.Verdict.SLAPass {
		t.Fatalf("expected SLA failure, got pass")
	}
	if stage.Operations["limit_create"].P99Ms <= 50 {
		t.Fatalf("expected limit_create p99 to exceed SLA, got %.2fms", stage.Operations["limit_create"].P99Ms)
	}
}
