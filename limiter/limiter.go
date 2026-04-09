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
	userLimitInfo sync.Map // taguuid -> *UserLimitInfo
	speedLimiter  sync.Map // taguuid -> *ratelimit.Bucket
	userOnlineIP  sync.Map // taguuid -> *sync.Map(ip -> uid)
	oldUserOnline map[string]int
	uuidToUID     sync.Map // uuid -> int
	aliveList     map[int]int
	dyWhitelist   map[int]struct{}
	dyTimeRanges  []timeRange
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
		oldUserOnline: make(map[string]int),
		aliveList:     aliveList,
	}
	if l.DynamicSpeedLimitConfig != nil {
		info.dyWhitelist = parseWhitelist(l.DynamicSpeedLimitConfig.DyLimitWhiteUserID)
		info.dyTimeRanges = parseTimeRanges(l.DynamicSpeedLimitConfig.DyLimitDuration)
	}
	for i := range users {
		info.uuidToUID.Store(users[i].Uuid, users[i].Id)
		ul := &UserLimitInfo{
			UID: users[i].Id,
		}
		if users[i].SpeedLimit != 0 {
			ul.SpeedLimit = users[i].SpeedLimit
		}
		if users[i].DeviceLimit != 0 {
			ul.DeviceLimit = users[i].DeviceLimit
		}
		info.userLimitInfo.Store(format.UserTag(tag, users[i].Uuid), ul)
	}
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
	for i := range deleted {
		key := format.UserTag(tag, deleted[i].Uuid)
		l.userLimitInfo.Delete(key)
		l.userOnlineIP.Delete(key)
		l.speedLimiter.Delete(key)
		l.uuidToUID.Delete(deleted[i].Uuid)
		l.mu.Lock()
		delete(l.aliveList, deleted[i].Id)
		l.mu.Unlock()
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
		l.userLimitInfo.Store(format.UserTag(tag, added[i].Uuid), ul)
		l.uuidToUID.Store(added[i].Uuid, added[i].Id)
	}
}

func (l *Limiter) IsWhitelisted(uid int) bool {
	if l.dyWhitelist == nil {
		return false
	}
	_, ok := l.dyWhitelist[uid]
	return ok
}

func (l *Limiter) InDynamicLimitTimeRange() bool {
	return inTimeRanges(l.dyTimeRanges, time.Now())
}

func (l *Limiter) IsDynamicLimited(tag, uuid string) bool {
	key := format.UserTag(tag, uuid)
	v, ok := l.userLimitInfo.Load(key)
	if !ok {
		return false
	}
	u := v.(*UserLimitInfo)
	return u.DynamicSpeedLimit > 0 && (u.ExpireTime == 0 || u.ExpireTime > time.Now().Unix())
}

func (l *Limiter) UpdateDynamicSpeedLimit(tag, uuid string, limit int, expire time.Time) error {
	key := format.UserTag(tag, uuid)
	v, ok := l.userLimitInfo.Load(key)
	if !ok {
		return errors.New("not found")
	}
	old := v.(*UserLimitInfo)
	info := *old
	info.DynamicSpeedLimit = limit
	info.ExpireTime = expire.Unix()
	l.userLimitInfo.Store(key, &info)
	return nil
}

func (l *Limiter) CheckLimit(taguuid string, ip string, isTcp bool, noSSUDP bool) (Bucket *ratelimit.Bucket, Reject bool) {
	ip = strings.TrimPrefix(ip, "::ffff:")

	nodeLimit := l.SpeedLimit
	userLimit := 0
	deviceLimit := 0
	var uid int

	v, ok := l.userLimitInfo.Load(taguuid)
	if !ok {
		return nil, true
	}
	u := v.(*UserLimitInfo)
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
		l.mu.RLock()
		aliveIp := l.aliveList[uid]
		l.mu.RUnlock()

		ipMapV, _ := l.userOnlineIP.LoadOrStore(taguuid, &sync.Map{})
		ipMap := ipMapV.(*sync.Map)
		if _, exists := ipMap.Load(ip); !exists {
			l.mu.RLock()
			oldUID, found := l.oldUserOnline[ip]
			l.mu.RUnlock()
			if found && oldUID == uid {
				l.mu.Lock()
				delete(l.oldUserOnline, ip)
				l.mu.Unlock()
				ipMap.Store(ip, uid)
			} else if deviceLimit > 0 && deviceLimit <= aliveIp {
				return nil, true
			} else {
				ipMap.Store(ip, uid)
			}
		}
	}

	if dynamicExpired {
		info := *u
		info.DynamicSpeedLimit = 0
		info.ExpireTime = 0
		l.userLimitInfo.Store(taguuid, &info)
		l.speedLimiter.Delete(taguuid)
	}

	limit := int64(determineSpeedLimit(nodeLimit, userLimit)) * 1000000 / 8
	if limit > 0 {
		if bv, ok := l.speedLimiter.Load(taguuid); ok {
			b := bv.(*ratelimit.Bucket)
			if b.Capacity() == limit {
				return b, false
			}
		}
		Bucket = ratelimit.NewBucketWithQuantum(time.Second, limit, limit)
		l.speedLimiter.Store(taguuid, Bucket)
		return Bucket, false
	}
	l.speedLimiter.Delete(taguuid)
	return nil, false
}

func (l *Limiter) SetAliveList(alive map[int]int) {
	l.mu.Lock()
	l.aliveList = alive
	l.mu.Unlock()
}

func (l *Limiter) GetUIDByUUID(uuid string) (int, bool) {
	v, ok := l.uuidToUID.Load(uuid)
	if !ok {
		return 0, false
	}
	return v.(int), true
}

func (l *Limiter) GetOnlineDevice() ([]panel.OnlineUser, error) {
	var onlineUser []panel.OnlineUser
	l.userOnlineIP.Range(func(_, value interface{}) bool {
		ipMap := value.(*sync.Map)
		ipMap.Range(func(ipKey, uidVal interface{}) bool {
			onlineUser = append(onlineUser, panel.OnlineUser{
				UID: uidVal.(int),
				IP:  ipKey.(string),
			})
			return true
		})
		return true
	})
	return onlineUser, nil
}

func (l *Limiter) MarkOnlineDeviceReported() {
	l.mu.Lock()
	l.oldUserOnline = make(map[string]int)
	l.mu.Unlock()

	l.userOnlineIP.Range(func(taguuid, value interface{}) bool {
		ipMap := value.(*sync.Map)
		ipMap.Range(func(ipKey, uidVal interface{}) bool {
			l.mu.Lock()
			l.oldUserOnline[ipKey.(string)] = uidVal.(int)
			l.mu.Unlock()
			return true
		})
		l.userOnlineIP.Delete(taguuid)
		return true
	})
}
