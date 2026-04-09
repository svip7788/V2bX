package limiter

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/format"
	"github.com/InazumaV/V2bX/conf"
	"github.com/juju/ratelimit"
)

var limitLock sync.RWMutex
var limiter map[string]*Limiter

func Init() {
	limiter = map[string]*Limiter{}
}

type Limiter struct {
	DomainRules   []*regexp.Regexp
	ProtocolRules []string
	SpeedLimit    int

	mu            sync.RWMutex
	userLimitInfo map[string]*UserLimitInfo
	speedLimiter  map[string]*ratelimit.Bucket
	userOnlineIP  map[string]map[string]int // taguuid -> (ip -> uid)
	oldUserOnline map[string]int            // ip -> uid
	uuidToUID     map[string]int
	aliveList     map[int]int
}

type UserLimitInfo struct {
	UID               int
	SpeedLimit        int
	DeviceLimit       int
	DynamicSpeedLimit int
	ExpireTime        int64
	OverLimit         bool
}

func AddLimiter(tag string, l *conf.LimitConfig, users []panel.UserInfo, aliveList map[int]int) *Limiter {
	info := &Limiter{
		SpeedLimit:    l.SpeedLimit,
		userLimitInfo: make(map[string]*UserLimitInfo, len(users)),
		speedLimiter:  make(map[string]*ratelimit.Bucket),
		userOnlineIP:  make(map[string]map[string]int),
		oldUserOnline: make(map[string]int),
		aliveList:     aliveList,
	}
	uuidmap := make(map[string]int, len(users))
	for i := range users {
		uuidmap[users[i].Uuid] = users[i].Id
		ul := &UserLimitInfo{
			UID: users[i].Id,
		}
		if users[i].SpeedLimit != 0 {
			ul.SpeedLimit = users[i].SpeedLimit
		}
		if users[i].DeviceLimit != 0 {
			ul.DeviceLimit = users[i].DeviceLimit
		}
		info.userLimitInfo[format.UserTag(tag, users[i].Uuid)] = ul
	}
	info.uuidToUID = uuidmap
	limitLock.Lock()
	limiter[tag] = info
	limitLock.Unlock()
	return info
}

func GetLimiter(tag string) (info *Limiter, err error) {
	limitLock.RLock()
	info, ok := limiter[tag]
	limitLock.RUnlock()
	if !ok {
		return nil, errors.New("not found")
	}
	return info, nil
}

func DeleteLimiter(tag string) {
	limitLock.Lock()
	delete(limiter, tag)
	limitLock.Unlock()
}

func (l *Limiter) UpdateUser(tag string, added []panel.UserInfo, deleted []panel.UserInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range deleted {
		key := format.UserTag(tag, deleted[i].Uuid)
		delete(l.userLimitInfo, key)
		delete(l.userOnlineIP, key)
		delete(l.speedLimiter, key)
		delete(l.uuidToUID, deleted[i].Uuid)
		delete(l.aliveList, deleted[i].Id)
	}
	for i := range added {
		ul := &UserLimitInfo{
			UID: added[i].Id,
		}
		if added[i].SpeedLimit != 0 {
			ul.SpeedLimit = added[i].SpeedLimit
			ul.ExpireTime = 0
		}
		if added[i].DeviceLimit != 0 {
			ul.DeviceLimit = added[i].DeviceLimit
		}
		l.userLimitInfo[format.UserTag(tag, added[i].Uuid)] = ul
		l.uuidToUID[added[i].Uuid] = added[i].Id
	}
}

func (l *Limiter) UpdateDynamicSpeedLimit(tag, uuid string, limit int, expire time.Time) error {
	key := format.UserTag(tag, uuid)
	l.mu.Lock()
	defer l.mu.Unlock()
	if old, ok := l.userLimitInfo[key]; ok {
		info := *old
		info.DynamicSpeedLimit = limit
		info.ExpireTime = expire.Unix()
		l.userLimitInfo[key] = &info
	} else {
		return errors.New("not found")
	}
	return nil
}

func (l *Limiter) CheckLimit(taguuid string, ip string, isTcp bool, noSSUDP bool) (Bucket *ratelimit.Bucket, Reject bool) {
	ip = strings.TrimPrefix(ip, "::ffff:")

	nodeLimit := l.SpeedLimit
	userLimit := 0
	deviceLimit := 0
	var uid int

	l.mu.RLock()
	u, ok := l.userLimitInfo[taguuid]
	if !ok {
		l.mu.RUnlock()
		return nil, true
	}
	deviceLimit = u.DeviceLimit
	uid = u.UID
	dynamicExpired := u.ExpireTime != 0 && u.ExpireTime < time.Now().Unix()
	if dynamicExpired {
		userLimit = u.SpeedLimit
	} else if u.DynamicSpeedLimit > 0 {
		userLimit = determineSpeedLimit(u.SpeedLimit, u.DynamicSpeedLimit)
	} else {
		userLimit = u.SpeedLimit
	}

	if noSSUDP {
		aliveIp := l.aliveList[uid]
		ipMap := l.userOnlineIP[taguuid]
		var isNewIP bool
		if ipMap != nil {
			if _, exists := ipMap[ip]; !exists {
				isNewIP = true
			}
		} else {
			isNewIP = true
		}
		l.mu.RUnlock()

		if isNewIP {
			l.mu.Lock()
			if l.userOnlineIP[taguuid] == nil {
				l.userOnlineIP[taguuid] = make(map[string]int)
			}
			if _, exists := l.userOnlineIP[taguuid][ip]; !exists {
				if oldUID, found := l.oldUserOnline[ip]; found && oldUID == uid {
					delete(l.oldUserOnline, ip)
					l.userOnlineIP[taguuid][ip] = uid
				} else if deviceLimit > 0 && deviceLimit <= aliveIp {
					l.mu.Unlock()
					return nil, true
				} else {
					l.userOnlineIP[taguuid][ip] = uid
				}
			}
			l.mu.Unlock()
		}
	} else {
		l.mu.RUnlock()
	}

	if dynamicExpired {
		l.mu.Lock()
		if info, ok := l.userLimitInfo[taguuid]; ok {
			cleared := *info
			cleared.DynamicSpeedLimit = 0
			cleared.ExpireTime = 0
			l.userLimitInfo[taguuid] = &cleared
		}
		delete(l.speedLimiter, taguuid)
		l.mu.Unlock()
	}

	limit := int64(determineSpeedLimit(nodeLimit, userLimit)) * 1000000 / 8
	if limit > 0 {
		l.mu.RLock()
		b := l.speedLimiter[taguuid]
		l.mu.RUnlock()
		if b != nil && b.Capacity() == limit {
			return b, false
		}
		Bucket = ratelimit.NewBucketWithQuantum(time.Second, limit, limit)
		l.mu.Lock()
		l.speedLimiter[taguuid] = Bucket
		l.mu.Unlock()
		return Bucket, false
	}
	l.mu.Lock()
	delete(l.speedLimiter, taguuid)
	l.mu.Unlock()
	return nil, false
}

func (l *Limiter) SetAliveList(alive map[int]int) {
	l.mu.Lock()
	l.aliveList = alive
	l.mu.Unlock()
}

func (l *Limiter) GetUIDByUUID(uuid string) (int, bool) {
	l.mu.RLock()
	uid, ok := l.uuidToUID[uuid]
	l.mu.RUnlock()
	return uid, ok
}

func (l *Limiter) GetOnlineDevice() ([]panel.OnlineUser, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	var onlineUser []panel.OnlineUser
	for _, ipMap := range l.userOnlineIP {
		for ip, uid := range ipMap {
			onlineUser = append(onlineUser, panel.OnlineUser{UID: uid, IP: ip})
		}
	}
	return onlineUser, nil
}

func (l *Limiter) MarkOnlineDeviceReported() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.oldUserOnline = make(map[string]int, len(l.userOnlineIP)*2)
	for taguuid, ipMap := range l.userOnlineIP {
		for ip, uid := range ipMap {
			l.oldUserOnline[ip] = uid
		}
		delete(l.userOnlineIP, taguuid)
	}
}
