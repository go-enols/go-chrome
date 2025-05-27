package chrome

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type Config struct {
	Cookies []*network.Cookie
	Proxy   string
	Show    bool
}

type Browser struct {
	Ctx    context.Context
	cancel context.CancelFunc

	AllocCtx context.Context

	Monit *MonitNetWork

	loop chan struct{} //事件循环锁

	Config
}

func NewBrowser(ctx context.Context, config Config, opt ...chromedp.ExecAllocatorOption) *Browser {
	var opts = append(
		chromedp.DefaultExecAllocatorOptions[:],
		// chromedp.Flag("blink-settings", "imagesEnabled=false"), // 禁用图片
		// 禁止视频和其他媒体
		chromedp.Flag("autoplay-policy", "no-user-gesture-required"),
		chromedp.Flag("disable-features", "site-per-process,Translate,BlinkGenPropertyTrees,AutoplayIgnoreWebAudio"),
		chromedp.Flag("disable-audio", true),
		chromedp.Flag("enable-automation", false),                            // 禁用自动化标志，避免浏览器显示"Chrome正在被自动化软件控制"的提示
		chromedp.Flag("disable-infobars", true),                              // 禁用信息栏，防止显示"Chrome正在被自动化软件控制"的提示条
		chromedp.Flag("remote-allow-origins", "*"),                           // 允许所有远程源连接，解决跨域问题
		chromedp.Flag("disable-extensions", true),                            // 禁用浏览器扩展，提高性能并减少干扰
		chromedp.Flag("disable-default-apps", true),                          // 禁用默认应用，减少不必要的资源加载
		chromedp.Flag("disable-popup-blocking", true),                        // 禁用弹窗拦截，允许网站弹出窗口
		chromedp.Flag("disable-web-security", true),                          // 禁用网页安全策略，绕过同源策略限制
		chromedp.Flag("disable-site-isolation-trials", true),                 // 禁用站点隔离试验，避免多进程导致的问题
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"), // 禁用站点隔离相关特性，提高性能和兼容性
		chromedp.Flag("disable-blink-features", "AutomationControlled"),      // 禁用自动化控制标志，这是防止被检测为自动化的关键选项
		// chromedp.Flag("no-sandbox", true),                       // 禁用沙盒模式，提高性能（安全性较低，谨慎使用）
		chromedp.Flag("ignore-certificate-errors", true),  // 忽略证书错误，允许访问自签名证书的网站
		chromedp.Flag("proxy-bypass-list", "<-loopback>"), // 设置代理绕过列表，确保所有请求都通过代理（不绕过本地回环地址）
		// chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND , EXCLUDE localhost"), // 自定义DNS解析规则
		chromedp.Flag("start-maximized", true),                    // 启动时最大化窗口，模拟真实用户行为
		chromedp.Flag("use-fake-ui-for-media-stream", true),       // 使用假的UI处理媒体流请求，自动允许摄像头/麦克风权限
		chromedp.Flag("disable-notifications", true),              // 禁用通知提示，避免干扰自动化流程
		chromedp.Flag("disable-hang-monitor", true),               // 禁用挂起监视器，防止长时间运行的脚本被终止
		chromedp.Flag("disable-dev-shm-usage", true),              // 禁用/dev/shm使用，解决某些Linux系统上的内存问题
		chromedp.Flag("disable-webrtc", true),                     // 禁用WebRTC，防止IP地址泄露
		chromedp.Flag("enforce-webrtc-ip-permission-check", true), // 强制WebRTC IP权限检查，增强隐私保护
	)
	if config.Show {
		opts = append(opts, chromedp.Flag("headless", false)) // 禁用无头模式，使浏览器可见，减少被检测为自动化工具的可能性
	}
	opts = append(opts, opt...)

	// 设置代理如果有的话
	if config.Proxy != "" {
		opts = append(opts, chromedp.ProxyServer(config.Proxy))
	}
	c, _ := chromedp.NewExecAllocator(
		ctx,
		opts...,
	)
	// 创建一个新的Chrome浏览器实例
	chromeCtx, chromeCancel := chromedp.NewContext(c)

	client := &Browser{
		AllocCtx: c,
		Ctx:      chromeCtx,
		cancel:   chromeCancel,
		Monit:    NewMonitNetWork(chromeCtx),
		loop:     make(chan struct{}),
	}
	if len(config.Cookies) > 0 {
		client.SetCookies(config.Cookies)
	}

	return client
}

func (b *Browser) Close() {
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil // 防止重复调用
	}
}

func (b *Browser) Run(opt ...any) error {
	var actions []chromedp.Action
	var timeOut = TimeOut
	var ctx = b.Ctx
	for _, v := range opt {
		switch data := v.(type) {
		case time.Duration:
			timeOut = data
		case int:
			timeOut = time.Second * time.Duration(data)
		case int64:
			timeOut = time.Second * time.Duration(data)
		case chromedp.Action:
			actions = append(actions, data)
		case context.Context:
			ctx = data
		}
	}
	// 创建一个带有超时的 Poll 操作
	timeoutAction := chromedp.ActionFunc(func(ctx context.Context) error {
		errCh := make(chan error, 1)

		go func() {
			errCh <- chromedp.Tasks(actions).Do(ctx)
		}()

		select {
		case err := <-errCh:
			return err
		case <-time.After(timeOut):
			return fmt.Errorf("操作超时")
		}
	})

	// 执行带有超时的操作
	return chromedp.Run(ctx, timeoutAction)
}

// 获取当前浏览器cookie
func (b *Browser) GetCookies() ([]*network.Cookie, error) {
	// 获取 cookie
	var cookies []*network.Cookie
	err := b.Run(
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 获取所有 cookie
			c, err := network.GetCookies().Do(ctx)
			if err != nil {
				return err
			}
			cookies = c
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	// 顺便更新配置中的cookie
	b.Cookies = cookies
	return cookies, nil
}

func (b *Browser) SetCookies(cookies []*network.Cookie) error {
	return b.Run(
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 对于每个 cookie，使用 SetCookie 设置
			for _, cookie := range cookies {
				expiry := cdp.TimeSinceEpoch(time.Unix(int64(cookie.Expires), 0))
				if err := network.SetCookie(cookie.Name, cookie.Value).
					WithDomain(cookie.Domain).
					WithPath(cookie.Path).
					WithSecure(cookie.Secure).
					WithHTTPOnly(cookie.HTTPOnly).
					WithSameSite(cookie.SameSite).
					WithExpires(&expiry).
					Do(ctx); err != nil {
					return err
				}
			}
			return nil
		}),
	)
}
