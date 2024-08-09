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
	Tag          string
	UserInfo     *sync.Map // Key: Email, Value: *UserInfo
	OnlineIPs    *sync.Map // Key: Email, Value: *{Key: IP, Value: *IPStatus}
	TokenBuckets *sync.Map // Key: Email, Value: *rate.Limiter
}

type UserInfo struct {
	ID        uint32
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

	go limiter.startCleanupTask(1 * time.Minute)

	return limiter
}

func (limiter *Limiter) GetUserBucket(tag string, id uint32, email string, ipLimit uint32, rateLimit uint64, ip string) (*rate.Limiter, bool, bool) {
	inboundInfoValue, _ := limiter.InboundInfo.LoadOrStore(tag, &InboundInfo{
		Tag:          tag,
		UserInfo:     new(sync.Map),
		OnlineIPs:    new(sync.Map),
		TokenBuckets: new(sync.Map),
	})
	inboundInfo := inboundInfoValue.(*InboundInfo)

	ipMappingsValue, _ := inboundInfo.OnlineIPs.LoadOrStore(email, new(sync.Map))
	ipMappings := ipMappingsValue.(*sync.Map)

	_, ipExists := ipMappings.LoadOrStore(ip, new(int64))

	// IP access
	timestamp := time.Now().Unix()
	ipMappings.Store(ip, timestamp)

	// IP limit
	if !ipExists {
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

	// Rate limit
	if rateLimit > 0 {
		if rateLimiter, emailExists := inboundInfo.TokenBuckets.Load(email); emailExists {
			return rateLimiter.(*rate.Limiter), true, false
		}

		bucket := rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit))
		inboundInfo.TokenBuckets.Store(email, bucket)

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
			limiter.cleanOnlineIPs(5 * time.Minute)
			limiter.cleanTokenBuckets()
		case <-limiter.stopChan:
			return
		}
	}
}

func (limiter *Limiter) cleanOnlineIPs(timeout time.Duration) {
	expirationTime := time.Now().Add(-timeout).Unix()

	limiter.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)

		var emailsToDelete []string

		inboundInfo.OnlineIPs.Range(func(key, value interface{}) bool {
			email := key.(string)
			ipMappings := value.(*sync.Map)

			var ipsToDelete []string

			ipMappings.Range(func(key, value interface{}) bool {
				ip := key.(string)
				ipTimestamp := value.(int64)

				if ipTimestamp < expirationTime {
					ipsToDelete = append(ipsToDelete, ip)
				}

				return true
			})

			for _, ip := range ipsToDelete {
				ipMappings.Delete(ip)
			}

			var ipCount uint32

			ipMappings.Range(func(_, _ interface{}) bool {
				ipCount++
				return true
			})

			if ipCount == 0 {
				emailsToDelete = append(emailsToDelete, email)
			}

			return true
		})

		for _, email := range emailsToDelete {
			inboundInfo.OnlineIPs.Delete(email)
		}

		return true
	})
}

func (limiter *Limiter) cleanTokenBuckets() {
	limiter.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)

		var emailsToDelete []string

		inboundInfo.OnlineIPs.Range(func(key, value interface{}) bool {
			email := key.(string)
			ipMappings := value.(*sync.Map)

			var ipCount uint32

			ipMappings.Range(func(_, _ interface{}) bool {
				ipCount++
				return true
			})

			if ipCount == 0 {
				emailsToDelete = append(emailsToDelete, email)
			}

			return true
		})

		for _, email := range emailsToDelete {
			inboundInfo.TokenBuckets.Delete(email)
		}

		return true
	})
}

func (limiter *Limiter) Stop() {
	close(limiter.stopChan)
	limiter.wg.Wait()
}
