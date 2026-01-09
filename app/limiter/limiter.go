package limiter

import (
	"golang.org/x/time/rate"
	"sync"
)

var instance *Limiter

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
	instance = &Limiter{
		Inbounds: &sync.Map{},
	}
}

func Get() *Limiter {
	return instance
}

func (l *Limiter) GetUserBucket(tag string, email string, rateLimit uint64) *rate.Limiter {
	ii, _ := l.Inbounds.LoadOrStore(tag, &Inbound{
		Tag:   tag,
		Users: &sync.Map{},
	})
	i := ii.(*Inbound)

	ui, _ := i.Users.LoadOrStore(email, &User{
		Email:  email,
		Bucket: rate.NewLimiter(rate.Limit(rateLimit), int(rateLimit * 3)),
	})
	u := ui.(*User)

	return u.Bucket
}

func (l *Limiter) RemoveInbound(tag string) {
	l.Inbounds.Delete(tag)
}

func (l *Limiter) RemoveUser(tag string, email string) {
	if ii, ok := l.Inbounds.Load(tag); ok {
		i := ii.(*Inbound)
		i.Users.Delete(email)
	}
}
