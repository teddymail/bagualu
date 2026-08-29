package network

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type InterfaceTraffic struct {
	Name          string
	DownloadBytes int64
	UploadBytes   int64
}

func ReadInterfaceTraffic(ctx context.Context) (InterfaceTraffic, error) {
	if runtime.GOOS == "darwin" {
		return readDarwin(ctx)
	}
	return readLinux()
}

func readLinux() (InterfaceTraffic, error) {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return InterfaceTraffic{}, fmt.Errorf("read interface traffic: %w", err)
	}
	defer file.Close()
	var result InterfaceTraffic
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if parts[0] == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			continue
		}
		rx, err1 := strconv.ParseInt(fields[0], 10, 64)
		tx, err2 := strconv.ParseInt(fields[8], 10, 64)
		if err1 != nil || err2 != nil {
			return InterfaceTraffic{}, fmt.Errorf("parse interface traffic for %s", parts[0])
		}
		result.Name = strings.TrimSpace(parts[0])
		result.DownloadBytes += rx
		result.UploadBytes += tx
	}
	if err := scanner.Err(); err != nil {
		return InterfaceTraffic{}, fmt.Errorf("scan interface traffic: %w", err)
	}
	return result, nil
}

func readDarwin(ctx context.Context) (InterfaceTraffic, error) {
	name, err := defaultInterface(ctx)
	if err != nil {
		return InterfaceTraffic{}, err
	}
	output, err := exec.CommandContext(ctx, "/usr/sbin/netstat", "-ib").Output()
	if err != nil {
		return InterfaceTraffic{}, fmt.Errorf("read interface traffic: %w", err)
	}
	lines := strings.Split(string(output), "\n")
	header := -1
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "Name" {
			header = i
			break
		}
	}
	if header < 0 {
		return InterfaceTraffic{}, fmt.Errorf("interface traffic header not found")
	}
	columns := strings.Fields(lines[header])
	rxIndex, txIndex := -1, -1
	for i, column := range columns {
		if column == "Ibytes" {
			rxIndex = i
		}
		if column == "Obytes" {
			txIndex = i
		}
	}
	if rxIndex < 0 || txIndex < 0 {
		return InterfaceTraffic{}, fmt.Errorf("interface traffic byte columns not found")
	}
	for _, line := range lines[header+1:] {
		fields := strings.Fields(line)
		if len(fields) <= rxIndex || len(fields) <= txIndex || fields[0] != name {
			continue
		}
		rx, err1 := strconv.ParseInt(fields[rxIndex], 10, 64)
		tx, err2 := strconv.ParseInt(fields[txIndex], 10, 64)
		if err1 != nil || err2 != nil {
			return InterfaceTraffic{}, fmt.Errorf("parse interface traffic for %s", name)
		}
		return InterfaceTraffic{Name: name, DownloadBytes: rx, UploadBytes: tx}, nil
	}
	return InterfaceTraffic{Name: name}, nil
}

func defaultInterface(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "/sbin/route", "-n", "get", "default").Output()
	if err != nil {
		return "", fmt.Errorf("find default interface: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "interface:" {
			return fields[1], nil
		}
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("list interfaces: %w", err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 {
			return iface.Name, nil
		}
	}
	return "", fmt.Errorf("default interface not found")
}
