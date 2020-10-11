package bot

import (
	"context"
	"fmt"
	"os"
	"syscall"

	hps "github.com/michurin/cnbot/pkg/helpers"
	"github.com/michurin/cnbot/pkg/tg"
)

// TODO split Run to make it embeddable
// Run have to obtain:
// - shutdown context
// - configs
// - logger
// - http client
// - http server
func Run(rootCtx context.Context) {
	ctx, cancel := ShutdownCtx(rootCtx, syscall.SIGTERM, os.Interrupt)
	defer cancel()

	configFile, infoMode, err := hps.CommandLine()
	if err != nil {
		hps.Log(ctx, err)
		return
	}

	bots, err := hps.ReadConfig(configFile)
	if err != nil {
		hps.Log(ctx, configFile, err)
		return
	}

	if infoMode {
		report, err := BotsReport(ctx, bots)
		if err != nil {
			hps.Log(ctx, err)
		}
		fmt.Print("\nREPORT\n\n" + report + "\n\n")
		return
	}

	msgQueue := make(chan tg.Message, 1000) // TODO make buffer size configurable

	done := make(chan struct{}, 1)
	doneCount := 0

	for botName, bot := range bots {
		doneCount++
		go func(n string, b hps.BotConfig) {
			defer func() { done <- struct{}{} }()
			Poller(ctx, n, b, msgQueue)
		}(botName, bot)
		if bot.BindAddress != "" {
			doneCount++
			go func(n string, b hps.BotConfig) {
				defer func() { done <- struct{}{} }()
				RunHTTPServer(ctx, b.BindAddress, b.WriteTimeout, b.ReadTimeout, &Handler{
					BotName:      n,
					Token:        b.Token,
					AllowedUsers: b.AllowedUsers,
				})
			}(botName, bot)
		}
	}

	if len(bots) > 0 {
		doneCount++
		go func() {
			defer func() { done <- struct{}{} }()
			MessageProcessor(ctx, msgQueue, bots)
			done <- struct{}{}
		}()
	}

	if doneCount > 0 {
		<-done // waiting for at least one exit
		doneCount--
		cancel() // cancel all
		for ; doneCount >= 0; doneCount-- {
			<-done
		}
	}

	hps.Log(ctx, "Bot has been stopped")
}