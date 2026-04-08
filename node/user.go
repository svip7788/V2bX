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
	if len(userTraffic) > 0 {
		err = c.apiClient.ReportUserTraffic(userTraffic)
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
			// Rollback failed means counters may not be restored, keep local dynamic stats.
			c.addDynamicTraffic(userTraffic)
		}
	}

	if onlineDevice, err := c.limiter.GetOnlineDevice(); err != nil {
		log.Print(err)
	} else if len(onlineDevice) > 0 {
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
		data := make(map[int][]string)
		reported := 0
		for _, online := range onlineDevice {
			if nocountUID != nil {
				if _, ok := nocountUID[online.UID]; ok {
					continue
				}
			}
			data[online.UID] = append(data[online.UID], online.IP)
			reported++
		}
		reportDone := false
		if len(data) > 0 {
			if err = c.apiClient.ReportNodeOnlineUsers(&data); err != nil {
				log.WithFields(log.Fields{
					"tag": c.tag,
					"err": err,
				}).Info("Report online users failed")
			} else {
				log.WithField("tag", c.tag).Infof("Total %d online users, %d Reported", len(onlineDevice), reported)
				reportDone = true
			}
		} else {
			reportDone = true
		}
		if reportDone {
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
	oldMap := make(map[userCompareKey]int)
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
