package tg

import (
	tgbotapi "github.com/go-telegram/bot"
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
		bot, err = tgbotapi.New(config.GlobalConfig.TG.Token, tgbotapi.WithHTTPClient(time.Second, proxyClient))
	} else {
		bot, err = tgbotapi.New(config.GlobalConfig.TG.Token)
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

func StartService() {
	if Instance.start {
		return
	}

	Instance.start = true

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := Instance.GetUpdatesChan(u)
	for update := range updates {
		if update.Message == nil || (!update.Message.Chat.IsGroup() && !update.Message.Chat.IsSuperGroup()) {
			continue
		}
		if chat, ok := Instance.Chats[update.Message.Chat.ID]; ok {
			logger.Infof("[%s]: %s %s", update.Message.From.FirstName, update.Message.Text, update.Message.Caption)
			chat.tempChan <- update.Message
		}
	}
}
