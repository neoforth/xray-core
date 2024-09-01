package limiter

import (
	"golang.org/x/time/rate"
	"sync"
)

var Manager *Limiter

type Limiter struct {
	Inbounds *sync.Map // Key: Tag, Value: *Inbound
}

type Inbound struct {
	Tag   string
	Users *sync.Map // Key: Email, Value: *User
}

type User struct {
	Email  string
	Bucket *rate.Limiter
}

func init() {
	Manager = New()
}

func New() *Limiter {
	return &Limiter{
		Inbounds: new(sync.Map),
	}
}

func (l *Limiter) GetUserBucket(tag string, email string, rateLimit uint64) *rate.Limiter {
	inboundValue, _ := l.Inbounds.LoadOrStore(tag, &Inbound{
		Tag:   tag,
		Users: new(sync.Map),
	})
	inbound := inboundValue.(*Inbound)

	userValue, _ := inbound.Users.LoadOrStore(email, &User{
		Email: email,
	})
	user := userValue.(*User)

	bucket := user.Bucket
	if bucket != nil {
		return bucket
	}
	bucket = rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit))
	user.Bucket = bucket
	return bucket
}

func (l *Limiter) RemoveInbound(tag string) {
	l.Inbounds.Delete(tag)
}

func (l *Limiter) RemoveUser(tag string, email string) {
	inboundValue, _ := l.Inbounds.Load(tag)
	inbound := inboundValue.(*Inbound)
	inbound.Users.Delete(email)
}
