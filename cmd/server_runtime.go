package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
	"github.com/InazumaV/V2bX/node"
	log "github.com/sirupsen/logrus"
)

const (
	childRestartDelay = 3 * time.Second
	childStopTimeout  = 10 * time.Second
)

type workerGeneration struct {
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	children map[int]*exec.Cmd
	wg       sync.WaitGroup
}

func newWorkerGeneration() *workerGeneration {
	ctx, cancel := context.WithCancel(context.Background())
	return &workerGeneration{
		ctx:      ctx,
		cancel:   cancel,
		children: make(map[int]*exec.Cmd),
	}
}

func (g *workerGeneration) setChild(nodeID int, cmd *exec.Cmd) {
	g.mu.Lock()
	g.children[nodeID] = cmd
	g.mu.Unlock()
}

func (g *workerGeneration) removeChild(nodeID int, cmd *exec.Cmd) {
	g.mu.Lock()
	if current, ok := g.children[nodeID]; ok && current == cmd {
		delete(g.children, nodeID)
	}
	g.mu.Unlock()
}

func (g *workerGeneration) snapshotChildren() map[int]*exec.Cmd {
	g.mu.Lock()
	defer g.mu.Unlock()
	snapshot := make(map[int]*exec.Cmd, len(g.children))
	for nodeID, cmd := range g.children {
		snapshot[nodeID] = cmd
	}
	return snapshot
}

type nodeProcessManager struct {
	configPath string

	mu      sync.Mutex
	current *workerGeneration
}

func newNodeProcessManager(configPath string) *nodeProcessManager {
	return &nodeProcessManager{configPath: configPath}
}

func (m *nodeProcessManager) StartAll(c *conf.Conf) error {
	nodeIDs, err := collectNodeIDs(c.NodeConfig)
	if err != nil {
		return err
	}
	gen := newWorkerGeneration()
	for _, nodeID := range nodeIDs {
		gen.wg.Add(1)
		go m.runChildLoop(gen, nodeID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil {
		gen.cancel()
		gen.wg.Wait()
		return errors.New("worker processes already running")
	}
	m.current = gen
	return nil
}

func (m *nodeProcessManager) Replace(c *conf.Conf) error {
	if _, err := collectNodeIDs(c.NodeConfig); err != nil {
		return err
	}
	if err := m.StopAll(); err != nil {
		return err
	}
	runtime.GC()
	return m.StartAll(c)
}

func (m *nodeProcessManager) StopAll() error {
	m.mu.Lock()
	gen := m.current
	m.current = nil
	m.mu.Unlock()
	if gen == nil {
		return nil
	}

	gen.cancel()
	sendSignalToChildren(gen.snapshotChildren(), syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		gen.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(childStopTimeout):
		log.Warn("Worker processes did not exit in time, forcing shutdown")
		sendSignalToChildren(gen.snapshotChildren(), os.Kill)
		<-done
		return nil
	}
}

func (m *nodeProcessManager) runChildLoop(gen *workerGeneration, nodeID int) {
	defer gen.wg.Done()

	for {
		select {
		case <-gen.ctx.Done():
			return
		default:
		}

		cmd, err := m.buildChildCommand(nodeID)
		if err != nil {
			log.WithFields(log.Fields{
				"node_id": nodeID,
				"err":     err,
			}).Error("Build child process failed")
			if !waitForRestart(gen.ctx) {
				return
			}
			continue
		}

		if err := cmd.Start(); err != nil {
			log.WithFields(log.Fields{
				"node_id": nodeID,
				"err":     err,
			}).Error("Start child process failed")
			if !waitForRestart(gen.ctx) {
				return
			}
			continue
		}

		gen.setChild(nodeID, cmd)
		log.WithFields(log.Fields{
			"node_id": nodeID,
			"pid":     cmd.Process.Pid,
		}).Info("Node worker started")

		err = cmd.Wait()
		gen.removeChild(nodeID, cmd)
		if gen.ctx.Err() != nil {
			return
		}

		fields := log.Fields{"node_id": nodeID}
		if cmd.Process != nil {
			fields["pid"] = cmd.Process.Pid
		}
		if err != nil {
			fields["err"] = err
			log.WithFields(fields).Warn("Node worker exited unexpectedly, restarting")
		} else {
			log.WithFields(fields).Warn("Node worker exited, restarting")
		}

		if !waitForRestart(gen.ctx) {
			return
		}
	}
}

func (m *nodeProcessManager) buildChildCommand(nodeID int) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable error: %w", err)
	}
	workDir := childWorkDir(executable, nodeID)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("create child work dir error: %w", err)
	}
	args := []string{
		"server",
		"-c", m.configPath,
		"--watch=false",
		"--child-node-id", strconv.Itoa(nodeID),
	}
	cmd := exec.Command(executable, args...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd, nil
}

func configureServerLogging(c *conf.Conf) (func(), error) {
	switch strings.ToLower(c.LogConfig.Level) {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}

	if c.LogConfig.Output == "" {
		return func() {}, nil
	}
	f, err := os.OpenFile(c.LogConfig.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return func() {}, err
	}
	log.SetOutput(f)
	return func() {
		_ = f.Close()
	}, nil
}

func runMasterServer(c *conf.Conf, configPath string, enableWatch bool) error {
	nodeIDs, err := collectNodeIDs(c.NodeConfig)
	if err != nil {
		return err
	}
	if len(nodeIDs) > 1 && c.LogConfig.PprofListen != "" {
		log.WithField("listen", c.LogConfig.PprofListen).Warn("Pprof is disabled in master-worker mode with multiple nodes")
	}

	manager := newNodeProcessManager(configPath)
	if err := manager.StartAll(c); err != nil {
		return err
	}
	defer func() {
		if err := manager.StopAll(); err != nil {
			log.WithField("err", err).Error("Stop worker processes failed")
		}
	}()

	log.WithField("node_ids", formatNodeIDs(c.NodeConfig)).Info("V2bX master started")

	if enableWatch {
		xdns, sdns := detectMasterWatchPaths(c)
		if err := c.Watch(configPath, xdns, sdns, func() {
			if err := manager.Replace(c); err != nil {
				log.WithField("err", err).Error("Reload worker processes failed")
				return
			}
			log.WithField("node_ids", formatNodeIDs(c.NodeConfig)).Info("Worker processes reloaded")
			runtime.GC()
		}); err != nil {
			return err
		}
	}

	waitForShutdownSignal()
	return nil
}

func runChildNodeServer(c *conf.Conf, configPath string, nodeID int) error {
	fullNodeCount := len(c.NodeConfig)
	selectedNodes, err := filterNodeConfigsByID(c.NodeConfig, nodeID)
	if err != nil {
		return err
	}
	c.NodeConfig = selectedNodes
	c.CoresConfig, err = prepareChildCoreConfigs(c.CoresConfig, selectedNodes[0], nodeID)
	if err != nil {
		return err
	}

	if fullNodeCount > 1 && c.LogConfig.PprofListen != "" {
		log.WithFields(log.Fields{
			"node_id": nodeID,
			"listen":  c.LogConfig.PprofListen,
		}).Warn("Skip pprof in child mode to avoid port conflicts")
		c.LogConfig.PprofListen = ""
	}

	log.WithField("node_id", nodeID).Info("Node worker booting")
	return runManagedNodeServer(c, configPath, false)
}

func runManagedNodeServer(c *conf.Conf, configPath string, enableWatch bool) error {
	if len(c.NodeConfig) == 0 {
		return errors.New("no node config")
	}

	if c.LogConfig.PprofListen != "" {
		go func() {
			log.Infof("pprof listening on %s", c.LogConfig.PprofListen)
			if err := http.ListenAndServe(c.LogConfig.PprofListen, nil); err != nil {
				log.WithField("err", err).Error("pprof server failed")
			}
		}()
	}

	limiter.Init()
	log.WithField("node_ids", formatNodeIDs(c.NodeConfig)).Info("Start V2bX worker runtime")

	vc, err := vCore.NewCore(c.CoresConfig)
	if err != nil {
		return fmt.Errorf("new core failed: %w", err)
	}
	defer func() {
		if err := vc.Close(); err != nil {
			log.WithField("err", err).Error("Close core failed")
		}
	}()

	if err := vc.Start(); err != nil {
		return fmt.Errorf("start core failed: %w", err)
	}
	log.Info("Core ", vc.Type(), " started")

	nodes := node.New()
	defer nodes.Close()

	if err := nodes.Start(c.NodeConfig, vc); err != nil {
		return fmt.Errorf("run nodes failed: %w", err)
	}
	log.Info("Nodes started")

	if enableWatch {
		xdns := os.Getenv("XRAY_DNS_PATH")
		sdns := os.Getenv("SING_DNS_PATH")
		if err := c.Watch(configPath, xdns, sdns, func() {
			nodes.Close()
			if err := vc.Close(); err != nil {
				log.WithField("err", err).Error("Restart node failed")
				return
			}
			vc, err = vCore.NewCore(c.CoresConfig)
			if err != nil {
				log.WithField("err", err).Error("New core failed")
				return
			}
			if err := vc.Start(); err != nil {
				log.WithField("err", err).Error("Start core failed")
				return
			}
			log.Info("Core ", vc.Type(), " restarted")
			if err := nodes.Start(c.NodeConfig, vc); err != nil {
				log.WithField("err", err).Error("Run nodes failed")
				return
			}
			log.Info("Nodes restarted")
			runtime.GC()
		}); err != nil {
			return err
		}
	}

	runtime.GC()
	waitForShutdownSignal()
	return nil
}

func detectMasterWatchPaths(c *conf.Conf) (string, string) {
	for _, coreConfig := range c.CoresConfig {
		if coreConfig.XrayConfig != nil && coreConfig.XrayConfig.DnsConfigPath != "" {
			return coreConfig.XrayConfig.DnsConfigPath, ""
		}
	}
	return "", ""
}

func collectNodeIDs(nodes []conf.NodeConfig) ([]int, error) {
	if len(nodes) == 0 {
		return nil, errors.New("no node config")
	}
	ids := make([]int, 0, len(nodes))
	seen := make(map[int]struct{}, len(nodes))
	for _, nodeConfig := range nodes {
		nodeID := nodeConfig.ApiConfig.NodeID
		if nodeID <= 0 {
			return nil, fmt.Errorf("invalid node id: %d", nodeID)
		}
		if _, ok := seen[nodeID]; ok {
			return nil, fmt.Errorf("duplicate node id: %d", nodeID)
		}
		seen[nodeID] = struct{}{}
		ids = append(ids, nodeID)
	}
	sort.Ints(ids)
	return ids, nil
}

func filterNodeConfigsByID(nodes []conf.NodeConfig, nodeID int) ([]conf.NodeConfig, error) {
	for _, nodeConfig := range nodes {
		if nodeConfig.ApiConfig.NodeID == nodeID {
			return []conf.NodeConfig{nodeConfig}, nil
		}
	}
	return nil, fmt.Errorf("node id %d not found in config", nodeID)
}

func filterCoreConfigsForNode(coreConfigs []conf.CoreConfig, nodeConfig conf.NodeConfig) []conf.CoreConfig {
	if len(coreConfigs) <= 1 {
		return coreConfigs
	}

	if nodeConfig.Options.CoreName != "" {
		filtered := make([]conf.CoreConfig, 0, 1)
		for _, coreConfig := range coreConfigs {
			if coreConfig.Name == nodeConfig.Options.CoreName {
				filtered = append(filtered, coreConfig)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	if nodeConfig.Options.Core != "" {
		filtered := make([]conf.CoreConfig, 0, len(coreConfigs))
		for _, coreConfig := range coreConfigs {
			if strings.EqualFold(coreConfig.Type, nodeConfig.Options.Core) {
				filtered = append(filtered, coreConfig)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	return coreConfigs
}

func prepareChildCoreConfigs(coreConfigs []conf.CoreConfig, nodeConfig conf.NodeConfig, nodeID int) ([]conf.CoreConfig, error) {
	filtered := filterCoreConfigsForNode(coreConfigs, nodeConfig)
	cloned := make([]conf.CoreConfig, len(filtered))
	copy(cloned, filtered)

	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable error: %w", err)
	}
	workDir := childWorkDir(executable, nodeID)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("create child work dir error: %w", err)
	}

	for i := range cloned {
		if cloned[i].SingConfig == nil {
			continue
		}
		singCopy := *cloned[i].SingConfig
		cloned[i].SingConfig = &singCopy
		if singCopy.OriginalPath == "" {
			continue
		}
		childPath, err := prepareChildSingConfig(singCopy.OriginalPath, workDir, nodeID)
		if err != nil {
			return nil, err
		}
		cloned[i].SingConfig.OriginalPath = childPath
	}

	return cloned, nil
}

func prepareChildSingConfig(originalPath, workDir string, nodeID int) (string, error) {
	data, err := os.ReadFile(originalPath)
	if err != nil {
		return "", fmt.Errorf("read sing original config error: %w", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return "", fmt.Errorf("unmarshal sing original config error: %w", err)
	}

	experimental := getOrCreateMap(root, "experimental")
	cacheFile := getOrCreateMap(experimental, "cache_file")
	cacheFile["enabled"] = true
	cacheFile["path"] = filepath.Join(workDir, "cache.db")

	childPath := filepath.Join(workDir, fmt.Sprintf("sing_origin_%d.json", nodeID))
	content, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal child sing config error: %w", err)
	}
	if err := os.WriteFile(childPath, content, 0644); err != nil {
		return "", fmt.Errorf("write child sing config error: %w", err)
	}
	return childPath, nil
}

func formatNodeIDs(nodes []conf.NodeConfig) string {
	if len(nodes) == 0 {
		return ""
	}
	ids := make([]string, 0, len(nodes))
	for _, nodeConfig := range nodes {
		ids = append(ids, strconv.Itoa(nodeConfig.ApiConfig.NodeID))
	}
	return strings.Join(ids, ",")
}

func childWorkDir(executable string, nodeID int) string {
	return filepath.Join(filepath.Dir(executable), fmt.Sprintf("node-%d", nodeID))
}

func getOrCreateMap(parent map[string]interface{}, key string) map[string]interface{} {
	if value, ok := parent[key]; ok {
		if child, ok := value.(map[string]interface{}); ok {
			return child
		}
	}
	child := make(map[string]interface{})
	parent[key] = child
	return child
}

func sendSignalToChildren(children map[int]*exec.Cmd, sig os.Signal) {
	for nodeID, cmd := range children {
		if cmd == nil || cmd.Process == nil {
			continue
		}
		if err := cmd.Process.Signal(sig); err != nil && !errors.Is(err, os.ErrProcessDone) {
			log.WithFields(log.Fields{
				"node_id": nodeID,
				"pid":     cmd.Process.Pid,
				"err":     err,
			}).Warn("Signal child process failed")
		}
	}
}

func waitForRestart(ctx context.Context) bool {
	timer := time.NewTimer(childRestartDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func waitForShutdownSignal() {
	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(osSignals)
	<-osSignals
}
