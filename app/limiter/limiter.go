package limiter

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type InboundInfo struct {
	Tag           string
	UserOnlineIPs *sync.Map // Key: Email, value: {Key: IP, value: UID}
	BucketHub     *sync.Map // key: Email, value: *rate.Limiter
}

type Limiter struct {
	InboundInfo *sync.Map // Key: Tag, Value: *InboundInfo
	stopChan    chan struct{}
}

func New() *Limiter {
	l := &Limiter{
		InboundInfo: new(sync.Map),
		stopChan:    make(chan struct{}),
	}
	go l.startCleanupTask()
	return l
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

	return nil, false, false
}

func (l *Limiter) startCleanupTask() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanUserOnlineIPs()
			l.cleanBucketHub()
		case <-l.stopChan:
			return
		}
	}
}

func (l *Limiter) cleanUserOnlineIPs() {
	l.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)
		inboundInfo.UserOnlineIPs.Range(func(key, _ interface{}) bool {
			inboundInfo.UserOnlineIPs.Delete(key)
			return true
		})
		return true
	})
}

func (l *Limiter) cleanBucketHub() {
	l.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)
		inboundInfo.BucketHub.Range(func(key, _ interface{}) bool {
			inboundInfo.BucketHub.Delete(key)
			return true
		})
		return true
	})
}

func (l *Limiter) Stop() {
	close(l.stopChan)
}
