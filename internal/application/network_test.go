package application

import (
	"context"
	"testing"
	"time"

	"github.com/teddymail/bagualu/internal/domain"
)

func TestBandwidthLoadGuardReportsBusyAndUnknown(t *testing.T) {
	values := []TrafficSample{{DownloadBytes: 0, UploadBytes: 0}, {DownloadBytes: 200, UploadBytes: 0}}
	index := 0
	guard := BandwidthLoadGuard{Reader: func(context.Context) (TrafficSample, error) { value := values[index]; index++; return value, nil }, DownloadCapacityBPS: 20, UploadCapacityBPS: 20, Threshold: .1, SampleWindow: time.Nanosecond}
	result := guard.Check(context.Background())
	if result.Status != LoadBusy || result.DownloadBps <= 0 {
		t.Fatalf("unexpected load result: %+v", result)
	}
	unknown := (BandwidthLoadGuard{}).Check(context.Background())
	if unknown.Status != LoadUnknown {
		t.Fatalf("expected unknown load, got %+v", unknown)
	}
}

func TestBandwidthLoadGuardCheckDuringSubtractsTaskTraffic(t *testing.T) {
	guard := &BandwidthLoadGuard{state: &bandwidthLoadState{
		start:   TrafficSample{DownloadBytes: 100, UploadBytes: 20},
		started: time.Now().Add(-time.Second), capacity: 10,
	}}
	guard.Reader = func(context.Context) (TrafficSample, error) {
		return TrafficSample{DownloadBytes: 130, UploadBytes: 20}, nil
	}
	result := guard.CheckDuring(context.Background(), domain.MeasurementOutcome{DownloadBytes: 10})
	if result.Status != LoadContended {
		t.Fatalf("expected background traffic to be contended, got %+v", result)
	}
}
