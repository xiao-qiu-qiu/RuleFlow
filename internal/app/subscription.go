package app

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxSubscriptionBodyBytes = 64 << 20

// FetchSubscriptionContent 从订阅地址获取原始内容和响应头。
func FetchSubscriptionContent(ctx context.Context, subURL string) (string, http.Header, error) {
	return fetchSubscriptionContent(ctx, subURL, &http.Client{Timeout: 30 * time.Second})
}

func fetchSubscriptionContent(ctx context.Context, subURL string, client *http.Client) (string, http.Header, error) {
	if subURL == "" {
		return "", nil, fmt.Errorf("订阅地址不能为空")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "clash.meta/v1.19.16")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Connection", "keep-alive")

	resp, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err // URL paths and queries may contain subscription credentials.
		}
		return "", nil, fmt.Errorf("获取订阅失败（网络错误）: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 尝试解压错误响应体（部分服务器对非 200 响应也启用 gzip）
		errorBody, _ := readResponseBody(resp)
		errorHint := ""
		if len(errorBody) > 0 {
			preview := string(errorBody)
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			errorHint = fmt.Sprintf("，响应内容: %s", preview)
		}
		return "", nil, fmt.Errorf("订阅服务器返回错误（HTTP %d）%s", resp.StatusCode, errorHint)
	}

	content, err := readResponseBody(resp)
	if err != nil {
		return "", nil, fmt.Errorf("读取订阅内容失败: %w", err)
	}

	if len(content) == 0 {
		return "", nil, fmt.Errorf("订阅服务器返回了空内容")
	}

	return string(content), resp.Header, nil
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	encoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	reader := io.Reader(resp.Body)
	var closeFn func() error

	switch encoding {
	case "", "identity":
		// 无压缩
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("解析 gzip 响应失败: %w", err)
		}
		reader = gr
		closeFn = gr.Close
	case "deflate":
		// zlib.NewReader consumes the header even when it fails. Retry raw
		// DEFLATE from the beginning, not from the partially consumed body.
		compressed, err := readLimitedSubscriptionBody(resp.Body, maxSubscriptionBodyBytes)
		if err != nil {
			return nil, err
		}
		zr, err := zlib.NewReader(bytes.NewReader(compressed))
		if err == nil {
			reader = zr
			closeFn = zr.Close
			break
		}
		fr := flate.NewReader(bytes.NewReader(compressed))
		reader = fr
		closeFn = fr.Close
	default:
		return nil, fmt.Errorf("不支持的响应压缩格式: %s", encoding)
	}

	if closeFn != nil {
		defer closeFn()
	}

	return readLimitedSubscriptionBody(reader, maxSubscriptionBodyBytes)
}

func readLimitedSubscriptionBody(reader io.Reader, limit int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("订阅或规则源内容超过大小限制（%d 字节）", limit)
	}
	return content, nil
}
