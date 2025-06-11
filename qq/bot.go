package qq

import (
	"bufio"
	"bytes"
	"fmt"
	qrcodeTerminal "github.com/Baozisoftware/qrcode-terminal-go"
	"github.com/LagrangeDev/LagrangeGo/client"
	"github.com/LagrangeDev/LagrangeGo/client/auth"
	"github.com/LagrangeDev/LagrangeGo/client/packets/wtlogin/qrcodestate"
	mirai "github.com/LagrangeDev/LagrangeGo/message"
	"github.com/LagrangeDev/LagrangeGo/utils/crypto"
	"github.com/sihuan/qqtg-bridge/config"
	"github.com/sihuan/qqtg-bridge/utils"
	"github.com/sirupsen/logrus"
	"github.com/tuotoo/qrcode"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Bot 全局 Bot
type Bot struct {
	*client.QQClient
	Chats map[int64]ChatChan
	start bool
}

// Instance Bot 实例
var Instance *Bot

// Bot 转发 Client
var proxyClient *http.Client

var logger = logrus.WithField("qq", "internal")

// Init 快速初始化
// 使用 config.GlobalConfig 初始化账号
// 使用 ./device.json 初始化设备信息
func Init() {
	mc := make(map[int64]ChatChan)
	Instance = &Bot{
		QQClient: client.NewClient(
			uint32(config.GlobalConfig.QQ.Account),
			config.GlobalConfig.QQ.Password,
		),
		Chats: mc,
		start: false,
	}
	// default android watch protocol may fail to log in
	// client.SystemDeviceInfo.Protocol = client.IPad

	appInfo := auth.AppList["linux"]["3.2.15-30366"]
	Instance.UseVersion(appInfo)

	if config.GlobalConfig.QQ.SignServer == "" {
		Instance.AddSignServer("https://sign.lagrangecore.org/api/sign/30366")
	} else {
		Instance.AddSignServer(config.GlobalConfig.QQ.SignServer)
	}

	b, _ := utils.FileExist("./device.json")
	if !b {
		logger.Warnln("no device.json, GenRandomDevice")
		GenRandomDevice()
	}
	device, err := auth.LoadOrSaveDevice("./device.json")

	if err != nil {
		logger.WithError(err).Panic("device.json error")
	}
	Instance.UseDevice(device)

	var proxyUrl *url.URL = nil
	if config.GlobalConfig.Proxy.Enable {
		var err error
		proxyUrl, err = url.Parse(config.GlobalConfig.Proxy.URL)
		if err != nil {
			logger.WithError(err).Errorln("Configure proxy failed")
			logger.Infoln("Try to init qq forward without proxy")
		}
	}

	if proxyUrl != nil {
		proxyTrans := &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
		}
		proxyClient = &http.Client{
			Transport: proxyTrans,
		}
	} else {
		proxyClient = &http.Client{}
	}
}

// GenRandomDevice 生成随机设备信息
func GenRandomDevice() {
	b, err := utils.FileExist("./device.json")
	if err != nil {
		logger.WithError(err).Errorln("device.json existence check error")
	} else if b {
		logger.Warn("device.json exists, write new device to file")
	}
	err = auth.NewDeviceInfo(int(crypto.RandU32())).Save("./device.json")
	if err != nil {
		logger.WithError(err).Errorf("unable to write device.json")
	}
}

// QrcodeLogin 扫码登陆
func QrcodeLogin() (*client.LoginResponse, error) {
	resp, _, err := Instance.FetchQRCode(1, 2, 1)
	if err != nil {
		return nil, err
	}
	fi, err := qrcode.Decode(bytes.NewReader(resp))
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile("qrcode.png", resp, 0o644)
	defer func() { _ = os.Remove("qrcode.png") }()
	if Instance.Uin != 0 {
		logger.Infof("Use mobile QQ scan QR code (qrcode.png) with ID %v login: ", Instance.Uin)
	} else {
		logger.Infof("Use mobile QQ scan QR code (qrcode.png) to login: ")
	}
	time.Sleep(time.Second)
	qrcodeTerminal.New().Get(fi.Content).Print()
	s, err := Instance.GetQRCodeResult()
	if err != nil {
		return nil, err
	}
	prevState := s
	for {
		time.Sleep(time.Second)
		s, _ = Instance.GetQRCodeResult()
		if prevState == s {
			continue
		}
		prevState = s

		switch s {
		case qrcodestate.Canceled:
			logger.Fatalf("Scan was canceled by user.")
		case qrcodestate.Expired:
			logger.Fatalf("QR code was expired.")
		case qrcodestate.WaitingForConfirm:
			logger.Infof("Scan succeed, please confirm login.")
		case qrcodestate.Confirmed:
			resp, err := Instance.QRCodeLogin()
			if err != nil {
				return nil, err
			}
			return resp, err
		case qrcodestate.WaitingForScan:
			// ignore
		}
	}
}

// Login 登录
func Login() {
	if exist, _ := utils.FileExist("session.token"); exist {
		logger.Infof("Find session token cache.")
		token, _ := os.ReadFile("session.token")
		sig, err := auth.UnmarshalSigInfo(token, true)
		if err == nil {
			if Instance.Uin != 0 && sig.Uin != Instance.Uin {
				logger.Fatalf("The QQ id in configure file (%v) is vary from cached token (%v) .", Instance.Uin, sig.Uin)
				logger.Fatalf("Exit now.")
				os.Exit(0)
			}
			Instance.UseSig(sig)
			if err = Instance.FastLogin(); err != nil {
				_ = os.Remove("session.token")
				logger.Warnf("Token login failed: %v .", err)
				os.Exit(1)
			} else {
				logger.Infof("Token login succeed.")
				return
			}
		}
	}

	resp, err := QrcodeLogin()
	console := bufio.NewReader(os.Stdin)

	for {
		if err != nil {
			logger.WithError(err).Fatal("unable to login")
		}

		var text string
		if !resp.Success {
			switch resp.Error {

			/*	case client.NeedCaptcha:
				img, _, _ := image.Decode(bytes.NewReader(resp.CaptchaImage))
				fmt.Println(asc2art.New("image", img).Art)
				fmt.Print("please input captcha: ")
				text, _ := console.ReadString('\n')
				resp, err = Instance.SubmitCaptcha(strings.ReplaceAll(text, "\n", ""), resp.CaptchaSign)
				continue  */

			case client.UnsafeDeviceError:
				fmt.Printf("device lock -> %v\n", resp.VerifyURL)
				os.Exit(4)

			/*	case client.SMSNeededError:
				fmt.Println("device lock enabled, Need SMS Code")
				fmt.Printf("Send SMS to %s ? (yes)", resp.SMSPhone)
				t, _ := console.ReadString('\n')
				t = strings.TrimSpace(t)
				if t != "yes" {
					os.Exit(2)
				}
				if !Instance.RequestSMS() {
					logger.Warnf("unable to request SMS Code")
					os.Exit(2)
				}
				logger.Warn("please input SMS Code: ")
				text, _ = console.ReadString('\n')
				resp, err = Instance.SubmitSMS(strings.ReplaceAll(strings.ReplaceAll(text, "\n", ""), "\r", ""))
				continue  */

			case client.TooManySMSRequestError:
				fmt.Printf("too many SMS request, please try later.\n")
				os.Exit(6)

			/*	case client.SMSOrVerifyNeededError:
				fmt.Println("device lock enabled, choose way to verify:")
				fmt.Println("1. Send SMS Code to ", resp.SMSPhone)
				fmt.Println("2. Scan QR Code")
				fmt.Print("input (1,2):")
				text, _ = console.ReadString('\n')
				text = strings.TrimSpace(text)
				switch text {
				case "1":
					if !Instance.RequestSMS() {
						fmt.Println("unable to request SMS Code")
						os.Exit(2)
					}
					fmt.Print("please input SMS Code: ")
					text, _ = console.ReadString('\n')
					resp, err = Instance.SubmitSMS(strings.ReplaceAll(strings.ReplaceAll(text, "\n", ""), "\r", ""))
					continue
				case "2":
					fmt.Printf("device lock -> %v\n", resp.VerifyUrl)
					os.Exit(2)
				default:
					fmt.Println("invalid input")
					os.Exit(2)
				}  */

			case client.SliderNeededError:
				logger.Warnf("please verify slider -> %v ", resp.VerifyURL)

				logger.Warn("input ticket: (Enter to submit)")
				text, _ = console.ReadString('\n')
				ticket := strings.TrimSpace(text)
				logger.Warn("input rand_str: (Enter to submit)")
				text, _ = console.ReadString('\n')
				randStr := strings.TrimSpace(text)

				if ticket == "" {
					fmt.Println("verify failed")
					os.Exit(2)
				}
				resp, err = Instance.SubmitCaptcha(ticket, randStr, strings.Split(strings.Split(resp.VerifyURL, "sid=")[1], "&")[0])
				continue

			case client.OtherLoginError, client.UnknownLoginError:
				logger.Fatalf("login failed: %v", resp.ErrorMessage)
			}

		}

		break
	}

	logger.Infof("qq login: %s", Instance.NickName())
	token, _ := Instance.Sig().Marshal()
	_ = os.WriteFile("session.token", token, 0o644)
}

// RefreshList 刷新联系人
func RefreshList() {
	logger.Info("start reload friends list")
	err := Instance.RefreshFriendCache()
	if err != nil {
		logger.WithError(err).Error("unable to load friends list")
	}
	logger.Infof("load %d friends", len(Instance.GetCachedAllFriendsInfo()))
	logger.Info("start reload groups list")
	err = Instance.RefreshAllGroupsInfo()
	if err != nil {
		logger.WithError(err).Error("unable to load groups list")
	}
	logger.Infof("load %d groups", len(Instance.GetCachedAllGroupsInfo()))
}

func MakeChan() {
	for _, gid := range config.GlobalConfig.QQ.Groups {
		Instance.NewGroupChan(gid)
	}
}

func StartService() {
	if Instance.start {
		return
	}

	Instance.start = true
	Instance.GroupMessageEvent.Subscribe(RouteMsg)
}

func RouteMsg(c *client.QQClient, msg *mirai.GroupMessage) {
	if msgChan, ok := Instance.Chats[int64(msg.GroupUin)]; ok {
		logger.Infof("[%s]: %s", msg.Sender.Nickname, msg.ToString())
		msgChan.tempChan <- msg
	}
}
