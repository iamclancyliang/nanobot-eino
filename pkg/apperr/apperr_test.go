package apperr

import (
	"errors"
	"strings"
	"testing"
)

func TestWrapPreservesSentinelAndKind(t *testing.T) {
	err := Wrap(KindNotFound, "tool.Resolve", ErrToolNotFound)

	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("expected errors.Is to match ErrToolNotFound")
	}
	if !Is(err, KindNotFound) {
		t.Fatalf("expected Is to match KindNotFound")
	}
	if got := KindOf(err); got != KindNotFound {
		t.Fatalf("KindOf = %q, want %q", got, KindNotFound)
	}
	if got := err.Error(); !strings.Contains(got, "tool.Resolve") || !strings.Contains(got, string(KindNotFound)) {
		t.Fatalf("expected error string to include op and kind, got %q", got)
	}
}

func TestRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "rate limited", err: Wrap(KindRateLimited, "model.Stream", errors.New("rate limit")), want: false},
		{name: "unavailable", err: Wrap(KindUnavailable, "model.Stream", errors.New("busy")), want: true},
		{name: "timeout", err: Wrap(KindTimeout, "model.Stream", errors.New("timeout")), want: true},
		{name: "network", err: Wrap(KindNetwork, "model.Stream", errors.New("reset")), want: true},
		{name: "invalid", err: Wrap(KindInvalid, "model.Stream", errors.New("bad request")), want: false},
		{name: "unknown", err: errors.New("boom"), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Retryable(tc.err); got != tc.want {
				t.Fatalf("Retryable = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeClassifiesProviderErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		kind Kind
		msg  string
	}{
		{name: "503 service busy", err: errors.New(`HTTP 503: {"error":{"message":"Service is too busy."}}`), kind: KindUnavailable, msg: "繁忙"},
		{name: "429 rate limit", err: errors.New(`HTTP 429: rate limit exceeded`), kind: KindRateLimited, msg: "频率受限"},
		{name: "401 auth", err: errors.New(`HTTP 401 Unauthorized: invalid api key`), kind: KindUnauthorized, msg: "鉴权失败"},
		{name: "404 model not found", err: errors.New(`HTTP 404: model_not_found`), kind: KindNotFound, msg: "模型不存在"},
		{name: "400 surfaces detail", err: errors.New(`HTTP 400: {"error":{"message":"Tool names must be unique."}}`), kind: KindInvalid, msg: "Tool names must be unique"},
		{name: "context canceled", err: errors.New("context canceled"), kind: KindCanceled, msg: "已取消"},
		{name: "i/o timeout", err: errors.New("dial tcp: i/o timeout"), kind: KindTimeout, msg: "超时"},
		{name: "network", err: errors.New("unexpected EOF"), kind: KindNetwork, msg: "网络异常"},
		{name: "unknown detail", err: errors.New(`weird error: {"error":{"message":"something specific"}}`), kind: KindUnknown, msg: "something specific"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Normalize("model.Stream", tc.err)
			if got := KindOf(err); got != tc.kind {
				t.Fatalf("KindOf = %q, want %q", got, tc.kind)
			}
			if got := PublicMessage(err); !strings.Contains(got, tc.msg) {
				t.Fatalf("PublicMessage should contain %q, got %q", tc.msg, got)
			}
		})
	}
}

func TestPublicMessageFallsBack(t *testing.T) {
	if got := PublicMessage(nil); got != DefaultPublicMessage {
		t.Fatalf("nil PublicMessage = %q, want %q", got, DefaultPublicMessage)
	}
	if got := PublicMessage(errors.New("boom")); got != DefaultPublicMessage {
		t.Fatalf("unknown PublicMessage = %q, want %q", got, DefaultPublicMessage)
	}
}
