package mihomo

import (
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
	stopRequested  bool
	restartCount   int
	failed         bool
}

func NewProcessManager(binary, config string) *ProcessManager {
	return &ProcessManager{binary: binary, config: config}
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
	m.command.Stdout, m.command.Stderr = os.Stdout, os.Stderr
	if err := m.command.Start(); err != nil {
		m.command = nil
		return fmt.Errorf("core_unavailable: %w", err)
	}
	cmd := m.command
	m.done = make(chan struct{})
	done := m.done
	go func() {
		_ = cmd.Wait()
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
	err := cmd.Process.Kill()
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
