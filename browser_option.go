package pageviewer

type browserOptions struct {
	Debug            bool
	Proxy            string
	IgnoreCertErrors bool
	ChromePath       string // 设定后可以复用浏览器cookie
	UserModeBrowser  bool   // 是否使用用户浏览器
}

type BrowserOption func(*browserOptions)

func WithDebug(debug bool) BrowserOption {
	return func(o *browserOptions) {
		o.Debug = debug
	}
}
func WithProxy(proxy string) BrowserOption {
	return func(o *browserOptions) {
		o.Proxy = proxy
	}
}
func WithIgnoreCertErrors(ignoreCertErrors bool) BrowserOption {
	return func(o *browserOptions) {
		o.IgnoreCertErrors = ignoreCertErrors
	}
}
func WithChromePath(chromePath string) BrowserOption {
	return func(o *browserOptions) {
		o.ChromePath = chromePath
	}
}

func WithUserModeBrowser(userModeBrowser bool) BrowserOption {
	return func(o *browserOptions) {
		o.UserModeBrowser = userModeBrowser
	}
}
