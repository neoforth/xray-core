// Package limiter is to control the links that go into the dispatcher
package limiter

import (
	"context"
	"sync"
	"sync/atomic"
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
	timestamp := time.Now().Unix()

	// Clean up expired IPs and count devices
	var deviceCount uint32
	var ipsToDelete []string

	ipMap.Range(func(key, value interface{}) bool {
		if timestamp-value.(int64) > 60 {
			ipsToDelete = append(ipsToDelete, key.(string))
		} else {
			atomic.AddUint32(&deviceCount, 1)
		}
		return true
	})

	// Delete expired IPs outside the Range loop to avoid modifying the map while iterating
	for _, ip := range ipsToDelete {
		ipMap.Delete(ip)
	}

	if deviceLimit > 0 && deviceCount >= deviceLimit {
		return nil, false, true
	}

	// Add current IP
	ipMap.Store(ip, timestamp)
	atomic.AddUint32(&deviceCount, 1) // Increment count for the new IP

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
