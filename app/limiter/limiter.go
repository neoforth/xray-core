package limiter

import (
	"math"
	"sort"
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
	UserOnlineIPs *sync.Map // Key: Email, Value: *{Key: IP, Value: *IPStatus}
	BucketHub     *sync.Map // Key: Email, Value: *rate.Limiter
}

type UserInfo struct {
	UID       uint32
	Email     string
	IPLimit   uint32
	RateLimit uint64
}

type IPStatus struct {
	AccessCount   uint64
	FirstSeen     int64
	LastSeen      int64
	ActivityScore float64
	mu            sync.RWMutex
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

	ipMappingsValue, _ := inboundInfo.UserOnlineIPs.LoadOrStore(email, new(sync.Map))
	ipMappings := ipMappingsValue.(*sync.Map)

	ipStatusValue, ipExists := ipMappings.LoadOrStore(ip, &IPStatus{
		AccessCount:   0,
		FirstSeen:     0,
		LastSeen:      0,
		ActivityScore: 0,
	})
	ipStatus := ipStatusValue.(*IPStatus)

	timestamp := time.Now().Unix()

	ipStatus.mu.Lock()
	ipStatus.AccessCount++
	ipStatus.LastSeen = timestamp
	ipStatus.mu.Unlock()

	if !ipExists {
		ipStatus.mu.Lock()
		ipStatus.FirstSeen = timestamp
		ipStatus.mu.Unlock()

		if ipLimit > 0 {
			var ipCount uint32

			ipMappings.Range(func(_, _ interface{}) bool {
				ipCount++
				return true
			})

			if userInfo.IPLimit != ipLimit {
				userInfo.IPLimit = ipLimit
			}

			if ipCount > ipLimit {
				ipMappings.Delete(ip)

				return nil, false, true
			}
		}
	}

	if rateLimit > 0 {
		if rateLimiter, emailExists := inboundInfo.BucketHub.Load(email); emailExists {
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
			limiter.cleanUserOnlineIPs(5 * time.Minute)
			limiter.cleanBucketHub()
		case <-limiter.stopChan:
			return
		}
	}
}

func (limiter *Limiter) cleanUserOnlineIPs(timeout time.Duration) {
	now := time.Now().Unix()
	windowSize := int64(timeout.Seconds())

	limiter.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)

		var allScores []float64

		inboundInfo.UserOnlineIPs.Range(func(_, value interface{}) bool {
			ipMappings := value.(*sync.Map)

			ipMappings.Range(func(_, value interface{}) bool {
				ipStatus := value.(*IPStatus)

				ipStatus.mu.RLock()
				accessCount := ipStatus.AccessCount
				timeSinceFirstSeen := now - ipStatus.FirstSeen
				timeSinceLastSeen := now - ipStatus.LastSeen
				ipStatus.mu.RUnlock()

				activityScore := float64(accessCount) / float64(timeSinceFirstSeen)
				decayFactor := math.Exp(-float64(timeSinceLastSeen) / float64(windowSize))

				ipActivityScore := activityScore * decayFactor

				ipStatus.mu.Lock()
				ipStatus.ActivityScore = ipActivityScore
				ipStatus.mu.Unlock()

				allScores = append(allScores, ipActivityScore)

				return true
			})

			return true
		})

		threshold := calculateThreshold(allScores)

		var emailsToDelete []string

		inboundInfo.UserOnlineIPs.Range(func(key, value interface{}) bool {
			email := key.(string)
			ipMappings := value.(*sync.Map)

			var ipsToDelete []string

			ipMappings.Range(func(key, value interface{}) bool {
				ip := key.(string)
				ipStatus := value.(*IPStatus)

				ipStatus.mu.RLock()
				timeSinceLastSeen := now - ipStatus.LastSeen
				ipActivityScore := ipStatus.ActivityScore
				ipStatus.mu.RUnlock()

				if timeSinceLastSeen > windowSize && ipActivityScore < threshold {
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
			inboundInfo.UserOnlineIPs.Delete(email)
		}

		return true
	})
}

func (limiter *Limiter) cleanBucketHub() {
	limiter.InboundInfo.Range(func(_, value interface{}) bool {
		inboundInfo := value.(*InboundInfo)

		var emailsToDelete []string

		inboundInfo.UserOnlineIPs.Range(func(key, value interface{}) bool {
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
			inboundInfo.BucketHub.Delete(email)
		}

		return true
	})
}

func (limiter *Limiter) Stop() {
	close(limiter.stopChan)
	limiter.wg.Wait()
}

func calculateThreshold(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}

	sort.Float64s(scores)

	var median float64
	num := len(scores)
	if num%2 == 0 {
		median = (scores[num/2-1] + scores[num/2]) / 2
	} else {
		median = scores[num/2]
	}

	if median < 0 {
		return 0
	}

	return median
}
