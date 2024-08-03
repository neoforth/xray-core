package limiter

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter struct {
	InboundInfo *sync.Map // Key: Tag, Value: *InboundInfo
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

type InboundInfo struct {
	Tag           string
	UserInfo      *sync.Map // Key: Email, Value: *UserInfo
	UserOnlineIPs *sync.Map // Key: Email, Value: *{Key: IP, Value: *{Key: int, Value: int64}}
	BucketHub     *sync.Map // Key: Email, Value: *rate.Limiter
}

type UserInfo struct {
	UID       uint32
	Email     string
	IPLimit   uint32
	RateLimit uint64
}

func New() *Limiter {
	limiter := &Limiter{
		InboundInfo: new(sync.Map),
		stopChan:    make(chan struct{}),
	}

	limiter.wg.Add(1)

	go limiter.startCleanupTask(5 * time.Minute)

	return limiter
}

func (limiter *Limiter) GetUserBucket(tag string, uid uint32, email string, ipLimit uint32, rateLimit uint64, ip string) (*rate.Limiter, bool, bool) {
	inboundInfoValue, _ := limiter.InboundInfo.LoadOrStore(tag, &InboundInfo{
		Tag:           tag,
		UserInfo:      new(sync.Map),
		UserOnlineIPs: new(sync.Map),
		BucketHub:     new(sync.Map),
	})
	inboundInfo := inboundInfoValue.(*InboundInfo)

	userInfoValue, _ := inboundInfo.UserInfo.LoadOrStore(email, &UserInfo{
		UID:       uid,
		Email:     email,
		IPLimit:   ipLimit,
		RateLimit: rateLimit,
	})
	userInfo := userInfoValue.(*UserInfo)

	userOnlineIPsValue, _ := inboundInfo.UserOnlineIPs.LoadOrStore(email, new(sync.Map))
	userOnlineIPs := userOnlineIPsValue.(*sync.Map)

	ipTimestampsValue, ipExists := userOnlineIPs.LoadOrStore(ip, new(sync.Map))
	ipTimestamps := ipTimestampsValue.(*sync.Map)

	timestamp := time.Now().Unix()
	ipTimestamps.Store(1, timestamp)

	if !ipExists {
		ipTimestamps.Store(0, timestamp)

		if ipLimit > 0 {
			var ipCount uint32
			userOnlineIPs.Range(func(_, _ interface{}) bool {
				ipCount++
				return true
			})

			if ipCount > ipLimit {
				userOnlineIPs.Delete(ip)
				return nil, false, true
			}
		}
	}

	if rateLimit > 0 {
		if rateLimiter, exists := inboundInfo.BucketHub.Load(email); exists {
			if userInfo.RateLimit == rateLimit {
				return rateLimiter.(*rate.Limiter), true, false
			}
			userInfo.RateLimit = rateLimit
		}
		bucket := rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit))
		inboundInfo.BucketHub.Store(email, bucket)
		return bucket, true, false
	}

	return nil, false, false
}

func (limiter *Limiter) startCleanupTask(interval time.Duration) {
	defer limiter.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			limiter.cleanUserOnlineIPs(30 * time.Minute)
			limiter.cleanBucketHub()
		case <-limiter.stopChan:
			return
		}
	}
}

func (limiter *Limiter) cleanUserOnlineIPs(timeout time.Duration) {
	expirationTime := time.Now().Add(-timeout).Unix()

	limiter.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)

		inboundInfo.UserOnlineIPs.Range(func(key, value interface{}) bool {
			email := key.(string)
			userOnlineIPs := value.(*sync.Map)

			userOnlineIPs.Range(func(key, value interface{}) bool {
				ip := key.(string)
				ipTimestamps := value.(*sync.Map)

				ipTimestamp, _ := ipTimestamps.Load(1)
				ipLastSeen := ipTimestamp.(int64)
				if ipLastSeen < expirationTime {
					userOnlineIPs.Delete(ip)
				}
				return true
			})

			var ipCount uint32
			userOnlineIPs.Range(func(_, _ interface{}) bool {
				ipCount++
				return true
			})

			if ipCount == 0 {
				inboundInfo.UserOnlineIPs.Delete(email)
			}
			return true
		})
		return true
	})
}

func (limiter *Limiter) cleanBucketHub() {
	limiter.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)

		inboundInfo.UserOnlineIPs.Range(func(key, value interface{}) bool {
			email := key.(string)
			userOnlineIPs := value.(*sync.Map)

			var ipCount uint32
			userOnlineIPs.Range(func(_, _ interface{}) bool {
				ipCount++
				return true
			})

			if ipCount == 0 {
				inboundInfo.BucketHub.Delete(email)
			}
			return true
		})
		return true
	})
}

func (limiter *Limiter) Stop() {
	close(limiter.stopChan)
	limiter.wg.Wait()
}
