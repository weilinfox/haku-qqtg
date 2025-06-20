package tg

import (
	"context"
	tgbotapi "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sihuan/qqtg-bridge/config"
	"github.com/sirupsen/logrus"
	"log"
	"net/http"
	"net/url"
	"time"
)

// Bot 全局 Bot
type Bot struct {
	*tgbotapi.Bot
	Chats map[int64]ChatChan
	start bool
	context.Context
}

// Instance Bot 实例
var Instance *Bot

var logger = logrus.WithField("tg", "internal")

// 使用 config.GlobalConfig 初始化 bot
func Init() {
	var (
		bot      *tgbotapi.Bot
		err      error
		proxyUrl *url.URL = nil
	)
	mc := make(map[int64]ChatChan)
	if config.GlobalConfig.Proxy.Enable {
		proxyUrl, err = url.Parse(config.GlobalConfig.Proxy.URL)
		if err != nil {
			logger.WithError(err).Errorln("Configure proxy failed")
			logger.Infoln("Try to init telegram bot without proxy")
		}
	}

	if proxyUrl != nil {
		proxyTrans := &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
		}
		proxyClient := &http.Client{
			Transport: proxyTrans,
		}
		bot, err = tgbotapi.New(config.GlobalConfig.TG.Token, tgbotapi.WithDefaultHandler(RouteMsg),
			tgbotapi.WithHTTPClient(time.Second, proxyClient))
	} else {
		bot, err = tgbotapi.New(config.GlobalConfig.TG.Token, tgbotapi.WithDefaultHandler(RouteMsg))
	}

	if err != nil {
		log.Panic(err)
	}
	Instance = &Bot{
		Bot:   bot,
		Chats: mc,
		start: false,
	}
}

func MakeChan() {
	for _, chatid := range config.GlobalConfig.TG.Chats {
		Instance.NewChatChan(chatid)
	}
}

func StartService(ctx *context.Context) {
	if Instance.start {
		return
	}

	Instance.Context = *ctx
	Instance.start = true

	Instance.Bot.Start(*ctx)
}

func RouteMsg(_ context.Context, b *tgbotapi.Bot, update *models.Update) {
	if update.Message == nil || (update.Message.Chat.Type != models.ChatTypeGroup && update.Message.Chat.Type != models.ChatTypeSupergroup) {
		return
	}

	if chat, ok := Instance.Chats[update.Message.Chat.ID]; ok {
		logger.Infof("[%s]: %s %s", update.Message.From.FirstName, update.Message.Text, update.Message.Caption)
		chat.tempChan <- update.Message
	}
}
