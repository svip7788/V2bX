package node

import (
	"context"
	"errors"

	"github.com/InazumaV/V2bX/api/panel"
	log "github.com/sirupsen/logrus"
)

func (c *Controller) reportUserTrafficTask() (err error) {
	c.runtimeMu.Lock()
	defer c.runtimeMu.Unlock()
	defer c.limiter.MarkOnlineDeviceReported()

	ctx := c.currentCtx()
	if ctx.Err() != nil {
		return nil
	}

	userTraffic, trafficErr := c.server.GetUserTrafficSlice(c.tag, true)
	if trafficErr != nil {
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": trafficErr,
		}).Error("Get user traffic slice failed")
	}

	var aliveData map[int][]string
	var onlineDeviceCount int
	if onlineDevice, err := c.limiter.GetOnlineDevice(); err != nil {
		log.Print(err)
	} else if len(onlineDevice) > 0 {
		onlineDeviceCount = len(onlineDevice)
		var nocountUID map[int]struct{}
		if len(userTraffic) > 0 && c.Options.DeviceOnlineMinTraffic > 0 {
			nocountUID = make(map[int]struct{})
			minTraffic := int64(c.Options.DeviceOnlineMinTraffic * 1000)
			for _, traffic := range userTraffic {
				if traffic.Upload+traffic.Download < minTraffic {
					nocountUID[traffic.UID] = struct{}{}
				}
			}
		}
		aliveData = make(map[int][]string, len(onlineDevice))
		for _, online := range onlineDevice {
			if nocountUID != nil {
				if _, ok := nocountUID[online.UID]; ok {
					continue
				}
			}
			aliveData[online.UID] = append(aliveData[online.UID], online.IP)
		}
	}

	if len(userTraffic) == 0 && len(aliveData) == 0 {
		return nil
	}

	if c.wsClient != nil && c.wsClient.IsConnected() && len(aliveData) > 0 {
		c.wsClient.SendDeviceReport(aliveData)
	}

	reportErr := c.apiClient.Report(ctx, userTraffic, aliveData)
	if reportErr != nil {
		entry := log.WithFields(log.Fields{
			"tag": c.tag,
			"err": reportErr,
		})
		switch {
		case errors.Is(reportErr, context.Canceled), errors.Is(reportErr, context.DeadlineExceeded):
			entry.Info("V2 report cancelled, traffic discarded")
		case panel.IsUnsupportedReportError(reportErr):
			entry.Info("V2 report unsupported, fallback to V1")
			c.reportV1(userTraffic, aliveData, onlineDeviceCount)
		default:
			entry.Warn("V2 report failed, traffic discarded")
		}
		return nil
	}
	if len(userTraffic) > 0 {
		c.addDynamicTraffic(userTraffic)
		var totalUp, totalDown int64
		for i := range userTraffic {
			totalUp += userTraffic[i].Upload
			totalDown += userTraffic[i].Download
		}
		log.WithField("tag", c.tag).Infof("Report %d users traffic (up=%d down=%d total=%.2f MB)",
			len(userTraffic), totalUp, totalDown, float64(totalUp+totalDown)/1024.0/1024.0)
	}
	if onlineDeviceCount > 0 {
		log.WithField("tag", c.tag).Infof("Total %d online users, %d Reported", onlineDeviceCount, len(aliveData))
	}
	return nil
}

func (c *Controller) reportV1(userTraffic []panel.UserTraffic, aliveData map[int][]string, onlineDeviceCount int) {
	ctx := c.currentCtx()
	if len(userTraffic) > 0 {
		err := c.apiClient.ReportUserTraffic(ctx, userTraffic)
		if err != nil {
			entry := log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			})
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				entry.Info("V1 report cancelled, traffic discarded")
			} else {
				entry.Info("Report user traffic failed")
			}
		} else {
			c.addDynamicTraffic(userTraffic)
			var totalUp, totalDown int64
			for i := range userTraffic {
				totalUp += userTraffic[i].Upload
				totalDown += userTraffic[i].Download
			}
			log.WithField("tag", c.tag).Infof("Report %d users traffic (up=%d down=%d total=%.2f MB)",
				len(userTraffic), totalUp, totalDown, float64(totalUp+totalDown)/1024.0/1024.0)
		}
	}
	if len(aliveData) > 0 {
		if err := c.apiClient.ReportNodeOnlineUsers(ctx, &aliveData); err != nil {
			entry := log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			})
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				entry.Info("Report online users cancelled")
			} else {
				entry.Info("Report online users failed")
			}
		} else {
			log.WithField("tag", c.tag).Infof("Total %d online users, %d Reported", onlineDeviceCount, len(aliveData))
		}
	}
}

func (c *Controller) rebuildUIDToUUID() {
	m := make(map[int]string, len(c.userList))
	for i := range c.userList {
		m[c.userList[i].Id] = c.userList[i].Uuid
	}
	c.uidToUUID = m
}

func (c *Controller) addDynamicTraffic(userTraffic []panel.UserTraffic) {
	if !c.LimitConfig.EnableDynamicSpeedLimit || c.LimitConfig.DynamicSpeedLimitConfig == nil {
		return
	}
	if len(userTraffic) == 0 {
		return
	}
	if c.traffic == nil {
		c.traffic = make(map[string]int64)
	}
	for i := range userTraffic {
		uuid, ok := c.uidToUUID[userTraffic[i].UID]
		if !ok {
			continue
		}
		c.traffic[uuid] += userTraffic[i].Upload + userTraffic[i].Download
	}
}

type userCompareKey struct {
	uuid   string
	speed  int
	device int
}

func compareUserList(old, new []panel.UserInfo) (deleted, added []panel.UserInfo) {
	oldMap := make(map[userCompareKey]int, len(old))
	for i, user := range old {
		key := userCompareKey{
			uuid:   user.Uuid,
			speed:  user.SpeedLimit,
			device: user.DeviceLimit,
		}
		oldMap[key] = i
	}

	for _, user := range new {
		key := userCompareKey{
			uuid:   user.Uuid,
			speed:  user.SpeedLimit,
			device: user.DeviceLimit,
		}
		if _, exists := oldMap[key]; !exists {
			added = append(added, user)
		} else {
			delete(oldMap, key)
		}
	}

	for _, index := range oldMap {
		deleted = append(deleted, old[index])
	}

	return deleted, added
}
