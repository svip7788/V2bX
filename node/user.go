package node

import (
	"github.com/InazumaV/V2bX/api/panel"
	log "github.com/sirupsen/logrus"
)

func (c *Controller) reportUserTrafficTask() (err error) {
	c.runtimeMu.Lock()
	defer c.runtimeMu.Unlock()

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

	reportErr := c.apiClient.Report(userTraffic, aliveData)
	if reportErr != nil {
		log.WithFields(log.Fields{
			"tag": c.tag,
			"err": reportErr,
		}).Info("V2 report failed, fallback to V1")
		if err := c.reportV1(userTraffic, aliveData, onlineDeviceCount); err != nil {
			return nil
		}
	} else {
		if len(userTraffic) > 0 {
			c.addDynamicTraffic(userTraffic)
			log.WithField("tag", c.tag).Infof("Report %d users traffic", len(userTraffic))
		}
		if onlineDeviceCount > 0 {
			log.WithField("tag", c.tag).Infof("Total %d online users, %d Reported", onlineDeviceCount, len(aliveData))
		}
	}

	if len(aliveData) > 0 || reportErr == nil {
		c.limiter.MarkOnlineDeviceReported()
	}
	return nil
}

func (c *Controller) reportV1(userTraffic []panel.UserTraffic, aliveData map[int][]string, onlineDeviceCount int) error {
	if len(userTraffic) > 0 {
		err := c.apiClient.ReportUserTraffic(userTraffic)
		rollbackFailed := false
		if err != nil {
			log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			}).Info("Report user traffic failed")
			if rollbackErr := c.server.RestoreUserTraffic(c.tag, userTraffic); rollbackErr != nil {
				rollbackFailed = true
				log.WithFields(log.Fields{
					"tag": c.tag,
					"err": rollbackErr,
				}).Error("Rollback user traffic failed")
			}
		} else {
			c.addDynamicTraffic(userTraffic)
			log.WithField("tag", c.tag).Infof("Report %d users traffic", len(userTraffic))
		}
		if rollbackFailed {
			c.addDynamicTraffic(userTraffic)
		}
	}
	if len(aliveData) > 0 {
		if err := c.apiClient.ReportNodeOnlineUsers(&aliveData); err != nil {
			log.WithFields(log.Fields{
				"tag": c.tag,
				"err": err,
			}).Info("Report online users failed")
		} else {
			log.WithField("tag", c.tag).Infof("Total %d online users, %d Reported", onlineDeviceCount, len(aliveData))
			c.limiter.MarkOnlineDeviceReported()
		}
	}
	return nil
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
	uidToUUID := make(map[int]string, len(c.userList))
	for i := range c.userList {
		uidToUUID[c.userList[i].Id] = c.userList[i].Uuid
	}
	for i := range userTraffic {
		uuid, ok := uidToUUID[userTraffic[i].UID]
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
