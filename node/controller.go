package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/task"
	"github.com/InazumaV/V2bX/conf"
	vCore "github.com/InazumaV/V2bX/core"
	"github.com/InazumaV/V2bX/limiter"
	log "github.com/sirupsen/logrus"
)

type Controller struct {
	server                    vCore.Core
	apiClient                 *panel.Client
	tag                       string
	limiter                   *limiter.Limiter
	traffic                   map[string]int64
	userList                  []panel.UserInfo
	aliveMap                  map[int]int
	info                      *panel.NodeInfo
	nodeInfoMonitorPeriodic   *task.Task
	userReportPeriodic        *task.Task
	renewCertPeriodic         *task.Task
	dynamicSpeedLimitPeriodic *task.Task
	onlineIpReportPeriodic    *task.Task
	wsClient                  *panel.WSClient
	wsStopCh                  chan struct{}
	runtimeMu                 sync.Mutex
	*conf.Options
}

// NewController return a Node controller with default parameters.
func NewController(server vCore.Core, api *panel.Client, config *conf.Options) *Controller {
	controller := &Controller{
		server:    server,
		Options:   config,
		apiClient: api,
	}
	return controller
}

// Start implement the Start() function of the service interface
func (c *Controller) Start() error {
	// First fetch Node Info
	var err error
	node, err := c.apiClient.GetNodeInfo()
	if err != nil {
		return fmt.Errorf("get node info error: %s", err)
	}
	// Update user
	c.userList, err = c.apiClient.GetUserList()
	if err != nil {
		return fmt.Errorf("get user list error: %s", err)
	}
	if len(c.userList) == 0 {
		return errors.New("add users error: not have any user")
	}
	c.aliveMap, err = c.apiClient.GetUserAlive()
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Warn("Get user alive list failed, fallback to empty alive list")
		c.aliveMap = make(map[int]int)
	}
	if len(c.Options.Name) == 0 {
		c.tag = c.buildNodeTag(node)
	} else {
		c.tag = c.Options.Name
	}

	// add limiter
	l := limiter.AddLimiter(c.tag, &c.LimitConfig, c.userList, c.aliveMap)
	// add rule limiter
	if err = l.UpdateRule(&node.Rules); err != nil {
		return fmt.Errorf("update rule error: %s", err)
	}
	c.limiter = l
	c.applyPanelCertConfig(node)
	if node.Security == panel.Tls {
		err = c.requestCert()
		if err != nil {
			return fmt.Errorf("request cert error: %s", err)
		}
	}
	// Add new tag
	err = c.server.AddNode(c.tag, node, c.Options)
	if err != nil {
		return fmt.Errorf("add new node error: %s", err)
	}
	added, err := c.server.AddUsers(&vCore.AddUsersParams{
		Tag:      c.tag,
		Users:    c.userList,
		NodeInfo: node,
	})
	if err != nil {
		return fmt.Errorf("add users error: %s", err)
	}
	log.WithField("tag", c.tag).Infof("Added %d new users", added)
	c.info = node
	c.startTasks(node)
	c.tryStartWebSocket()
	return nil
}

// Close implement the Close() function of the service interface
func (c *Controller) Close() error {
	if c.wsStopCh != nil {
		close(c.wsStopCh)
	}
	if c.wsClient != nil {
		c.wsClient.Close()
	}
	limiter.DeleteLimiter(c.tag)
	if c.nodeInfoMonitorPeriodic != nil {
		c.nodeInfoMonitorPeriodic.Close()
	}
	if c.userReportPeriodic != nil {
		c.userReportPeriodic.Close()
	}
	if c.renewCertPeriodic != nil {
		c.renewCertPeriodic.Close()
	}
	if c.dynamicSpeedLimitPeriodic != nil {
		c.dynamicSpeedLimitPeriodic.Close()
	}
	if c.onlineIpReportPeriodic != nil {
		c.onlineIpReportPeriodic.Close()
	}
	err := c.server.DelNode(c.tag)
	if err != nil {
		return fmt.Errorf("del node error: %s", err)
	}
	return nil
}

func (c *Controller) buildNodeTag(node *panel.NodeInfo) string {
	return fmt.Sprintf("[%s]-%s:%d", c.apiClient.APIHost, node.Type, node.Id)
}

func (c *Controller) tryStartWebSocket() {
	resp, err := c.apiClient.Handshake()
	if err != nil {
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Info("WS handshake failed, using HTTP polling only")
		return
	}
	if !resp.Websocket.Enabled || resp.Websocket.WsURL == "" {
		log.WithField("tag", c.tag).Info("WS not enabled by panel, using HTTP polling only")
		return
	}
	ws, err := c.apiClient.NewWSClient(resp.Websocket.WsURL)
	if err != nil {
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": err,
		}).Warn("WS connect failed, using HTTP polling only")
		return
	}
	c.wsClient = ws
	c.wsStopCh = make(chan struct{})
	go c.handleWSEvents()
	log.WithField("tag", c.tag).Info("WS real-time push enabled")
}

func (c *Controller) handleWSEvents() {
	for {
		select {
		case <-c.wsStopCh:
			return
		case evt, ok := <-c.wsClient.Events():
			if !ok {
				return
			}
			c.processWSEvent(evt)
		}
	}
}

func (c *Controller) processWSEvent(evt panel.WSEvent) {
	switch evt.Event {
	case "sync.config":
		log.WithField("tag", c.tag).Info("WS: received config push, triggering reload")
		if err := c.nodeInfoMonitor(); err != nil {
			log.WithField("err", err).Warn("WS: nodeInfoMonitor failed after sync.config")
		}
		return
	}

	c.runtimeMu.Lock()
	defer c.runtimeMu.Unlock()

	switch evt.Event {
	case "sync.users":
		var data panel.WSUsersData
		if err := json.Unmarshal(evt.Data, &data); err != nil {
			log.WithField("err", err).Warn("WS: unmarshal sync.users failed")
			return
		}
		c.applyFullUsers(data.Users)

	case "sync.user.delta":
		var data panel.WSUserDeltaData
		if err := json.Unmarshal(evt.Data, &data); err != nil {
			log.WithField("err", err).Warn("WS: unmarshal sync.user.delta failed")
			return
		}
		c.applyUserDelta(data.Action, data.Users)

	case "sync.devices":
		var data panel.WSDevicesData
		if err := json.Unmarshal(evt.Data, &data); err != nil {
			log.WithField("err", err).Warn("WS: unmarshal sync.devices failed")
			return
		}
		alive := make(map[int]int, len(data.Users))
		for uidStr, ips := range data.Users {
			var uid int
			fmt.Sscanf(uidStr, "%d", &uid)
			alive[uid] = len(ips)
		}
		c.limiter.SetAliveList(alive)
		c.aliveMap = alive
	}
}

func (c *Controller) applyFullUsers(newUsers []panel.UserInfo) {
	deleted, added := compareUserList(c.userList, newUsers)
	if len(deleted) > 0 {
		if err := c.server.DelUsers(deleted, c.tag, c.info); err != nil {
			log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			}).Error("WS: delete users failed")
			return
		}
	}
	if len(added) > 0 {
		if _, err := c.server.AddUsers(&vCore.AddUsersParams{
			Tag:      c.tag,
			NodeInfo: c.info,
			Users:    added,
		}); err != nil {
			log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			}).Error("WS: add users failed")
			return
		}
	}
	if len(added) > 0 || len(deleted) > 0 {
		c.limiter.UpdateUser(c.tag, added, deleted)
		if c.LimitConfig.EnableDynamicSpeedLimit {
			for i := range deleted {
				delete(c.traffic, deleted[i].Uuid)
			}
		}
		log.WithField("tag", c.tag).
			Infof("WS: %d user deleted, %d user added", len(deleted), len(added))
	}
	c.userList = newUsers
}

func (c *Controller) applyUserDelta(action string, users []panel.UserInfo) {
	switch action {
	case "add":
		if len(users) == 0 {
			return
		}
		if _, err := c.server.AddUsers(&vCore.AddUsersParams{
			Tag:      c.tag,
			NodeInfo: c.info,
			Users:    users,
		}); err != nil {
			log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			}).Error("WS: delta add users failed")
			return
		}
		c.limiter.UpdateUser(c.tag, users, nil)
		c.userList = append(c.userList, users...)
		log.WithField("tag", c.tag).Infof("WS: delta added %d users", len(users))

	case "remove":
		if len(users) == 0 {
			return
		}
		if err := c.server.DelUsers(users, c.tag, c.info); err != nil {
			log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			}).Error("WS: delta remove users failed")
			return
		}
		c.limiter.UpdateUser(c.tag, nil, users)
		removeSet := make(map[string]struct{}, len(users))
		for _, u := range users {
			removeSet[u.Uuid] = struct{}{}
			if c.LimitConfig.EnableDynamicSpeedLimit {
				delete(c.traffic, u.Uuid)
			}
		}
		filtered := c.userList[:0]
		for _, u := range c.userList {
			if _, rm := removeSet[u.Uuid]; !rm {
				filtered = append(filtered, u)
			}
		}
		c.userList = filtered
		log.WithField("tag", c.tag).Infof("WS: delta removed %d users", len(users))
	}
}

func (c *Controller) applyPanelCertConfig(node *panel.NodeInfo) {
	if node.CertConfig == nil {
		return
	}
	pc := node.CertConfig
	if pc.CertMode != "" {
		c.CertConfig.CertMode = pc.CertMode
	}
	if pc.CertFile != "" {
		c.CertConfig.CertFile = pc.CertFile
	}
	if pc.KeyFile != "" {
		c.CertConfig.KeyFile = pc.KeyFile
	}
	if pc.CertDomain != "" {
		c.CertConfig.CertDomain = pc.CertDomain
	}
	if pc.Provider != "" {
		c.CertConfig.Provider = pc.Provider
	}
	if pc.Email != "" {
		c.CertConfig.Email = pc.Email
	}
	c.CertConfig.RejectUnknownSni = pc.RejectUnknownSni
	log.WithField("tag", c.tag).Info("Applied cert config from panel")
}
