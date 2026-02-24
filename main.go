package main

import (
	"fmt"
	"os"
	"trade-go/app"
	"trade-go/config"
)

func main() {
	config.Load()
	runner := app.NewRunner()
	if err := runner.Setup(); err != nil {
		fmt.Printf("交易所初始化失败: %v\n", err)
		return
	}

	mode := os.Getenv("MODE")
	if err := runner.Run(mode); err != nil {
		fmt.Printf("启动失败: %v\n", err)
	}
}
