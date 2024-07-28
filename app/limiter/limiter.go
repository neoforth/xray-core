// Package limiter is to control the links that go into the dispatcher
package limiter

import (
	"context"
	"sync"

	"github.com/xtls/xray-core/common/errors"
	"golang.org/x/time/rate"
)

type InboundInfo struct {
	Tag           string
	UserOnlineIPs *sync.Map // Key: Email, value: {Key: IP, value: UID}
	BucketHub     *sync.Map // key: Email, value: *rate.Limiter
}

type Limiter struct {
	InboundInfo *sync.Map // Key: Tag, Value: *InboundInfo
}

func New() *Limiter {
	return &Limiter{
		InboundInfo: new(sync.Map),
	}
}

func (l *Limiter) GetUserBucket(tag string, uid uint32, email string, deviceLimit uint32, speedLimit uint64, ip string) (*rate.Limiter, bool, bool) {
	value, _ := l.InboundInfo.LoadOrStore(tag, &InboundInfo{
		Tag:           tag,
		UserOnlineIPs: new(sync.Map),
		BucketHub:     new(sync.Map),
	})
	inboundInfo := value.(*InboundInfo)

	// Local device limit
	userDevices, _ := inboundInfo.UserOnlineIPs.LoadOrStore(email, new(sync.Map))
	ipMap := userDevices.(*sync.Map)
	if _, loaded := ipMap.LoadOrStore(ip, uid); !loaded {
		var deviceCount uint32
		ipMap.Range(func(_, _ interface{}) bool {
			deviceCount++
			return true
		})
		if deviceCount > deviceLimit && deviceLimit > 0 {
			ipMap.Delete(ip)
			return nil, false, true
		}
	}

	if speedLimit > 0 {
		limiter := rate.NewLimiter(rate.Limit(speedLimit), int(speedLimit))
		if v, ok := inboundInfo.BucketHub.LoadOrStore(email, limiter); ok {
			bucket := v.(*rate.Limiter)
			return bucket, true, false
		}
		return limiter, true, false
	}

	errors.LogDebug(context.Background(), "Failed to get or create limiter")
	return nil, false, false
}
