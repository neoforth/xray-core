package limiter

import (
	"context"

	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/errors"
	"golang.org/x/time/rate"
)

type Writer struct {
	writer  buf.Writer
	limiter *rate.Limiter
	ctx     context.Context
}

func (l *Limiter) RateWriter(ctx context.Context, writer buf.Writer, limiter *rate.Limiter) buf.Writer {
	if ctx == nil {
		errors.LogWarning(context.Background(), "limiter: context is nil, falling back to background")
		ctx = context.Background()
	}

	return &Writer{
		writer:  writer,
		limiter: limiter,
		ctx:     ctx,
	}
}

func (w *Writer) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if err := w.limiter.WaitN(w.ctx, int(mb.Len())); err != nil {
		return newError("failed to wait for rate limiter").Base(err)
	}

	return w.writer.WriteMultiBuffer(mb)
}

func newError(values ...interface{}) *errors.Error {
	return errors.New(values...)
}
