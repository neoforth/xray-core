// Package limiter is to control the links that go into the dispatcher
package limiter

type Limiter struct {}

func New() *Limiter {
	return &Limiter{}
}
