package utils

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"kiro2api/config"

	"golang.org/x/net/proxy"
)

var (
	// SharedHTTPClient 共享的HTTP客户端实例，优化了连接池和性能配置
	SharedHTTPClient *http.Client
)

func init() {
	// 检查TLS配置并记录日志
	skipTLS := shouldSkipTLSVerify()
	if skipTLS {
		os.Stderr.WriteString("[WARNING] TLS证书验证已禁用 - 仅适用于开发/调试环境\n")
	}

	// 创建基础 dialer
	baseDialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: config.HTTPClientKeepAlive,
		DualStack: true,
	}

	// 创建 transport
	transport := &http.Transport{
		// 连接池配置
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     120 * time.Second,

		// 连接建立配置
		DialContext: baseDialer.DialContext,

		// TLS配置
		TLSHandshakeTimeout: config.HTTPClientTLSHandshakeTimeout,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLS,
			MinVersion:         tls.VersionTLS12,
			MaxVersion:         tls.VersionTLS13,
			CipherSuites: []uint16{
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_AES_128_GCM_SHA256,
			},
		},

		// HTTP配置
		ForceAttemptHTTP2:     false,
		DisableCompression:    false,
		WriteBufferSize:       32 * 1024,
		ReadBufferSize:        32 * 1024,
		ResponseHeaderTimeout: 60 * time.Second,
	}

	// 配置代理（支持 HTTP/HTTPS/SOCKS5）
	if err := configureProxy(transport, baseDialer); err != nil {
		os.Stderr.WriteString(fmt.Sprintf("[WARNING] 代理配置失败: %v，使用直连\n", err))
	}

	// 创建统一的HTTP客户端
	SharedHTTPClient = &http.Client{
		Transport: transport,
	}
}

// shouldSkipTLSVerify 根据GIN_MODE决定是否跳过TLS证书验证
func shouldSkipTLSVerify() bool {
	return os.Getenv("GIN_MODE") == "debug"
}

// configureProxy 配置代理（支持 HTTP/HTTPS/SOCKS5）
// 优先使用 PROXY_URL 环境变量，其次使用标准的 HTTP_PROXY/HTTPS_PROXY
func configureProxy(transport *http.Transport, baseDialer *net.Dialer) error {
	proxyURL := os.Getenv("PROXY_URL")
	if proxyURL == "" {
		// 回退到标准环境变量
		transport.Proxy = http.ProxyFromEnvironment
		return nil
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return fmt.Errorf("解析代理 URL 失败: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https":
		// HTTP/HTTPS 代理
		transport.Proxy = http.ProxyURL(parsed)
		os.Stderr.WriteString(fmt.Sprintf("[INFO] 已配置 HTTP 代理: %s\n", parsed.Host))

	case "socks5", "socks5h":
		// SOCKS5 代理
		var auth *proxy.Auth
		if parsed.User != nil {
			password, _ := parsed.User.Password()
			auth = &proxy.Auth{
				User:     parsed.User.Username(),
				Password: password,
			}
		}

		dialer, err := proxy.SOCKS5("tcp", parsed.Host, auth, baseDialer)
		if err != nil {
			return fmt.Errorf("创建 SOCKS5 dialer 失败: %w", err)
		}

		// 转换为 ContextDialer
		if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
			transport.DialContext = contextDialer.DialContext
		} else {
			// 回退：包装为 DialContext
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		}
		os.Stderr.WriteString(fmt.Sprintf("[INFO] 已配置 SOCKS5 代理: %s\n", parsed.Host))

	default:
		return fmt.Errorf("不支持的代理协议: %s", scheme)
	}

	return nil
}

// DoRequest 执行HTTP请求
func DoRequest(req *http.Request) (*http.Response, error) {
	return SharedHTTPClient.Do(req)
}

// ProxyAwareClient 支持代理池的HTTP客户端
type ProxyAwareClient struct {
	baseTransport *http.Transport
}

// NewProxyAwareClient 创建支持代理池的客户端
func NewProxyAwareClient() *ProxyAwareClient {
	return &ProxyAwareClient{
		baseTransport: SharedHTTPClient.Transport.(*http.Transport).Clone(),
	}
}

// DoWithProxy 使用指定代理执行请求
func (c *ProxyAwareClient) DoWithProxy(req *http.Request, proxyURL string) (*http.Response, error) {
	if proxyURL == "" {
		return SharedHTTPClient.Do(req)
	}

	// 解析代理URL
	proxy, err := parseProxyURL(proxyURL)
	if err != nil {
		return nil, err
	}

	// 克隆transport并设置代理
	transport := c.baseTransport.Clone()
	transport.Proxy = http.ProxyURL(proxy)

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}

	return client.Do(req)
}

// parseProxyURL 解析代理URL
func parseProxyURL(proxyURL string) (*url.URL, error) {
	return url.Parse(proxyURL)
}
