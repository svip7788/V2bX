package limiter

import (
	"time"

	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/format"
)

func (l *Limiter) AddDynamicSpeedLimit(tag string, userInfo *panel.UserInfo, limitNum int, expire int64) error {
	key := format.UserTag(tag, userInfo.Uuid)
	l.mu.Lock()
	defer l.mu.Unlock()
	if old, ok := l.userLimitInfo[key]; ok {
		info := *old
		info.DynamicSpeedLimit = limitNum
		info.ExpireTime = time.Now().Add(time.Duration(expire) * time.Second).Unix()
		l.userLimitInfo[key] = &info
	} else {
		l.userLimitInfo[key] = &UserLimitInfo{
			DynamicSpeedLimit: limitNum,
			ExpireTime:        time.Now().Add(time.Duration(expire) * time.Second).Unix(),
		}
	}
	return nil
}

// determineSpeedLimit returns the minimum non-zero rate
func determineSpeedLimit(limit1, limit2 int) (limit int) {
	if limit1 == 0 || limit2 == 0 {
		if limit1 > limit2 {
			return limit1
		} else if limit1 < limit2 {
			return limit2
		} else {
			return 0
		}
	} else {
		if limit1 > limit2 {
			return limit2
		} else if limit1 < limit2 {
			return limit1
		} else {
			return limit1
		}
	}
}
