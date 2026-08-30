package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/teddymail/bagualu/internal/application"
	"github.com/teddymail/bagualu/internal/domain"
	"github.com/teddymail/bagualu/internal/infrastructure/logging"
	"github.com/teddymail/bagualu/internal/infrastructure/mihomo"
	"github.com/teddymail/bagualu/internal/infrastructure/network"
	"github.com/teddymail/bagualu/internal/infrastructure/persistence"
	httptransport "github.com/teddymail/bagualu/internal/transport/http"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:18787", "management listen address")
	control := flag.String("mihomo-control", "http://127.0.0.1:19090", "managed Mihomo control API")
	token := flag.String("mihomo-token", "", "managed Mihomo API token")
	binary := flag.String("mihomo-binary", "", "Bagualu-managed Mihomo executable (auto-detected when empty)")
	proxyPort := flag.Int("mihomo-proxy-port", 17890, "Bagualu-managed local mixed proxy port")
	selector := flag.String("mihomo-selector", "Bagualu-Test", "private Mihomo selector group")
	mihomoRepository := flag.String("mihomo-repository", "MetaCubeX/mihomo", "official Mihomo release repository")
	mihomoVersion := flag.String("mihomo-version", "latest", "Mihomo release tag or latest")
	wanDownload := flag.Float64("wan-download-bps", 0, "configured WAN download capacity in bytes per second")
	wanUpload := flag.Float64("wan-upload-bps", 0, "configured WAN upload capacity in bytes per second")
	loadThreshold := flag.Float64("load-threshold", 0.1, "background load protection ratio")
	queueLimit := flag.Int("test-queue-limit", 32, "maximum pending test tasks")
	dbPath := flag.String("db", "/etc/bagualu/bagualu.db", "SQLite database path")
	adminPassword := flag.String("admin-password", "admin", "initial admin password (used once if none is set)")
	resetPasswordStdin := flag.Bool("reset-password-stdin", false, "reset the Bagualu admin password from stdin and exit")
	configFile := flag.String("config", "", "OpenWrt UCI-style configuration file")
	statusFile := flag.String("status-file", "", "runtime status JSON file for LuCI")
	flag.Parse()
	runtimeLogs := logging.NewBuffer(1000)
	log.SetOutput(io.MultiWriter(os.Stderr, runtimeLogs.Writer("bagualu", "stderr")))
	runCtx, stopRun := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopRun()
	uciValues := map[string]string{}
	if *configFile != "" {
		values, err := readUCIConfig(*configFile)
		if err != nil {
			log.Fatalf("read config: %v", err)
		}
		uciValues = values
		if value := values["listen"]; value != "" {
			port := values["port"]
			if port == "" {
				port = "18787"
			}
			*listen = net.JoinHostPort(value, port)
		}
		if value := values["data_dir"]; value != "" {
			*dbPath = filepath.Join(value, "bagualu.db")
		}
		if value := values["mihomo_control_port"]; value != "" {
			*control = "http://127.0.0.1:" + value
		}
		if value := values["mihomo_proxy_port"]; value != "" {
			if parsed, err := strconv.Atoi(value); err == nil {
				*proxyPort = parsed
			}
		}
		if value := values["mihomo_binary"]; value != "" {
			*binary = value
		}
		if value := values["mihomo_token"]; value != "" {
			*token = value
		}
		if value := values["admin_password"]; value != "" {
			*adminPassword = value
		}
		if value := values["mihomo_selector"]; value != "" {
			*selector = value
		}
		if value := values["mihomo_repository"]; value != "" {
			*mihomoRepository = value
		}
		if value := values["mihomo_version"]; value != "" {
			*mihomoVersion = value
		}
		*wanDownload, *wanUpload, *loadThreshold = parseBandwidthConfig(*wanDownload, *wanUpload, *loadThreshold, values)
		if value := values["test_queue_limit"]; value != "" {
			if parsed, parseErr := strconv.Atoi(value); parseErr == nil {
				*queueLimit = parsed
			}
		}
		if value := values["status_file"]; value != "" {
			*statusFile = value
		}
	}
	if *binary == "" {
		if found, err := exec.LookPath("mihomo"); err == nil {
			*binary = found
		} else {
			*binary = "/usr/bin/mihomo"
		}

	}

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0700); err != nil {
		log.Fatalf("create data directory: %v", err)
	}
	if *resetPasswordStdin {
		passwordBytes, err := io.ReadAll(io.LimitReader(os.Stdin, 4097))
		if err != nil {
			log.Fatalf("read reset password: %v", err)
		}
		password := strings.TrimRight(string(passwordBytes), "\r\n")
		if len(password) < 8 {
			log.Fatal("reset password must be at least 8 characters")
		}
		resetStore, err := persistence.Open(*dbPath)
		if err != nil {
			log.Fatalf("open database for password reset: %v", err)
		}
		if err := httptransport.ResetAdminPassword(context.Background(), resetStore, password); err != nil {
			resetStore.Close()
			log.Fatalf("reset Bagualu admin password: %v", err)
		}
		if err := resetStore.Close(); err != nil {
			log.Fatalf("close database after password reset: %v", err)
		}
		return
	}
	store, err := persistence.Open(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()
	if err := store.JobRepo().DeleteInactive(context.Background()); err != nil {
		log.Printf("cleanup inactive jobs: %v", err)
	}
	if err := store.JobRepo().CancelOrphanedActive(context.Background()); err != nil {
		log.Printf("cancel orphaned jobs: %v", err)
	}

	core := mihomo.NewClient(*control, *token)
	controlURL, err := url.Parse(*control)
	if err != nil || controlURL.Host == "" {
		log.Fatalf("invalid Mihomo control URL: %q", *control)
	}
	controlAddress := controlURL.Host
	configDir := filepath.Dir(*dbPath)
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("find home directory: %v", err)
		}
		configDir = filepath.Join(home, ".config", "mihomo")
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Fatalf("create Mihomo config directory: %v", err)
	}
	mihomoConfigPath := filepath.Join(configDir, "bagualu.yaml")
	process := mihomo.NewProcessManager(*binary, mihomoConfigPath)
	process.SetLogHandler(func(stream, message string) {
		runtimeLogs.Add("mihomo", stream, logging.DetectLevel(message), message)
	})
	installer := mihomo.NewInstaller(*binary, *mihomoRepository, *mihomoVersion)
	startupError := ""
	var startupErrorMu sync.RWMutex
	setStartupError := func(value string) {
		startupErrorMu.Lock()
		startupError = value
		startupErrorMu.Unlock()
	}
	getStartupError := func() string {
		startupErrorMu.RLock()
		defer startupErrorMu.RUnlock()
		return startupError
	}
	activeConfigDigest := ""
	initialNodes, _ := store.NodeRepo().FindAll(context.Background(), domain.NodeFilter{Status: domain.NodeActive})
	if config, digest, configErr := mihomo.Config(initialNodes, controlAddress, *token, *proxyPort); configErr == nil {
		activeConfigDigest = digest
		if err := os.MkdirAll(filepath.Dir(*dbPath), 0700); err != nil {
			setStartupError(fmt.Sprintf("%s: %v", domain.ErrCodeCoreUnavailable, err))
		} else if err := os.WriteFile(mihomoConfigPath, config, 0600); err != nil {
			setStartupError(fmt.Sprintf("%s: %v", domain.ErrCodeCoreUnavailable, err))
		} else if err := process.Start(context.Background(), "-ext-ctl", controlAddress, "-secret", *token); err != nil {
			setStartupError(err.Error())
			log.Printf("mihomo unavailable: %v", err)
		} else {
			process.Monitor(runCtx, 3, 2*time.Second, "-ext-ctl", controlAddress, "-secret", *token)
		}
	} else {
		setStartupError(fmt.Sprintf("%s: %v", domain.ErrCodeCoreUnavailable, configErr))
	}
	defer func() {
		_ = process.Stop()
		if *statusFile != "" {
			_ = writeRuntimeStatus(*statusFile, map[string]any{"service_state": "stopped", "bagualu_pid": os.Getpid(), "mihomo_pid": 0, "updated_at": time.Now().UTC()})
		}
	}()
	status := func() (result domain.CoreStatus) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		result, _ = core.Status(ctx)
		result.PID = process.PID()
		result.Proxy = "127.0.0.1:" + fmt.Sprint(*proxyPort)
		result.AutoRestarts = process.RestartCount()
		if getStartupError() != "" {
			result.Available = false
			result.State = "failed"
			result.ErrorCode = domain.ErrCodeCoreUnavailable
		} else if process.Failed() {
			result.Available = false
			result.State = "failed"
			result.ErrorCode = domain.ErrCodeCoreUnavailable
		} else if result.Available {
			result.State = "running"
		} else if result.PID > 0 {
			result.State = "degraded"
		} else {
			result.State = "stopped"
		}
		return
	}
	coreInstallStatus := func(ctx context.Context) domain.CoreInstallStatus {
		result := installer.Status(ctx)
		if err := getStartupError(); err != "" && result.Error == "" {
			result.Error = err
		}
		return result
	}
	startCore := func() error {
		if err := process.Start(context.Background(), "-ext-ctl", controlAddress, "-secret", *token); err != nil {
			setStartupError(err.Error())
			return err
		}
		setStartupError("")
		process.Monitor(runCtx, 3, 2*time.Second, "-ext-ctl", controlAddress, "-secret", *token)
		return nil
	}
	coreInstall := func(ctx context.Context) (domain.CoreInstallResult, error) {
		if err := process.Stop(); err != nil {
			return domain.CoreInstallResult{}, fmt.Errorf("stop Mihomo before install: %w", err)
		}
		result, err := installer.InstallLatest(ctx)
		if err != nil {
			setStartupError(fmt.Sprintf("%s: %v", domain.ErrCodeCoreUnavailable, err))
			_ = startCore()
			return domain.CoreInstallResult{}, err
		}
		if err := startCore(); err != nil {
			return result, err
		}
		return result, nil
	}
	coreInstallUpload := func(ctx context.Context, source io.Reader, name string) (domain.CoreInstallResult, error) {
		archive, err := io.ReadAll(io.LimitReader(source, 128<<20+1))
		if err != nil {
			return domain.CoreInstallResult{}, err
		}
		if len(archive) == 0 || len(archive) > 128<<20 {
			return domain.CoreInstallResult{}, fmt.Errorf("uploaded Mihomo file is empty or too large")
		}
		if err := mihomo.ValidateArchive(archive); err != nil {
			return domain.CoreInstallResult{}, err
		}
		if err := process.Stop(); err != nil {
			return domain.CoreInstallResult{}, fmt.Errorf("stop Mihomo before install: %w", err)
		}
		result, err := installer.InstallFile(ctx, bytes.NewReader(archive), name)
		if err != nil {
			setStartupError(fmt.Sprintf("%s: %v", domain.ErrCodeCoreUnavailable, err))
			_ = startCore()
			return domain.CoreInstallResult{}, err
		}
		if err := startCore(); err != nil {
			return result, err
		}
		return result, nil
	}
	managementListener, err := net.Listen("tcp", *listen)
	if err != nil {
		if *statusFile != "" {
			_ = writeRuntimeStatus(*statusFile, map[string]any{
				"service_state": "failed", "expected_state": "enabled", "bagualu_pid": os.Getpid(),
				"management": *listen, "control": controlAddress, "proxy": "127.0.0.1:" + strconv.Itoa(*proxyPort),
				"error_code": "management_port_unavailable", "error": err.Error(), "updated_at": time.Now().UTC(),
			})
		}
		log.Fatalf("listen management address %s: %v", *listen, err)
	}
	defer managementListener.Close()
	if *statusFile != "" {
		writeStatus := func() {
			coreStatus := status()
			payload := map[string]any{
				"service_state": coreStatus.State, "expected_state": map[bool]string{true: "enabled", false: "disabled"}[uciValues["enabled"] != "0"],
				"bagualu_pid": os.Getpid(), "mihomo_pid": coreStatus.PID, "mihomo": coreStatus,
				"management": *listen, "control": controlAddress, "proxy": coreStatus.Proxy,
				"error_code": coreStatus.ErrorCode, "updated_at": time.Now().UTC(),
			}
			if err := writeRuntimeStatus(*statusFile, payload); err != nil {
				log.Printf("write runtime status: %v", err)
			}
		}
		writeStatus()
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					writeStatus()
				}
			}
		}()
	}

	queue := application.NewTestQueue(*queueLimit)
	refresher := application.NewUpstreamRefresher(store.UpstreamRepo(), store.NodeRepo(), store.MeasurementRepo(), nil)
	refreshRunner := application.NewUpstreamRefreshRunner(store.JobRepo(), refresher)
	scoreService := application.NewScoreService(store.NodeRepo(), store.MeasurementRepo(), store.ScoreSnapshotRepo(), domain.DefaultScorePolicy())
	if raw, err := store.SettingsRepo().Get(context.Background(), "score_policy"); err == nil && raw != "" {
		var savedPolicy domain.ScorePolicy
		if json.Unmarshal([]byte(raw), &savedPolicy) == nil {
			_ = scoreService.SetPolicy(savedPolicy)
		}
	}
	scoreRunner := application.NewScoreRunner(store.JobRepo(), scoreService)
	geoService := application.NewGeoService(store.NodeRepo(), application.NewHTTPGeoLookup("", nil))
	tester, testerErr := mihomo.NewTester(core, *proxyPort, *selector, "bagualu-default", process.PID)
	if tester != nil {
		tester.SetConfigDigest(activeConfigDigest)
	}
	endpointCircuit := application.NewEndpointCircuit(2, 5*time.Minute)
	trafficReader := func(ctx context.Context) (application.TrafficSample, error) {
		traffic, err := network.ReadInterfaceTraffic(ctx)
		return application.TrafficSample{DownloadBytes: traffic.DownloadBytes, UploadBytes: traffic.UploadBytes}, err
	}
	loadGuard := application.NewBandwidthLoadGuard(trafficReader, func() (float64, float64, float64) {
		return *wanDownload, *wanUpload, *loadThreshold
	})
	orchestrator := application.NewOrchestrator(queue, &application.ICMPBaseline{}, loadGuard,
		func(outcome domain.MeasurementOutcome) {
			now := time.Now().UTC()
			if node, findErr := store.NodeRepo().FindByID(context.Background(), outcome.NodeID); findErr == nil {
				opened := endpointCircuit.Record(*node, outcome)
				if opened || outcome.Success && outcome.Kind == domain.TestConnectivity && node.Status == domain.NodeEndpointUnreachable {
					nodes, _ := store.NodeRepo().FindAll(context.Background(), domain.NodeFilter{})
					for i := range nodes {
						if nodes[i].EndpointIP != node.EndpointIP || nodes[i].Status == domain.NodeDisabled || nodes[i].Status == domain.NodeExpired {
							continue
						}
						if opened {
							_ = store.NodeRepo().UpdateStatus(context.Background(), nodes[i].ID, domain.NodeEndpointUnreachable)
						} else if nodes[i].Status == domain.NodeEndpointUnreachable {
							_ = store.NodeRepo().UpdateStatus(context.Background(), nodes[i].ID, domain.NodeActive)
						}
					}
				}
			}
			if outcome.Success && outcome.Evidence.ExitIP != "" {
				geoCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				if err := geoService.UpdateNode(geoCtx, outcome.NodeID, outcome.Evidence.ExitIP); err != nil {
					log.Printf("update node geography: %v", err)
				}
				cancel()
			}
			if node, findErr := store.NodeRepo().FindByID(context.Background(), outcome.NodeID); findErr == nil {
				if outcome.Infrastructure {
					// Infrastructure conditions never change node health.
				} else if outcome.Success && outcome.Kind == domain.TestConnectivity && node.Status != domain.NodeDisabled && node.Status != domain.NodeExpired {
					_ = store.NodeRepo().UpdateStatus(context.Background(), node.ID, domain.NodeActive)
				} else if outcome.Kind == domain.TestConnectivity {
					_ = store.NodeRepo().UpdateStatus(context.Background(), node.ID, domain.NodeUnreachable)
				}
			}
			measurement := &domain.Measurement{ID: outcome.JobID, NodeID: outcome.NodeID, Kind: string(outcome.Kind),
				Success: outcome.Success, ErrorCode: outcome.ErrorCode, ErrorDetail: outcome.ErrorDetail, FailureStage: outcome.FailureStage,
				LatencyMS: outcome.LatencyMS, FirstByteMS: outcome.FirstByteMS, SpeedBytesPerSec: outcome.SpeedBytesPerSec,
				EffectiveDownloadDurationMS: outcome.EffectiveDownloadDurationMS, Bytes: outcome.DownloadBytes,
				ProxyProtocol: outcome.ProxyProtocol, TestURL: outcome.TestURL, ExitIP: outcome.ExitIP,
				BaselineTarget: outcome.BaselineTarget, SpeedSource: outcome.SpeedSource,
				LoadStatus: outcome.LoadStatus, BackgroundUploadBPS: outcome.BackgroundUploadBPS,
				BackgroundDownloadBPS: outcome.BackgroundDownloadBPS,
				UploadBytes:           outcome.UploadBytes,
				WANDownloadBefore:     outcome.WANDownloadBefore, WANDownloadAfter: outcome.WANDownloadAfter,
				WANUploadBefore: outcome.WANUploadBefore, WANUploadAfter: outcome.WANUploadAfter,
				WANDownloadCapacityBPS: outcome.WANDownloadCapacityBPS, WANUploadCapacityBPS: outcome.WANUploadCapacityBPS,
				LoadThreshold: outcome.LoadThreshold, LoadSampleDurationMS: outcome.LoadSampleDurationMS,
				CoreEvidence: outcome.Evidence, Infrastructure: outcome.Infrastructure, CreatedAt: now}
			if err := store.MeasurementRepo().Save(context.Background(), measurement); err != nil {
				log.Printf("save measurement: %v", err)
			} else if !outcome.Infrastructure {
				if _, err := scoreService.Recalculate(context.Background(), outcome.NodeID); err != nil {
					log.Printf("recalculate score: %v", err)
				}
			}
			status := domain.JobSucceeded
			switch {
			case outcome.Status == "cancelled":
				status = domain.JobCancelled
			case outcome.Status == "paused" || outcome.Status == "scheduled":
				if outcome.Infrastructure {
					status = domain.JobDone
				} else {
					status = domain.JobScheduled
				}
			case !outcome.Success:
				status = domain.JobFailed
			}
			detail := outcome.ErrorDetail
			if detail == "" {
				detail = outcome.ErrorCode
			}
			_ = store.JobRepo().UpdateStatusDetail(context.Background(), outcome.JobID, status, 100,
				outcome.ErrorCode, detail, outcome.FailureStage)
		})
	queueDone := make(chan struct{})
	go func() {
		defer close(queueDone)
		queue.Run(runCtx)
	}()
	reloadCoreConfig := func(ctx context.Context) error {
		nodes, err := store.NodeRepo().FindAll(ctx, domain.NodeFilter{Status: domain.NodeActive})
		if err != nil {
			return err
		}
		config, digest, err := mihomo.Config(nodes, controlAddress, *token, *proxyPort)
		if err != nil {
			return err
		}
		if digest == activeConfigDigest {
			return nil
		}
		if err := os.WriteFile(mihomoConfigPath, config, 0600); err != nil {
			return err
		}
		activeConfigDigest = digest
		if tester != nil {
			tester.SetConfigDigest(activeConfigDigest)
		}
		return core.LoadConfig(ctx, mihomoConfigPath)
	}
	var testSubmitWithSource func(context.Context, string, domain.TestKind, string) (string, error)
	testSubmitWithSource = func(ctx context.Context, nodeID string, kind domain.TestKind, speedSource string) (string, error) {
		if getStartupError() != "" {
			return "", fmt.Errorf("%s", domain.ErrCodeCoreUnavailable)
		}
		if current := status(); !current.Available {
			if current.ErrorCode != "" {
				return "", fmt.Errorf("%s", current.ErrorCode)
			}
			return "", fmt.Errorf("%s", domain.ErrCodeCoreUnavailable)
		}
		node, err := store.NodeRepo().FindByID(ctx, nodeID)
		if err != nil {
			return "", err
		}
		if err := endpointCircuit.Before(*node); err != nil {
			return "", err
		}
		jobID := uuid.NewString()
		now := time.Now().UTC()
		if err := store.JobRepo().Save(ctx, &domain.Job{ID: jobID, Kind: "test_" + string(kind), EntityID: nodeID, Status: domain.JobPending, CreatedAt: now, UpdatedAt: now}); err != nil {
			return "", err
		}
		policy := loadTestPolicy(ctx, store, uciValues)
		target := policy.ConnectivityURL
		maxBytes := int64(policy.ThroughputBytes)
		if kind == domain.TestThroughput {
			target = speedSource
			if target == "" {
				target = selectSpeedSource(policy, time.Now())
			}
			if target == "" {
				err := fmt.Errorf("%s: no healthy download source is configured", domain.ErrCodeSpeedSourceUnavailable)
				_ = store.JobRepo().UpdateStatusDetail(context.Background(), jobID, domain.JobFailed, 100,
					domain.ErrCodeSpeedSourceUnavailable, err.Error(), "speed_source")
				return "", err
			}
		}
		job := application.TestJob{ID: jobID, NodeID: nodeID, Kind: kind}
		job.Retry = policy.RetryCount
		if kind == domain.TestThroughput {
			job.SpeedSourceHealth = func(sourceCtx context.Context) error {
				return checkSpeedSource(sourceCtx, target)
			}
		}
		job.OnStart = func() {
			_ = store.JobRepo().UpdateStatus(context.Background(), jobID, domain.JobRunning, 1, "")
		}
		job.Run = func(runCtx context.Context) (domain.MeasurementOutcome, error) {
			if err := reloadCoreConfig(runCtx); err != nil {
				return domain.MeasurementOutcome{Status: "failed", ErrorCode: domain.ErrCodeCoreAPIUnavailable}, err
			}
			if testerErr != nil || tester == nil {
				if testerErr == nil {
					testerErr = fmt.Errorf(domain.ErrCodeCoreUnavailable)
				}
				return domain.MeasurementOutcome{Status: "failed", ErrorCode: domain.ErrCodeCoreUnavailable}, testerErr
			}
			if kind == domain.TestThroughput {
				outcome, testErr := tester.Throughput(runCtx, mihomo.ProxyName(*node), target, maxBytes)
				outcome.ProxyProtocol, outcome.TestURL, outcome.SpeedSource = node.Protocol, target, target
				return outcome, testErr
			}
			if kind == domain.TestConnectivity {
				outcome, testErr := tester.Connectivity(runCtx, mihomo.ProxyName(*node), target)
				outcome.ProxyProtocol, outcome.TestURL = node.Protocol, target
				return outcome, testErr
			}
			result, pingErr := network.Ping(runCtx, node.Address, 1, 2*time.Second)
			outcome := domain.MeasurementOutcome{Status: "succeeded", Success: pingErr == nil,
				LatencyMS: result.LatencyMS, TestURL: node.Address, ProxyProtocol: node.Protocol,
				BaselineTarget: "www.baidu.com"}
			if pingErr != nil {
				outcome.Status = "failed"
				outcome.ErrorCode = "ping_failed"
				outcome.FailureStage = "icmp"
			}
			return outcome, pingErr
		}
		if err := orchestrator.Submit(job); err != nil {
			_ = store.JobRepo().UpdateStatusDetail(context.Background(), jobID, domain.JobFailed, 100,
				domain.ErrCodeMeasurementFailed, err.Error(), "queue")
			return "", err
		}
		return jobID, nil
	}
	testSubmit := func(ctx context.Context, nodeID string, kind domain.TestKind) (string, error) {
		return testSubmitWithSource(ctx, nodeID, kind, "")
	}
	automaticTests := application.NewAutomaticTestPlanner(
		store.NodeRepo(),
		store.MeasurementRepo(),
		func() bool {
			pending, running := queue.Snapshot()
			return running || pending > 0
		},
		testSubmitWithSource,
	)
	scheduler := application.Scheduler{
		RefreshInterval: time.Minute,
		TestInterval:    time.Second,
		CleanupInterval: time.Hour,
		Refresh: func(ctx context.Context) error {
			upstreams, err := store.UpstreamRepo().FindAll(ctx)
			if err != nil {
				return err
			}
			activeJobs, _ := store.JobRepo().FindActive(ctx, 0)
			for _, upstream := range upstreams {
				if !upstream.Enabled {
					continue
				}
				active := false
				for _, job := range activeJobs {
					if job.Kind == "refresh_upstream" && job.EntityID == upstream.ID {
						active = true
						break
					}
				}
				if active {
					continue
				}
				interval := upstream.RefreshInterval
				if interval <= 0 {
					interval = time.Hour
				}
				records, _ := store.UpstreamRepo().FindRefreshRecords(ctx, upstream.ID, 1)
				if len(records) > 0 && time.Since(records[0].CreatedAt) < interval {
					continue
				}
				_, _ = refreshRunner.Submit(ctx, upstream.ID)
			}
			return nil
		},
		Test: func(ctx context.Context, now time.Time) error {
			policy := loadTestPolicy(ctx, store, uciValues)
			localNow := now.Local()
			dayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, localNow.Location()).UTC()
			return automaticTests.Run(ctx, now, application.AutomaticTestPolicy{
				PingInterval:            time.Duration(policy.IntervalSeconds) * time.Second,
				ThroughputAllowed:       policy.ThroughputEnabled && inAllowedWindow(localNow, policy.AllowedWindows),
				ThroughputDayStart:      dayStart,
				ThroughputRetryInterval: 5 * time.Minute,
				SpeedSource:             selectSpeedSource(policy, localNow),
			})
		},
		Cleanup: func(ctx context.Context) error {
			now := time.Now().UTC()
			return store.ReportRepo().CleanupWithRetention(ctx, now.Add(-30*24*time.Hour), now.Add(-7*24*time.Hour), now.Add(-90*24*time.Hour), now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour))
		},
	}
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		scheduler.Run(runCtx)
	}()

	srv := httptransport.NewServerWithConfig(httptransport.Config{
		CoreStatus:        status,
		CoreInstallStatus: coreInstallStatus,
		CoreInstall:       coreInstall,
		CoreInstallUpload: coreInstallUpload,
		RuntimeLogs:       runtimeLogs,
		CoreRuntime: func(ctx context.Context) map[string]any {
			snapshot, err := core.GetConnections(ctx)
			if err != nil {
				return map[string]any{"connections": []any{}, "error": err.Error()}
			}
			return map[string]any{"connections": snapshot.Connections, "core_download_bytes": snapshot.DownloadTotal, "core_upload_bytes": snapshot.UploadTotal}
		},
		CoreTraffic: func() map[string]any {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			interfaceTraffic, err := network.ReadInterfaceTraffic(ctx)
			if err != nil {
				return map[string]any{"connections": 0, "download_bytes": 0, "upload_bytes": 0, "error": err.Error()}
			}
			snapshot, err := core.GetConnections(ctx)
			if err != nil {
				return map[string]any{
					"connections": 0, "download_bytes": interfaceTraffic.DownloadBytes,
					"upload_bytes": interfaceTraffic.UploadBytes, "interface": interfaceTraffic.Name,
					"error": err.Error(),
				}
			}
			return map[string]any{
				"connections":            len(snapshot.Connections),
				"bagualu_download_bytes": snapshot.DownloadTotal, "bagualu_upload_bytes": snapshot.UploadTotal,
				"wan_download_bytes": interfaceTraffic.DownloadBytes, "wan_upload_bytes": interfaceTraffic.UploadBytes,
				"download_bytes": interfaceTraffic.DownloadBytes, "upload_bytes": interfaceTraffic.UploadBytes,
				"interface": interfaceTraffic.Name, "core_download_bytes": snapshot.DownloadTotal,
				"core_upload_bytes": snapshot.UploadTotal,
			}
		},
		Store:         store,
		AdminPassword: *adminPassword,
		TestSubmit:    testSubmit,
		RefreshSubmit: refreshRunner.Submit,
		TestCancel: func(ctx context.Context, id string) (bool, error) {
			return orchestrator.Cancel(id)
		},
		ScoreRecalculate: scoreRunner.Submit,
		ScorePolicyGet:   scoreService.Policy,
		ScorePolicySet: func(policy domain.ScorePolicy) error {
			if err := scoreService.SetPolicy(policy); err != nil {
				return err
			}
			encoded, err := json.Marshal(policy)
			if err != nil {
				return err
			}
			return store.SettingsRepo().Set(context.Background(), "score_policy", string(encoded))
		},
		CoreReload: reloadCoreConfig,
		RuntimeSnapshot: func() map[string]any {
			pending, running := queue.Snapshot()
			id, nodeID, kind := queue.Current()
			return map[string]any{"queue_length": pending, "running": running, "current_task_id": id, "current_node_id": nodeID, "current_kind": kind}
		},
	})

	server := &http.Server{
		Addr:              *listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("bagualu listening on %s", *listen)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.Serve(managementListener) }()
	shutdown := func(closeServer bool) {
		if err := process.Stop(); err != nil {
			log.Printf("stop Mihomo during shutdown: %v", err)
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeServer {
			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Printf("shutdown management server: %v", err)
			}
		}
		select {
		case <-schedulerDone:
		case <-shutdownCtx.Done():
			log.Printf("scheduler shutdown timeout")
		}
		if !queue.WaitUntilIdle(shutdownCtx) {
			log.Printf("test queue shutdown timeout")
		}
		select {
		case <-queueDone:
		case <-shutdownCtx.Done():
			log.Printf("test queue worker shutdown timeout")
		}
	}
	select {
	case <-runCtx.Done():
		shutdown(true)
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			log.Printf("management server stopped: %v", err)
		}
		stopRun()
		shutdown(false)
	}
}

func writeRuntimeStatus(path string, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, encoded, 0644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func readUCIConfig(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if !strings.HasPrefix(line, "option ") {
			continue
		}
		fields := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(line, "option ")), " ", 2)
		if len(fields) != 2 {
			continue
		}
		value := strings.TrimSpace(fields[1])
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		values[fields[0]] = value
	}
	return values, nil
}

type runtimeTestPolicy struct {
	ThroughputEnabled bool
	ConnectivityURL   string
	ThroughputURLs    []string
	ThroughputBytes   int
	IntervalSeconds   int
	RetryCount        int
	AllowedWindows    []string
}

func loadTestPolicy(ctx context.Context, store *persistence.Store, uciValues ...map[string]string) runtimeTestPolicy {
	policy := runtimeTestPolicy{
		ThroughputEnabled: true,
		ConnectivityURL:   "http://www.gstatic.com/generate_204",
		ThroughputURLs:    []string{"https://speed.cloudflare.com/__down?bytes=1048576"},
		ThroughputBytes:   1 << 20,
		IntervalSeconds:   60,
		RetryCount:        2,
		AllowedWindows:    []string{"02:00-06:00"},
	}
	if store == nil {
		if len(uciValues) > 0 {
			applyUCIOverrides(&policy, uciValues[0])
		}
		return policy
	}
	if raw, err := store.SettingsRepo().Get(ctx, "test_policy"); err == nil && raw != "" {
		var values map[string]any
		if json.Unmarshal([]byte(raw), &values) == nil {
			if value, ok := values["throughput_enabled"].(bool); ok {
				policy.ThroughputEnabled = value
			}
			if value, ok := values["connectivity_url"].(string); ok && strings.TrimSpace(value) != "" {
				policy.ConnectivityURL = value
			}
			if value, ok := values["throughput_url"].(string); ok && strings.TrimSpace(value) != "" {
				policy.ThroughputURLs = []string{strings.TrimSpace(value)}
			}
			if sources, ok := values["speed_sources"].([]any); ok {
				policy.ThroughputURLs = policy.ThroughputURLs[:0]
				for _, value := range sources {
					if source, valid := value.(string); valid && strings.TrimSpace(source) != "" {
						policy.ThroughputURLs = append(policy.ThroughputURLs, strings.TrimSpace(source))
					}
				}
			}
			if value, ok := values["throughput_bytes"].(float64); ok && value >= 1024 && value <= 100<<20 {
				policy.ThroughputBytes = int(value)
			}
			if value, ok := values["interval_seconds"].(float64); ok && value >= 1 && value <= 86400 {
				policy.IntervalSeconds = int(value)
			}
			if value, ok := values["retry_count"].(float64); ok && value >= 0 && value <= 5 {
				policy.RetryCount = int(value)
			}
			if windows, ok := values["allowed_windows"].([]any); ok {
				policy.AllowedWindows = policy.AllowedWindows[:0]
				for _, value := range windows {
					if window, valid := value.(string); valid && strings.TrimSpace(window) != "" {
						policy.AllowedWindows = append(policy.AllowedWindows, window)
					}
				}
			}
		}
	}
	if len(uciValues) > 0 {
		applyUCIOverrides(&policy, uciValues[0])
	}
	return policy
}

func parseBandwidthConfig(download, upload, threshold float64, values map[string]string) (float64, float64, float64) {
	if value := values["wan_download_bps"]; value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			download = parsed
		}
	}
	if value := values["wan_upload_bps"]; value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			upload = parsed
		}
	}
	if value := values["load_threshold"]; value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			threshold = parsed
		}
	}
	return download, upload, threshold
}

func applyUCIOverrides(policy *runtimeTestPolicy, values map[string]string) {
	if value, ok := values["throughput_enabled"]; ok {
		policy.ThroughputEnabled = value != "0" && strings.ToLower(value) != "false"
	}
	if value, ok := values["throughput_url"]; ok && strings.TrimSpace(value) != "" {
		policy.ThroughputURLs = []string{strings.TrimSpace(value)}
	}
	if value, ok := values["throughput_urls"]; ok {
		policy.ThroughputURLs = nil
		for _, source := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(source); trimmed != "" {
				policy.ThroughputURLs = append(policy.ThroughputURLs, trimmed)
			}
		}
	}
	if value, ok := values["throughput_bytes"]; ok {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 1024 && parsed <= 100<<20 {
			policy.ThroughputBytes = parsed
		}
	}
	if value, ok := values["interval_seconds"]; ok {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 1 && parsed <= 86400 {
			policy.IntervalSeconds = parsed
		}
	}
	if value, ok := values["throughput_windows"]; ok {
		policy.AllowedWindows = nil
		for _, window := range strings.Split(value, ",") {
			if trimmed := strings.TrimSpace(window); trimmed != "" {
				policy.AllowedWindows = append(policy.AllowedWindows, trimmed)
			}
		}
	}
}

func selectSpeedSource(policy runtimeTestPolicy, now time.Time) string {
	if len(policy.ThroughputURLs) == 0 {
		return ""
	}
	return policy.ThroughputURLs[(now.YearDay()-1)%len(policy.ThroughputURLs)]
}

func inAllowedWindow(now time.Time, windows []string) bool {
	if len(windows) == 0 {
		return true
	}
	minutes := now.Hour()*60 + now.Minute()
	for _, raw := range windows {
		parts := strings.SplitN(raw, "-", 2)
		if len(parts) != 2 {
			continue
		}
		start, startOK := parseClock(parts[0])
		end, endOK := parseClock(parts[1])
		if !startOK || !endOK {
			continue
		}
		if start <= end && minutes >= start && minutes < end || start > end && (minutes >= start || minutes < end) {
			return true
		}
	}
	return false
}

func parseClock(value string) (int, bool) {
	parts := strings.SplitN(strings.TrimSpace(value), ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func checkSpeedSource(ctx context.Context, target string) error {
	return checkSpeedSourceWithClient(ctx, target, sharedSpeedSourceClient)
}

var sharedSpeedSourceClient = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return fmt.Errorf("too many redirects")
	}
	return nil
}}

func checkSpeedSourceWithClient(ctx context.Context, target string, client *http.Client) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Range", "bytes=0-0")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("speed source returned HTTP %d", response.StatusCode)
	}
	return nil
}
