package service

import (
	"testing"
	"time"

	"synapseqa/backend/internal/model"
)

func TestPerfEventHubReplaysMoreThanChannelCapacity(t *testing.T) {
	hub := NewPerfEventHub()
	for sequence := int64(1); sequence <= 40; sequence++ {
		if !hub.Publish(1, model.PerfSampleEvent{Sequence: sequence, WindowMs: 1000}) {
			t.Fatalf("publish sequence %d failed", sequence)
		}
	}
	stream, cancel := hub.Subscribe(1, 0)
	defer cancel()
	for sequence := int64(1); sequence <= 40; sequence++ {
		message := <-stream
		if message.Event != "perf.sample" || message.ID != sequence {
			t.Fatalf("unexpected replay message: %+v", message)
		}
	}
}

func TestPerfEventHubSendsResetWhenReplayGapExceedsRing(t *testing.T) {
	hub := NewPerfEventHub()
	for sequence := int64(1); sequence <= 300; sequence++ {
		hub.Publish(1, model.PerfSampleEvent{Sequence: sequence, WindowMs: 1000})
	}
	stream, cancel := hub.Subscribe(1, 0)
	defer cancel()
	message := <-stream
	if message.Event != "perf.reset" {
		t.Fatalf("expected reset for replay gap, got %+v", message)
	}
	series, ok := message.Data.(model.PerfSeries)
	if !ok || series.LastSequence != 300 || len(series.Points) == 0 {
		t.Fatalf("reset did not contain full series: %+v", message.Data)
	}
}

func TestPerfEventHubExpiresTerminalRuns(t *testing.T) {
	hub := NewPerfEventHub()
	hub.Publish(1, model.PerfSampleEvent{Sequence: 1, WindowMs: 1000})
	hub.MarkTerminal(1, model.PerfRunCompleted, model.PerfSeries{Version: 1, LastSequence: 1, Points: []model.PerfSampleEvent{}})
	hub.CleanupExpired(time.Now().Add(perfHubTerminalTTL + time.Second))
	if _, ok := hub.Snapshot(1); ok {
		t.Fatal("terminal hub run should expire")
	}
}
