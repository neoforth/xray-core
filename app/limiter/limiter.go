// Package limiter is to control the links that go into the dispatcher
package limiter

import (
	"context"
	"sync"
	"time"

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
	limiter := &Limiter{
		InboundInfo: new(sync.Map),
	}

	// Start a goroutine to clear UserOnlineIPs and BucketHub every 60 seconds
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			limiter.InboundInfo.Range(func(_, value interface{}) bool {
				inboundInfo := value.(*InboundInfo)
				/*
				inboundInfo.UserOnlineIPs.Range(func(key, _ interface{}) bool {
					inboundInfo.UserOnlineIPs.Delete(key)
					return true
				})
    				*/
				inboundInfo.BucketHub.Range(func(key, _ interface{}) bool {
					inboundInfo.BucketHub.Delete(key)
					return true
				})
				return true
			})
		}
	}()

	return limiter
}

func (l *Limiter) GetUserBucket(tag string, uid uint32, email string, deviceLimit uint32, speedLimit uint64, ip string) (*rate.Limiter, bool, bool) {
	value, _ := l.InboundInfo.LoadOrStore(tag, &InboundInfo{
		Tag:           tag,
		UserOnlineIPs: new(sync.Map),
		BucketHub:     new(sync.Map),
	})
	inboundInfo := value.(*InboundInfo)

	/* Local device limit
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
 	*/

	// Speed limit
	if speedLimit > 0 {
		if v, ok := inboundInfo.BucketHub.Load(email); ok {
			bucket := v.(*rate.Limiter)
			return bucket, true, false
		}
		limiter := rate.NewLimiter(rate.Limit(speedLimit), int(speedLimit))
		inboundInfo.BucketHub.Store(email, limiter)
		return limiter, true, false
	}

	errors.LogDebug(context.Background(), "Failed to get or create limiter")
	return nil, false, false
}
