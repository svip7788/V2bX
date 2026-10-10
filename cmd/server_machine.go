package cmd

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
	"github.com/InazumaV/V2bX/machine"
	log "github.com/sirupsen/logrus"
)

// runMachineServer 机器模式单进程运行：节点由面板下发并可热增删，故不拆子进程
func runMachineServer(c *conf.Conf) error {
	mc := c.MachineConfig
	if strings.TrimSpace(mc.APIHost) == "" || mc.Token == "" {
		return fmt.Errorf("Machine.ApiHost 和 Machine.Token 不能为空")
	}
	if len(c.NodeConfig) > 0 {
		log.Warn("Machine mode enabled, Nodes in config are ignored")
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
	cores := mc.CoreConfigs(c.CoresConfig)
	coreTypes := make([]string, 0, len(cores))
	for _, cc := range cores {
		coreTypes = append(coreTypes, strings.ToLower(cc.Type))
	}

	vc, err := vCore.NewCore(cores)
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

	m, err := machine.NewManager(mc, vc, coreTypes, version)
	if err != nil {
		return err
	}
	m.Start()
	defer m.Close()

	runtime.GC()
	waitForShutdownSignal()
	return nil
}
