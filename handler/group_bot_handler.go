package handler

import (
	"fmt"
	"github.com/eatmoreapple/openwechat"
	"github.com/robfig/cron"
	"log"
	"os"
	loader "sf-bot/handler/load"
	"sf-bot/handler/service"
	"strings"
)

var groupHandler *GroupBotHandler

type GroupBotHandler struct {
	closeReplySuffix bool
	groupNames       []string
	syncGroups       map[string]*openwechat.Group
	aiteMe           string
	mode             string
	morningPaperMode SendMode
	moyu             bool
}

// return is break
func (g *GroupBotHandler) recvCommand(command string, f func(i string)) bool {
	if strings.Contains(command, "#关闭后缀#") {
		g.closeReplySuffix = true
		f(g.infos())
		return true
	}

	if strings.Contains(command, "#开启后缀#") {
		g.closeReplySuffix = false
		f(g.infos())
		return true
	}

	if !g.closeReplySuffix {
		f(command + "\n" + "------------\n" + "已收到，我知道你很急，但是你先别急！有问题联系Alex_")
	}
	return false
}

func (g *GroupBotHandler) exists(name string) bool {
	//split := strings.Split(loader.GroupName(), ",")
	for _, key := range g.groupNames {
		if strings.Contains(name, key) {
			return true
		}
	}
	return false
}

func (g *GroupBotHandler) infos() string {
	info := ""
	info = info + "----setting----\n后缀："
	if g.closeReplySuffix {
		info += "已关闭\n"
	} else {
		info += "已开启\n"
	}
	info = info + "----setting----"
	return info
}

func (g *GroupBotHandler) recv(ctx *openwechat.MessageContext) {

	if ctx.IsText() {

		log.Printf("receive the msg = %s \n", ctx.Content)

		if strings.Contains(ctx.Content, g.aiteMe) {

			//sender in group
			user, _ := ctx.SenderInGroup()
			fmt.Printf("the user_info = %s \n", user)
			id := user.NickName
			fmt.Printf("nickName = %s \n ", id)

			msg := strings.ReplaceAll(ctx.Content, g.aiteMe, "")
			if command := g.recvCommand(msg, func(info string) { ctx.ReplyText(info) }); command {
				return
			}

			if sender, err := ctx.Sender(); err == nil {
				if group, b := sender.AsGroup(); b {
					g.syncAsk(group, id, msg)
					return
				}
			}
		}

	} else {
		//todo 2023/6/4 lamkeizyi - 其他消息暂未支持
	}
}

// 异步不适用GPT
func (g *GroupBotHandler) syncAsk(group *openwechat.Group, senderNickName, msg string) {
	g.syncGroups[group.NickName] = group
	if g.mode == "gpt" {
		answer := agent.AskAgent().Ask(groupPrefix+senderNickName, msg)
		if _, err := wx.SendTextToGroup(group, "@"+senderNickName+" \n"+answer); err != nil {
			log.Printf("wx.SendTextToGroup has error  = %s ", err)
			return
		}

	} else {
		go func() {
			answer := agent.AskAgent().Ask(senderNickName, msg)
			wx.SendTextToGroup(group, answer)
		}()
	}

}

// 发送图片
func (g *GroupBotHandler) sendPic(fileName string) error {
	return g.send(fileName, PIC)
}

func (g *GroupBotHandler) sendText(text string) error {
	return g.send(text, TEXT)
}

type SendMode int

const (
	VIDEO SendMode = 0
	TEXT  SendMode = 1
	PIC   SendMode = 2
)

func (g *GroupBotHandler) send(s string, mode SendMode) error {
	groups, err := wx.Groups()
	if err != nil {
		log.Println("get groups error ", err)
	} else {
		for _, v := range groups {
			for _, name := range g.groupNames {
				if strings.Contains(v.NickName, name) {
					//fmt.Println(fileName)

					if mode == PIC {
						open, _ := os.Open(s)
						if _, err := v.SendImage(open); err != nil {
							log.Println("send pic has error")
							return err
						}
					}

					if mode == TEXT {
						if _, err := v.SendText(s); err != nil {
							log.Println("send text has error")
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

func CreateGroupBotHandler() *GroupBotHandler {
	if loader.LoadBool("group.enable") {
		groupHandler = &GroupBotHandler{
			aiteMe:           loader.Load("group.aite_me"),
			closeReplySuffix: false,
			groupNames:       strings.Split(loader.GroupName(), ","),
			syncGroups:       make(map[string]*openwechat.Group),
			mode:             "gpt",
			morningPaperMode: SendMode(loader.LoadInt("group.morning_paper_mode")),
		}

		go func() {
			// 2023/6/6 lamkeizyi - 工作日9点半运行
			if loader.LoadBool("group.morning_paper") {
				fmt.Println("initialize 9:30 timer")
				c := cron.New()
				c.AddFunc("1 30 9 ? * 2,3,4,5,6", func() {
					if groupHandler.morningPaperMode == TEXT {
						if dayTextService, err := service.GetPicDayTextService(); err == nil {
							log.Println("cuz has error that send pic . convert to text to sending... ")
							groupHandler.sendText(dayTextService.ToString())
						}
					} else {
						service.StartMoyuPicDayService(func(name string) {
							if err := groupHandler.sendPic(name); err != nil {
								//convert to text
								if dayTextService, err := service.GetPicDayTextService(); err == nil {
									log.Println("cuz has error that send pic . convert to text to sending... ")
									groupHandler.sendText(dayTextService.ToString())
								}

							}
						})
					}
				})
				c.Start()
				select {}
			}

		}()

		go func() {
			// 2023/6/6 lamkeizyi - 工作日10点
			if loader.LoadBool("group.moyu") {
				fmt.Println("initialize 10:00 timer")
				c := cron.New()
				c.AddFunc("1 0 10 ? * 2,3,4,5,6", func() {
					service.StartMoyuPicDayService(func(name string) {
						groupHandler.sendPic(name)
					})
				})
				c.Start()
				select {}
			}
		}()

		return groupHandler
	}
	return nil
}
