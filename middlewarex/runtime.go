package middlewarex

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Recovery converts panics into internal errors and logs them.
func Recovery[Req, Resp any](logger Logger) Middleware[Req, Resp] {
	return func(next Handler[Req, Resp]) Handler[Req, Resp] {
		return func(ctx context.Context, req Req) (resp Resp, err error) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}

				panicErr := fmt.Errorf("%w: %v", errRecoveredPanic, recovered)
				if logger != nil {
					logger.Log(ctx, Event{
						Level:   "error",
						Name:    "recovery",
						Message: "panic recovered",
						Err:     panicErr,
					})
				}
				err = Internal(panicErr)
			}()

			return next(ctx, req)
		}
	}
}

// Timeout cancels handler context after duration and maps deadline errors.
func Timeout[Req, Resp any](duration time.Duration) Middleware[Req, Resp] {
	return func(next Handler[Req, Resp]) Handler[Req, Resp] {
		return func(ctx context.Context, req Req) (Resp, error) {
			if duration <= 0 {
				return *new(Resp), Internal(errTimeoutNonPositive)
			}

			ctx, cancel := context.WithTimeout(ctx, duration)
			defer cancel()

			resp, err := next(ctx, req)
			if err != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
					return *new(Resp), TimeoutError(context.DeadlineExceeded)
				}
				return resp, err
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return *new(Resp), TimeoutError(context.DeadlineExceeded)
			}

			return resp, nil
		}
	}
}
