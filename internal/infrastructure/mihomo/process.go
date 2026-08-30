package mihomo

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

type ProcessManager struct {
	mu             sync.Mutex
	command        *exec.Cmd
	done           chan struct{}
	binary, config string
	logHandler     func(stream, message string)
	stopRequested  bool
	restartCount   int
	failed         bool
}

func NewProcessManager(binary, config string) *ProcessManager {
	return &ProcessManager{binary: binary, config: config}
}

func (m *ProcessManager) SetLogHandler(handler func(stream, message string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logHandler = handler
}

func (m *ProcessManager) Start(ctx context.Context, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.command != nil && m.command.Process != nil {
		if m.command.ProcessState == nil {
			return nil
		}
		m.command = nil
	}
	m.stopRequested = false
	m.failed = false
	if _, err := os.Stat(m.binary); err != nil {
		return fmt.Errorf("core_unavailable: %w", err)
	}
	m.command = exec.CommandContext(ctx, m.binary, append([]string{"-f", m.config}, args...)...)
	configureCommand(m.command)
	stdout, err := m.command.StdoutPipe()
	if err != nil {
		m.command = nil
		return fmt.Errorf("core_unavailable: capture stdout: %w", err)
	}
	stderr, err := m.command.StderrPipe()
	if err != nil {
		m.command = nil
		return fmt.Errorf("core_unavailable: capture stderr: %w", err)
	}
	if err := m.command.Start(); err != nil {
		m.command = nil
		return fmt.Errorf("core_unavailable: %w", err)
	}
	cmd := m.command
	m.done = make(chan struct{})
	done := m.done
	go func() {
		var output sync.WaitGroup
		output.Add(2)
		go func() {
			defer output.Done()
			m.captureOutput(stdout, "stdout", os.Stdout)
		}()
		go func() {
			defer output.Done()
			m.captureOutput(stderr, "stderr", os.Stderr)
		}()
		err := cmd.Wait()
		output.Wait()
		if err != nil {
			m.emitLog("process", fmt.Sprintf("Mihomo 进程退出: %v", err))
		}
		close(done)
		m.mu.Lock()
		if m.command == cmd {
			m.command = nil
			m.done = nil
		}
		m.mu.Unlock()
	}()
	return nil
}

func (m *ProcessManager) captureOutput(reader interface{ Read([]byte) (int, error) }, stream string, mirror *os.File) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintln(mirror, line)
		m.emitLog(stream, line)
	}
	if err := scanner.Err(); err != nil {
		m.emitLog(stream, fmt.Sprintf("读取 Mihomo %s 日志失败: %v", stream, err))
	}
}

func (m *ProcessManager) emitLog(stream, message string) {
	m.mu.Lock()
	handler := m.logHandler
	m.mu.Unlock()
	if handler != nil {
		handler(stream, message)
	}
}

func (m *ProcessManager) Monitor(ctx context.Context, maxRestarts int, backoff time.Duration, args ...string) {
	if maxRestarts < 0 {
		maxRestarts = 0
	}
	if backoff <= 0 {
		backoff = time.Second
	}
	go func() {
		for {
			m.mu.Lock()
			done := m.done
			m.mu.Unlock()
			if done == nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-done:
			}
			m.mu.Lock()
			stopped := m.stopRequested
			if !stopped && m.restartCount >= maxRestarts {
				m.failed = true
			}
			shouldRestart := !stopped && !m.failed
			if shouldRestart {
				m.restartCount++
			}
			m.mu.Unlock()
			if !shouldRestart {
				return
			}
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			m.mu.Lock()
			stoppedAfterBackoff := m.stopRequested
			m.mu.Unlock()
			if stoppedAfterBackoff {
				return
			}
			if err := m.Start(ctx, args...); err != nil {
				m.mu.Lock()
				m.failed = true
				m.mu.Unlock()
				return
			}
		}
	}()
}

func (m *ProcessManager) Stop() error {
	m.mu.Lock()
	m.stopRequested = true
	if m.command == nil || m.command.Process == nil {
		m.mu.Unlock()
		return nil
	}
	cmd, done := m.command, m.done
	err := killCommand(cmd)
	m.mu.Unlock()
	<-done
	m.mu.Lock()
	if m.command == cmd {
		m.command = nil
		m.done = nil
	}
	m.mu.Unlock()
	return err
}

func (m *ProcessManager) RestartCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restartCount
}

func (m *ProcessManager) Failed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failed
}

func (m *ProcessManager) PID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.command == nil || m.command.Process == nil || m.command.ProcessState != nil {
		return 0
	}
	return m.command.Process.Pid
}
