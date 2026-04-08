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
	UserOnlineIP  *sync.Map      // Key: TagUUID, value: {Key: Ip, value: Uid}
	OldUserOnline *sync.Map      // Key: Ip, value: Uid
	UUIDtoUID     map[string]int // Key: UUID, value: Uid
	UserLimitInfo *sync.Map      // Key: TagUUID value: UserLimitInfo
	SpeedLimiter  *sync.Map      // key: TagUUID, value: *ratelimit.Bucket
	AliveList     map[int]int    // Key: Uid, value: alive_ip
	mu            sync.RWMutex   // protects UUIDtoUID and AliveList
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
		UserOnlineIP:  new(sync.Map),
		UserLimitInfo: new(sync.Map),
		SpeedLimiter:  new(sync.Map),
		AliveList:     aliveList,
		OldUserOnline: new(sync.Map),
	}
	uuidmap := make(map[string]int)
	for i := range users {
		uuidmap[users[i].Uuid] = users[i].Id
		userLimit := &UserLimitInfo{}
		userLimit.UID = users[i].Id
		if users[i].SpeedLimit != 0 {
			userLimit.SpeedLimit = users[i].SpeedLimit
		}
		if users[i].DeviceLimit != 0 {
			userLimit.DeviceLimit = users[i].DeviceLimit
		}
		userLimit.OverLimit = false
		info.UserLimitInfo.Store(format.UserTag(tag, users[i].Uuid), userLimit)
	}
	info.UUIDtoUID = uuidmap
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
		l.UserLimitInfo.Delete(format.UserTag(tag, deleted[i].Uuid))
		l.UserOnlineIP.Delete(format.UserTag(tag, deleted[i].Uuid))
		l.SpeedLimiter.Delete(format.UserTag(tag, deleted[i].Uuid))
		delete(l.UUIDtoUID, deleted[i].Uuid)
		delete(l.AliveList, deleted[i].Id)
	}
	for i := range added {
		userLimit := &UserLimitInfo{
			UID: added[i].Id,
		}
		if added[i].SpeedLimit != 0 {
			userLimit.SpeedLimit = added[i].SpeedLimit
			userLimit.ExpireTime = 0
		}
		if added[i].DeviceLimit != 0 {
			userLimit.DeviceLimit = added[i].DeviceLimit
		}
		userLimit.OverLimit = false
		l.UserLimitInfo.Store(format.UserTag(tag, added[i].Uuid), userLimit)
		l.UUIDtoUID[added[i].Uuid] = added[i].Id
	}
}

func (l *Limiter) UpdateDynamicSpeedLimit(tag, uuid string, limit int, expire time.Time) error {
	key := format.UserTag(tag, uuid)
	l.mu.Lock()
	defer l.mu.Unlock()
	if v, ok := l.UserLimitInfo.Load(key); ok {
		oldInfo := v.(*UserLimitInfo)
		info := *oldInfo
		info.DynamicSpeedLimit = limit
		info.ExpireTime = expire.Unix()
		l.UserLimitInfo.Store(key, &info)
	} else {
		return errors.New("not found")
	}
	return nil
}

func (l *Limiter) CheckLimit(taguuid string, ip string, isTcp bool, noSSUDP bool) (Bucket *ratelimit.Bucket, Reject bool) {
	// check if ipv4 mapped ipv6
	ip = strings.TrimPrefix(ip, "::ffff:")

	// check and gen speed limit Bucket
	nodeLimit := l.SpeedLimit
	userLimit := 0
	deviceLimit := 0
	var uid int
	if v, ok := l.UserLimitInfo.Load(taguuid); ok {
		u := *(v.(*UserLimitInfo))
		deviceLimit = u.DeviceLimit
		uid = u.UID
		if u.ExpireTime < time.Now().Unix() && u.ExpireTime != 0 {
			l.SpeedLimiter.Delete(taguuid)
			userLimit = u.SpeedLimit
		} else {
			userLimit = determineSpeedLimit(u.SpeedLimit, u.DynamicSpeedLimit)
		}
	} else {
		return nil, true
	}
	if noSSUDP {
		l.mu.RLock()
		aliveIp := l.AliveList[uid]
		l.mu.RUnlock()
		if v, ok := l.UserOnlineIP.Load(taguuid); ok {
			oldipMap := v.(*sync.Map)
			if _, loaded := oldipMap.LoadOrStore(ip, uid); !loaded {
				if v, loaded := l.OldUserOnline.Load(ip); loaded {
					if v.(int) == uid {
						l.OldUserOnline.Delete(ip)
					}
				} else if deviceLimit > 0 && deviceLimit <= aliveIp {
					oldipMap.Delete(ip)
					return nil, true
				}
			}
		} else {
			newipMap := new(sync.Map)
			newipMap.Store(ip, uid)
			if actual, loaded := l.UserOnlineIP.LoadOrStore(taguuid, newipMap); loaded {
				existingMap := actual.(*sync.Map)
				if _, stored := existingMap.LoadOrStore(ip, uid); !stored {
					if v, ok := l.OldUserOnline.Load(ip); ok {
						if v.(int) == uid {
							l.OldUserOnline.Delete(ip)
						}
					} else if deviceLimit > 0 && deviceLimit <= aliveIp {
						existingMap.Delete(ip)
						return nil, true
					}
				}
			} else if v, ok := l.OldUserOnline.Load(ip); ok {
				if v.(int) == uid {
					l.OldUserOnline.Delete(ip)
				}
			} else if deviceLimit > 0 && deviceLimit <= aliveIp {
				l.UserOnlineIP.Delete(taguuid)
				return nil, true
			}
		}
	}

	limit := int64(determineSpeedLimit(nodeLimit, userLimit)) * 1000000 / 8
	if limit > 0 {
		if v, ok := l.SpeedLimiter.Load(taguuid); ok {
			b := v.(*ratelimit.Bucket)
			if b.Capacity() == limit {
				return b, false
			}
			// limit changed, replace bucket
		}
		Bucket = ratelimit.NewBucketWithQuantum(time.Second, limit, limit)
		l.SpeedLimiter.Store(taguuid, Bucket)
		return Bucket, false
	}
	l.SpeedLimiter.Delete(taguuid)
	return nil, false
}

func (l *Limiter) SetAliveList(alive map[int]int) {
	l.mu.Lock()
	l.AliveList = alive
	l.mu.Unlock()
}

func (l *Limiter) GetUIDByUUID(uuid string) (int, bool) {
	l.mu.RLock()
	uid, ok := l.UUIDtoUID[uuid]
	l.mu.RUnlock()
	return uid, ok
}

func (l *Limiter) GetOnlineDevice() ([]panel.OnlineUser, error) {
	var onlineUser []panel.OnlineUser
	l.UserOnlineIP.Range(func(key, value interface{}) bool {
		ipMap := value.(*sync.Map)
		ipMap.Range(func(key, value interface{}) bool {
			uid := value.(int)
			ip := key.(string)
			onlineUser = append(onlineUser, panel.OnlineUser{UID: uid, IP: ip})
			return true
		})
		return true
	})
	return onlineUser, nil
}

func (l *Limiter) MarkOnlineDeviceReported() {
	l.OldUserOnline.Range(func(key, value interface{}) bool {
		l.OldUserOnline.Delete(key)
		return true
	})
	l.UserOnlineIP.Range(func(key, value interface{}) bool {
		taguuid := key.(string)
		ipMap := value.(*sync.Map)
		ipMap.Range(func(key, value interface{}) bool {
			uid := value.(int)
			ip := key.(string)
			l.OldUserOnline.Store(ip, uid)
			return true
		})
		l.UserOnlineIP.Delete(taguuid)
		return true
	})
}

type UserIpList struct {
	Uid    int      `json:"Uid"`
	IpList []string `json:"Ips"`
}
