package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fmt.Fprintln(os.Stderr, "signal test — press Ctrl+C")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	count := 0
	last := int64(0)

	go func() {
		for {
			sig := <-sigCh
			fmt.Fprintf(os.Stderr, "got signal: %v\n", sig)
			now := time.Now().UnixMilli()
			if last > 0 && now-last < 1500 {
				fmt.Fprintln(os.Stderr, "double press — exiting")
				os.Exit(0)
			}
			last = now
			count++
			fmt.Fprintf(os.Stderr, "interrupt #%d\n", count)
		}
	}()

	for {
		fmt.Fprintf(os.Stderr, "> ")
		var s string
		fmt.Scanln(&s)
		if s != "" {
			fmt.Fprintf(os.Stderr, "you typed: %s\n", s)
		}
	}
}
