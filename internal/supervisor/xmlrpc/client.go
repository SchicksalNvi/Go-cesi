package xmlrpc

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	url      string
	username string
	password string
	client   *http.Client
}

func NewClient(host string, port int, username, password string) (*Client, error) {
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}

	baseURL, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("invalid host URL: %v", err)
	}

	// 设置认证信息
	if username != "" || password != "" {
		baseURL.User = url.UserPassword(username, password)
	}

	// 添加端口和RPC2路径
	baseURL.Host = fmt.Sprintf("%s:%d", baseURL.Hostname(), port)
	baseURL.Path = "/RPC2"

	return &Client{
		url:      baseURL.String(),
		username: username,
		password: password,
		client: &http.Client{
			// 35s:须大于上层 TimeoutManager.SingleOperationTimeout(30s)。
			// startProcess/stopProcess 同步 wait=true 时 supervisord 会阻塞到操作完成
			// (慢启动/优雅停止可能耗时数秒~数十秒);若此超时过小,会在操作尚未完成时
			// 返回 HTTP 超时,误判操作失败,而实际后台仍在执行。
			Timeout: 35 * time.Second,
		},
	}, nil
}

func (c *Client) Call(method string, args []interface{}) (interface{}, error) {
	// 构建XML-RPC请求
	request := fmt.Sprintf(`<?xml version="1.0"?>
<methodCall>
	<methodName>%s</methodName>
	<params>%s</params>
</methodCall>`, method, c.encodeParams(args))

	// 发送请求
	resp, err := c.client.Post(c.url, "text/xml", bytes.NewBufferString(request))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应内容(使用限制读取器,防止恶意/异常节点返回超大响应导致内存膨胀)
	limitedBody := io.LimitReader(resp.Body, 10*1024*1024) // 限制最大 10MB
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	// 简单粗暴的解析：直接返回XML字符串，让上层处理
	return string(body), nil
}

func (c *Client) encodeParams(args []interface{}) string {
	if len(args) == 0 {
		return ""
	}

	params := make([]string, len(args))
	for i, arg := range args {
		params[i] = fmt.Sprintf("<param><value>%s</value></param>", c.encodeValue(arg))
	}

	return strings.Join(params, "")
}

func (c *Client) encodeValue(v interface{}) string {
	switch v := v.(type) {
	case string:
		return fmt.Sprintf("<string>%s</string>", v)
	case int, int32, int64:
		return fmt.Sprintf("<int>%d</int>", v)
	case float32, float64:
		return fmt.Sprintf("<double>%f</double>", v)
	case bool:
		return fmt.Sprintf("<boolean>%d</boolean>", map[bool]int{false: 0, true: 1}[v])
	default:
		return fmt.Sprintf("<string>%v</string>", v)
	}
}
