package limiter

import (
	"golang.org/x/time/rate"
	"sync"
)

var limiter *Limiter

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
	limiter = &Limiter{
		Inbounds: &sync.Map{},
	}
}

func Get() *Limiter {
	return limiter
}

func (l *Limiter) GetUserBucket(tag string, email string, rateLimit uint64) *rate.Limiter {
	inboundValue, _ := l.Inbounds.LoadOrStore(tag, &Inbound{
		Tag:   tag,
		Users: &sync.Map{},
	})
	inbound := inboundValue.(*Inbound)

	userValue, _ := inbound.Users.LoadOrStore(email, &User{
		Email:  email,
		Bucket: rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit)),
	})
	user := userValue.(*User)

	return user.Bucket
}

func (l *Limiter) RemoveInbound(tag string) {
	l.Inbounds.Delete(tag)
}

func (l *Limiter) RemoveUser(tag string, email string) {
	if inboundValue, found := l.Inbounds.Load(tag); found {
		inbound := inboundValue.(*Inbound)
		inbound.Users.Delete(email)
	}
}
