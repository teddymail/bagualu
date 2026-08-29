package network

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type PingResult struct {
	LatencyMS float64
	Probes    int
	Received  int
}

var pingTimePattern = regexp.MustCompile(`time[=<]([0-9]+(?:\.[0-9]+)?)\s*ms`)

// Ping uses the router's default network namespace and never uses Mihomo.
func Ping(ctx context.Context, address string, probes int, timeout time.Duration) (PingResult, error) {
	if strings.TrimSpace(address) == "" {
		return PingResult{}, fmt.Errorf("ping target is empty")
	}
	if probes < 1 {
		probes = 1
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	timeoutArg := strconv.Itoa(int(timeout.Seconds()))
	if runtime.GOOS == "darwin" {
		timeoutArg = strconv.Itoa(int(timeout.Milliseconds()))
	}
	args := []string{"-c", strconv.Itoa(probes), "-W", timeoutArg, address}
	output, err := exec.CommandContext(ctx, "ping", args...).CombinedOutput()
	result := ParsePingOutput(string(output), probes)
	if err != nil || result.Received == 0 {
		if err == nil {
			err = fmt.Errorf("no ping replies")
		}
		return result, err
	}
	return result, nil
}

func ParsePingOutput(output string, probes int) PingResult {
	if probes < 1 {
		probes = 1
	}
	values := make([]float64, 0, probes)
	for _, match := range pingTimePattern.FindAllStringSubmatch(output, -1) {
		value, err := strconv.ParseFloat(match[1], 64)
		if err == nil {
			values = append(values, value)
		}
	}
	var latency float64
	for _, value := range values {
		latency += value
	}
	if len(values) > 0 {
		latency /= float64(len(values))
	}
	return PingResult{LatencyMS: latency, Probes: probes, Received: len(values)}
}
