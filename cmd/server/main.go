package main

import (
	"fmt"
	"os"

	"bb_erp_echo/internal/app"
)

func main() {
	// app.New 会完成配置加载、数据库连接、自动迁移、权限策略加载和路由注册。
	erp, err := app.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Run 会阻塞监听 HTTP 请求，并在收到中断信号时执行优雅关闭。
	if err := erp.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
