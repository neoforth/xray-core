// Package limiter is to control the links that go into the dispather
package limiter

import (
	//"golang.org/x/time/rate"
	"sync"
)

type Limiter struct {
	UserOnlineIPs   *sync.Map // Key: Email, value: {Key: IP, value: UID}
}

func New() *Limiter {
	return &Limiter{
		UserOnlineIPs: new(sync.Map),
	}
}

func (l *Limiter) CheckDeviceLimit(uid uint32, email string, deviceLimit uint32, ip string) bool {

		// Local device limit
		ipMap := new(sync.Map)
		ipMap.Store(ip, uid)
		// If any device is online
		if v, ok := l.UserOnlineIPs.LoadOrStore(email, ipMap); ok {
			ipMap := v.(*sync.Map)
			// If this is a new ip
			if _, ok := ipMap.LoadOrStore(ip, uid); !ok {
				var counter uint32 = 0
				ipMap.Range(func(key, value interface{}) bool {
					counter++
					return true
				})
				if counter > deviceLimit {
					ipMap.Delete(ip)
					return true
				}
			}
		}

		return false
}

func (l *Limiter) ResetDeviceLimit() error {
	l.UserOnlineIPs.Range(func(key, value interface{}) bool {
		email := key.(string)
		l.UserOnlineIPs.Delete(email)
		return true
	})

	return nil
}
