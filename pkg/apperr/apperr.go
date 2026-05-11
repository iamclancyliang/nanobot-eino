package apperr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

const DefaultPublicMessage = "🐈 处理消息时出错，请稍后重试，或查看 gateway 日志获取详情。"

type Kind string

const (
	KindUnknown        Kind = "unknown"
	KindInvalid        Kind = "invalid"
	KindUnauthorized   Kind = "unauthorized"
	KindNotFound       Kind = "not_found"
	KindRateLimited    Kind = "rate_limited"
	KindUnavailable    Kind = "unavailable"
	KindTimeout        Kind = "timeout"
	KindCanceled       Kind = "canceled"
	KindNetwork        Kind = "network"
	KindContextTooLong Kind = "context_too_long"
	KindMaxSteps       Kind = "max_steps"
)

var (
	ErrToolNotFound = errors.New("tool not found")
)

type Error struct {
	Kind       Kind
	Op         string
	StatusCode int
	Public     string
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := make([]string, 0, 4)
	if e.Op != "" {
		parts = append(parts, e.Op)
	}
	if e.Kind != "" {
		parts = append(parts, string(e.Kind))
	}
	if e.StatusCode != 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", e.StatusCode))
	}
	prefix := strings.Join(parts, ": ")
	if e.Err == nil {
		if prefix == "" {
			return string(KindUnknown)
		}
		return prefix
	}
	if prefix == "" {
		return e.Err.Error()
	}
	return prefix + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func Wrap(kind Kind, op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Op: op, Err: err}
}

func WithStatus(kind Kind, op string, statusCode int, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Op: op, StatusCode: statusCode, Err: err}
}

func KindOf(err error) Kind {
	var appErr *Error
	if errors.As(err, &appErr) && appErr.Kind != "" {
		return appErr.Kind
	}
	return KindUnknown
}

func Is(err error, kind Kind) bool {
	return KindOf(err) == kind
}

func Retryable(err error) bool {
	switch KindOf(err) {
	case KindUnavailable, KindTimeout, KindNetwork:
		return true
	default:
		return false
	}
}

func PublicMessage(err error) string {
	if err == nil {
		return DefaultPublicMessage
	}
	normalized := Normalize("", err)
	var appErr *Error
	if errors.As(normalized, &appErr) {
		if appErr.Public != "" {
			return appErr.Public
		}
	}
	if detail := extractAPIErrorMessage(err.Error()); detail != "" {
		return "🐈 处理消息时出错：" + detail
	}
	return DefaultPublicMessage
}

func Normalize(op string, err error) error {
	if err == nil {
		return nil
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Kind: KindCanceled, Op: op, Public: "🐈 任务已取消。", Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: KindTimeout, Op: op, Public: "🐈 模型调用超时，请稍后重试。", Err: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Kind: KindTimeout, Op: op, Public: "🐈 模型调用超时，请稍后重试。", Err: err}
	}

	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "HTTP 503"),
		strings.Contains(lower, "service is too busy"),
		strings.Contains(lower, "service unavailable"):
		return &Error{Kind: KindUnavailable, Op: op, StatusCode: 503, Public: "🐈 模型服务暂时繁忙（HTTP 503），请稍后再试。", Err: err}
	case strings.Contains(msg, "HTTP 429"),
		strings.Contains(lower, "rate limit"),
		strings.Contains(lower, "too many requests"):
		return &Error{Kind: KindRateLimited, Op: op, StatusCode: 429, Public: "🐈 模型调用频率受限（HTTP 429），请稍候片刻再试。", Err: err}
	case strings.Contains(msg, "HTTP 401"),
		strings.Contains(msg, "HTTP 403"),
		strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "invalid api key"),
		strings.Contains(lower, "authentication"):
		return &Error{Kind: KindUnauthorized, Op: op, Public: "🐈 模型 API 鉴权失败，请检查 apiKey 配置。", Err: err}
	case strings.Contains(lower, "model_not_found"),
		strings.Contains(lower, "model not found"),
		strings.Contains(msg, "HTTP 404"):
		return &Error{Kind: KindNotFound, Op: op, StatusCode: 404, Public: "🐈 模型不存在或不可用，请检查 model 配置。", Err: err}
	case strings.Contains(msg, "HTTP 400"):
		public := "🐈 模型请求参数有误（HTTP 400），请检查日志。"
		if detail := extractAPIErrorMessage(msg); detail != "" {
			public = "🐈 模型请求参数有误：" + detail
		}
		return &Error{Kind: KindInvalid, Op: op, StatusCode: 400, Public: public, Err: err}
	case strings.Contains(msg, "HTTP 500"),
		strings.Contains(msg, "HTTP 502"),
		strings.Contains(msg, "HTTP 504"):
		return &Error{Kind: KindUnavailable, Op: op, Public: "🐈 模型服务异常，请稍后重试。", Err: err}
	case strings.Contains(lower, "context canceled"):
		return &Error{Kind: KindCanceled, Op: op, Public: "🐈 任务已取消。", Err: err}
	case strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "i/o timeout"),
		strings.Contains(lower, "tls handshake timeout"),
		strings.Contains(lower, "timeout"):
		return &Error{Kind: KindTimeout, Op: op, Public: "🐈 模型调用超时，请稍后重试。", Err: err}
	case strings.Contains(lower, "unexpected eof"),
		strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "connection refused"):
		return &Error{Kind: KindNetwork, Op: op, Public: "🐈 网络异常，请稍后重试。", Err: err}
	}

	public := ""
	if detail := extractAPIErrorMessage(msg); detail != "" {
		public = "🐈 处理消息时出错：" + detail
	}
	return &Error{Kind: KindUnknown, Op: op, Public: public, Err: err}
}

func extractAPIErrorMessage(s string) string {
	const key = `"message":"`
	idx := strings.Index(s, key)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(key):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	detail := strings.TrimSpace(rest[:end])
	if len(detail) > 200 {
		detail = detail[:200] + "…"
	}
	return detail
}
