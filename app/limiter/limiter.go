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
	l := &Limiter{
		InboundInfo: new(sync.Map),
		stopChan:    make(chan struct{}),
	}

	l.wg.Add(1) // Add a count to the WaitGroup for the cleanup task goroutine

	go l.startCleanupTask(5 * time.Minute)

	return l
}

func (l *Limiter) GetUserBucket(tag string, uid uint32, email string, ipLimit uint32, rateLimit uint64, ip string) (*rate.Limiter, bool, bool) {
	// Load or create InboundInfo
	v, _ := l.InboundInfo.LoadOrStore(tag, &InboundInfo{
		Tag:           tag,
		UserInfo:      new(sync.Map),
		UserOnlineIPs: new(sync.Map),
		BucketHub:     new(sync.Map),
	})
	inboundInfo := v.(*InboundInfo)

	// Load or create UserInfo
	v2, _ := inboundInfo.UserInfo.LoadOrStore(email, &UserInfo{
		UID:       uid,
		Email:     email,
		IPLimit:   ipLimit,
		RateLimit: rateLimit,
	})
	userInfo := v2.(*UserInfo)

	// Load or create ipMappings
	v3, _ := inboundInfo.UserOnlineIPs.LoadOrStore(email, new(sync.Map))
	ipMappings := v3.(*sync.Map)

	// Load or create ipTimestamps
	v4, ok := ipMappings.LoadOrStore(ip, new(sync.Map))
	ipTimestamps := v4.(*sync.Map)

	// Store the last seen timestamp for this IP
	timestamp := time.Now().Unix()
	ipTimestamps.Store(1, timestamp)

	// Store the first seen timestamp for this IP if first time connecting
	if !ok {
		ipTimestamps.Store(0, timestamp)

		// Manage IP limits
		if ipLimit > 0 {
			var ipCount uint32
			ipMappings.Range(func(_, _ interface{}) bool {
				ipCount++
				return true
			})

			if ipCount > ipLimit {
				ipMappings.Delete(ip)
				return nil, false, true
			}
		}
	}

	// Manage rate limits
	if rateLimit > 0 {
		if rateLimiter, ok := inboundInfo.BucketHub.Load(email); ok {
			if userInfo.RateLimit == rateLimit {
				bucket := rateLimiter.(*rate.Limiter)
				return bucket, true, false
			}
			userInfo.RateLimit = rateLimit
			inboundInfo.BucketHub.Delete(email)
		}
		bucket := rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit))
		inboundInfo.BucketHub.Store(email, bucket)
		return bucket, true, false
	}

	return nil, false, false
}

func (l *Limiter) startCleanupTask(interval time.Duration) {
	defer l.wg.Done() // Decrease the count in the WaitGroup when the goroutine finishes

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanUserOnlineIPs(30 * time.Minute)
			l.cleanBucketHub()
		case <-l.stopChan:
			return
		}
	}
}

func (l *Limiter) cleanUserOnlineIPs(timeout time.Duration) {
	// Calculate the expiration time
	expirationTime := time.Now().Add(-timeout).Unix()

	// Iterate over each tag in InboundInfo
	l.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)

		// Iterate over each email in UserOnlineIPs
		inboundInfo.UserOnlineIPs.Range(func(key, value interface{}) bool {
			email := key.(string)
			ipMappings := value.(*sync.Map)

			// Iterate over each IP in ipMappings
			ipMappings.Range(func(key, value interface{}) bool {
				ip := key.(string)
				ipTimestamps := value.(*sync.Map)

				// Delete the IP entry if expired
				ipTimestamp, _ := ipTimestamps.Load(1)
				ipLastSeen := ipTimestamp.(int64)
				if ipLastSeen < expirationTime {
					ipMappings.Delete(ip)
				}
				return true
			})

			// Count the number of IPs for this email
			var ipCount uint32
			ipMappings.Range(func(_, _ interface{}) bool {
				ipCount++
				return true
			})

			// If no IPs under this email, delete the email entry
			if ipCount == 0 {
				inboundInfo.UserOnlineIPs.Delete(email)
			}
			return true
		})
		return true
	})
}

func (l *Limiter) cleanBucketHub() {
	// Iterate over each tag in InboundInfo
	l.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)

		// Iterate over each email in UserOnlineIPs
		inboundInfo.UserOnlineIPs.Range(func(key, value interface{}) bool {
			email := key.(string)
			ipMappings := value.(*sync.Map)

			// Count the number of IPs for this email
			var ipCount uint32
			ipMappings.Range(func(_, _ interface{}) bool {
				ipCount++
				return true
			})

			// If no IPs under this email, delete the rate limiter
			if ipCount == 0 {
				inboundInfo.BucketHub.Delete(email)
			}
			return true
		})
		return true
	})
}

func (l *Limiter) Stop() {
	close(l.stopChan)
	l.wg.Wait() // Wait for all goroutines to finish
}
