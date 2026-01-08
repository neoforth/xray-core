package limiter

import (
	"context"
	"io"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"golang.org/x/time/rate"
)

type Writer struct {
	writer  buf.Writer
	limiter *rate.Limiter
	w       io.Writer
	ctx     context.Context
}

func RateWriter(ctx context.Context, writer buf.Writer, limiter *rate.Limiter) buf.Writer {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Writer{
		writer:  writer,
		limiter: limiter,
		w:       writer,
		ctx:     ctx,
	}
}

func (w *Writer) Close() error {
	return common.Close(w.writer)
}

func (w *Writer) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if err := w.limiter.WaitN(w.ctx, int(mb.Len())); err != nil {
		return newError("failed to wait for rate limiter").Base(err)
	}
	return w.writer.WriteMultiBuffer(mb)
}
