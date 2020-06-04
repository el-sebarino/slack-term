package handlers

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0xAX/notificator"
	"github.com/erroneousboat/termui"
	termbox "github.com/nsf/termbox-go"
	"github.com/slack-go/slack"

	"github.com/erroneousboat/slack-term/components"
	"github.com/erroneousboat/slack-term/config"
	"github.com/erroneousboat/slack-term/context"
	"github.com/erroneousboat/slack-term/views"
)

var scrollTimer *time.Timer
var notifyTimer *time.Timer
// var (
//         // TODO make it functional
//         COMMANDS = []string {
//                 "l", // LIST
//         }
// )


// actionMap binds specific action names to the function counterparts,
// these action names can then be used to bind them to specific keys
// in the Config.
var actionMap = map[string]func(*context.AppContext){
	"space":               actionSpace,
	"backspace":           actionBackSpace,
	"delete":              actionDelete,
	"cursor-right":        actionMoveCursorRight,
	"cursor-left":         actionMoveCursorLeft,
	"send":                actionSend,
	"quit":                actionQuit,
	"mode-insert":         actionInsertMode,
	"mode-command":        actionCommandMode,
	"mode-search":         actionSearchMode,
	"clear-input":         actionClearInput,
	"thread-up":           actionMoveCursorUpThreads,
	"thread-down":         actionMoveCursorDownThreads,
	"chat-up":             actionScrollUpChat,
	"chat-down":           actionScrollDownChat,
	"help":                actionHelp,
}

// Initialize will start a combination of event handlers and 'background tasks'
func Initialize(ctx *context.AppContext) {

	// Keyboard events
	eventHandler(ctx)

	// RTM incoming events
	messageHandler(ctx)

	// User presence
	go actionSetPresenceAll(ctx)
}

// eventHandler will handle events created by the user
func eventHandler(ctx *context.AppContext) {
	go func() {
		for {
			ctx.EventQueue <- termbox.PollEvent()
		}
	}()

	go func() {
		for {
			ev := <-ctx.EventQueue
			handleTermboxEvents(ctx, ev)
			handleMoreTermboxEvents(ctx, ev)

			// Place your debugging statements here
			// if ctx.Debug {
			// 	ctx.View.Debug.Println(
			// 		"event received",
			// 	)
			// }
		}
	}()
}

func handleTermboxEvents(ctx *context.AppContext, ev termbox.Event) bool {
	switch ev.Type {
	case termbox.EventKey:
		actionKeyEvent(ctx, ev)
	case termbox.EventResize:
		actionResizeEvent(ctx, ev)
	}

	return true
}

func handleMoreTermboxEvents(ctx *context.AppContext, ev termbox.Event) bool {
	for {
		select {
		case ev := <-ctx.EventQueue:
			ok := handleTermboxEvents(ctx, ev)
			if !ok {
				return false
			}
		default:
			return true
		}
	}
}

// messageHandler will handle events created by the service
func messageHandler(ctx *context.AppContext) {
	go func() {
		for {
			select {
			case rtmEvent := <-ctx.Service.RTM.IncomingEvents:
				switch ev := rtmEvent.Data.(type) {
				case *slack.MessageEvent:

					// Construct message
					msg, err := ctx.Service.CreateMessageFromMessageEvent(ev, ev.Channel)
					if err != nil {
						continue
					}

					// Add message to the CHAT firehose

                                        // Get the thread timestamp of the event, we need to
                                        // check the previous message as well, because edited
                                        // message don't have the thread timestamp
                                        var threadTimestamp string
                                        if ev.ThreadTimestamp != "" {
                                                threadTimestamp = ev.ThreadTimestamp
                                        } else if ev.PreviousMessage != nil && ev.PreviousMessage.ThreadTimestamp != "" {
                                                threadTimestamp = ev.PreviousMessage.ThreadTimestamp
                                        } else {
                                                threadTimestamp = ""
                                        }

                                        // When timestamp isn't set this is a thread reply,
                                        // handle as such
                                        if threadTimestamp != "" {
                                                ctx.View.Chat.AddReply(threadTimestamp, msg)
                                        } else if threadTimestamp == "" && ctx.Focus == context.ChatFocus {
                                                // TODO  should be part of addMessage?
                                                ctx.View.Chat.AddMessage(msg)
                                                ctx.View.Chat.SetBorderLabel(ctx.View.Chat.GetCurrentChannelString())
                                                termui.Render(ctx.View.Chat)
                                        }

                                        // we (mis)use actionChangeChannel, to rerender, the
                                        // view when a new thread has been started
                                        if ctx.View.Chat.IsNewThread(threadTimestamp) {
                                                // TODO
                                        } else {
                                                termui.Render(ctx.View.Chat)
                                        }

                                        // TODO: set Chat.Offset to 0, to automatically scroll
                                        // down?

					// Set new message indicator for channel, I'm leaving
					// this here because I also want to be notified when
					// I'm currently in a channel but not in the terminal
					// window (tmux). But only create a notification when
					// it comes from someone else but the current user.
					if ev.User != ctx.Service.CurrentUserID {
						actionNewMessage(ctx, ev)
					}
				case *slack.PresenceChangeEvent:
                                        // TODO:
				case *slack.RTMError:
					ctx.View.Debug.Println(
						ev.Error(),
					)
				}
			}
		}
	}()
}

func actionKeyEvent(ctx *context.AppContext, ev termbox.Event) {

	keyStr := getKeyString(ev)

	// Get the action name (actionStr) from the key that
	// has been pressed. If this is found try to uncover
	// the associated function with this key and execute
	// it.
	actionStr, ok := ctx.Config.KeyMap[ctx.Mode][keyStr]
	if ok {
		action, ok := actionMap[actionStr]
		if ok {
			action(ctx)
		}
	} else {
		if ctx.Mode == context.InsertMode && ev.Ch != 0 {
			actionInput(ctx.View, ev.Ch)
		}
	}
}

func actionResizeEvent(ctx *context.AppContext, ev termbox.Event) {
	// When terminal window is too small termui will panic, here
	// we won't resize when the terminal window is too small.
	if termui.TermWidth() < 25 || termui.TermHeight() < 5 {
		return
	}

	termui.Body.Width = termui.TermWidth()

	// Vertical resize components
	ctx.View.Chat.List.Height = termui.TermHeight() - ctx.View.Input.Par.Height
	ctx.View.Debug.List.Height = termui.TermHeight() - ctx.View.Input.Par.Height

	termui.Body.Align()
	termui.Render(termui.Body)
}

func actionRedrawGrid(ctx *context.AppContext, threads bool, debug bool) {
	termui.Clear()
	termui.Body = termui.NewGrid()
	termui.Body.X = 0
	termui.Body.Y = 0
	termui.Body.BgColor = termui.ThemeAttr("bg")
	termui.Body.Width = termui.TermWidth()

	columns := []*termui.Row{
	}

	if threads && debug {
		columns = append(
			columns,
			[]*termui.Row{
				termui.NewCol(ctx.Config.MainWidth-ctx.Config.ThreadsWidth-3, 0, ctx.View.Chat),
				termui.NewCol(ctx.Config.ThreadsWidth, 0, ctx.View.Threads),
				termui.NewCol(3, 0, ctx.View.Debug),
			}...,
		)
	} else if threads {
		columns = append(
			columns,
			[]*termui.Row{
				termui.NewCol(ctx.Config.MainWidth-ctx.Config.ThreadsWidth, 0, ctx.View.Chat),
				termui.NewCol(ctx.Config.ThreadsWidth, 0, ctx.View.Threads),
			}...,
		)
	} else if debug {
		columns = append(
			columns,
			[]*termui.Row{
				termui.NewCol(ctx.Config.MainWidth-5, 0, ctx.View.Chat),
				termui.NewCol(ctx.Config.MainWidth-6, 0, ctx.View.Debug),
			}...,
		)
	} else {
		columns = append(
			columns,
			[]*termui.Row{
				termui.NewCol(ctx.Config.MainWidth, 0, ctx.View.Chat),
			}...,
		)
	}

	termui.Body.AddRows(
		termui.NewRow(columns...),
		termui.NewRow(
			termui.NewCol(ctx.Config.SidebarWidth, 0, ctx.View.Mode),
			termui.NewCol(ctx.Config.MainWidth, 0, ctx.View.Input),
		),
	)

	termui.Body.Align()
	termui.Render(termui.Body)
}

func actionInput(view *views.View, key rune) {
	view.Input.Insert(key)
	termui.Render(view.Input)
}

func actionClearInput(ctx *context.AppContext) {
	// Clear input
	ctx.View.Input.Clear()
	ctx.View.Refresh()

	// Set command mode
	actionCommandMode(ctx)
}

func actionSpace(ctx *context.AppContext) {
	actionInput(ctx.View, ' ')
}

func actionBackSpace(ctx *context.AppContext) {
	ctx.View.Input.Backspace()
	termui.Render(ctx.View.Input)
}

func actionDelete(ctx *context.AppContext) {
	ctx.View.Input.Delete()
	termui.Render(ctx.View.Input)
}

func actionMoveCursorRight(ctx *context.AppContext) {
	ctx.View.Input.MoveCursorRight()
	termui.Render(ctx.View.Input)
}

func actionMoveCursorLeft(ctx *context.AppContext) {
	ctx.View.Input.MoveCursorLeft()
	termui.Render(ctx.View.Input)
}

func isChannelSet(input string) (bool, string, string, string) {
       re := regexp.MustCompile(`^\s*/(\d+)([a-z]*)`)
       matchedIndexPairs := re.FindSubmatchIndex([]byte(input))
        if matchedIndexPairs == nil {
                return false, "", "", input
        }
        left := matchedIndexPairs[2]
        middle := matchedIndexPairs[3]
        right := matchedIndexPairs[4]
        abbrev := input[left:middle]
        threadabbrev := input[middle:right]
        rest := input[right:]
        return true, abbrev, threadabbrev, rest
}

func isCmd(input string) (bool, string) {
       re := regexp.MustCompile(`^\s*/(\w+)`)
       matchedIndexPairs := re.FindSubmatchIndex([]byte(input))
        if matchedIndexPairs == nil {
                return false, ""
        }
        left := matchedIndexPairs[2]
        right := matchedIndexPairs[3]
        cmd := input[left:right]
        return true, cmd
        // rest := input[right:]
}

func actionList(ctx *context.AppContext) {
        ctx.View.Debug.Println( fmt.Sprintf("Listing channels"))
        chanStrings := ctx.View.Chat.GetChannelsList()
        for _, c := range chanStrings {
                ctx.View.Debug.Println( fmt.Sprintf(c))
        }
}

func actionSend(ctx *context.AppContext) {
	if !ctx.View.Input.IsEmpty() {

		// Clear message before sending, to combat
		// quick succession of actionSend
		message := ctx.View.Input.GetText()
		ctx.View.Input.Clear()
		termui.Render(ctx.View.Input)
               
                isChannelSetCmd, abbrev, thabbrev, message := isChannelSet(message)
                if isChannelSetCmd {
                        ctx.View.Chat.SetChannel(abbrev, thabbrev)
                        ctx.View.Debug.Println( fmt.Sprintf("Set channel to %s", abbrev))
                        ctx.View.Debug.Println( fmt.Sprintf(" channel is now %s", ctx.View.Chat.GetCurrentChannelString()))
                }
                        
                isCmd, commandStr := isCmd(message) 
                if isCmd {
                        ctx.View.Debug.Println( fmt.Sprintf("Got command '%s'", commandStr))
                        //TODO not hardcoding
                        switch commandStr {
                        case "l":
                                actionList(ctx)
                        case "q":
                                actionQuit(ctx)
                        default:
                                actionHelp(ctx)
                        }
                } else {
			if ctx.Focus == context.ChatFocus && len(message) > 0 {
                                ctx.View.Debug.Println( fmt.Sprintf("Sending on channel %s", ctx.View.Chat.GetCurrentChannelString()))
			 	 // err := ctx.Service.SendMessage(
                                        // ctx.View.Chat.GetCurrentChannel().ID,
			 	 // 	message,
			 	 // )
			 	 // if err != nil {
			 	 // 	ctx.View.Debug.Println( err.Error(),)
                                // }

			 }
		}

	}
}

// actionQuit will exit the program by using os.Exit, this is
// done because we are using a custom termui EvtStream. Which
// we won't be able to call termui.StopLoop() on. See main.go
// for the customEvtStream and why this is done.
func actionQuit(ctx *context.AppContext) {
	termbox.Close()
	os.Exit(0)
}

func actionInsertMode(ctx *context.AppContext) {
	ctx.Mode = context.InsertMode
	ctx.View.Mode.SetInsertMode()
}

func actionCommandMode(ctx *context.AppContext) {
	ctx.Mode = context.CommandMode
	ctx.View.Mode.SetCommandMode()
}

func actionSearchMode(ctx *context.AppContext) {
	ctx.Mode = context.SearchMode
	ctx.View.Mode.SetSearchMode()
}

func actionGetMessages(ctx *context.AppContext) {
	msgs, _, err := ctx.Service.GetMessages(
                "TODO: get channel ID",
		ctx.View.Chat.GetMaxItems(),
	)
	if err != nil {
		termbox.Close()
		log.Println(err)
		os.Exit(0)
	}

	ctx.View.Chat.SetMessages(msgs)

	termui.Render(ctx.View.Chat)
}

func actionChangeThread(ctx *context.AppContext) {
	// Clear messages from Chat pane
	ctx.View.Chat.ClearMessages()

	// The first channel in the Thread list is current Channel. Set context
	// Focus and messages accordingly.
	var err error
	msgs := []components.Message{}
	if ctx.View.Threads.SelectedChannel == 0 {
		ctx.Focus = context.ChatFocus

		msgs, _, err = ctx.Service.GetMessages(
                        "TODO: choose channel",
			ctx.View.Chat.GetMaxItems(),
		)
		if err != nil {
			termbox.Close()
			log.Println(err)
			os.Exit(0)
		}
	} else {
		ctx.Focus = context.ThreadFocus

		msgs, err = ctx.Service.GetMessageByID(
                        "TODO: get channel ID",
                        "TODO: get channel ID",
		)
		if err != nil {
			termbox.Close()
			log.Println(err)
			os.Exit(0)
		}
	}

	// Set messages for the channel
	ctx.View.Chat.SetMessages(msgs)

	termui.Render(ctx.View.Threads)
	termui.Render(ctx.View.Chat)
}

func actionMoveCursorUpThreads(ctx *context.AppContext) {
	go func() {
		if scrollTimer != nil {
			scrollTimer.Stop()
		}

		ctx.View.Threads.MoveCursorUp()
		termui.Render(ctx.View.Threads)

		scrollTimer = time.NewTimer(time.Second / 4)
		<-scrollTimer.C

		// Only actually change channel when the timer expires
		actionChangeThread(ctx)
	}()
}

func actionMoveCursorDownThreads(ctx *context.AppContext) {
	go func() {
		if scrollTimer != nil {
			scrollTimer.Stop()
		}

		ctx.View.Threads.MoveCursorDown()
		termui.Render(ctx.View.Threads)

		scrollTimer = time.NewTimer(time.Second / 4)
		<-scrollTimer.C

		// Only actually change thread when the timer expires
		actionChangeThread(ctx)
	}()
}

// actionNewMessage will set the new message indicator for a channel, and
// if configured will also display a desktop notification
func actionNewMessage(ctx *context.AppContext, ev *slack.MessageEvent) {

	// Terminal bell
	//fmt.Print("\a")

	// Desktop notification
	if ctx.Config.Notify == config.NotifyMention {
		if isMention(ctx, ev) {
			createNotifyMessage(ctx, ev)
		}
	} else if ctx.Config.Notify == config.NotifyAll {
		createNotifyMessage(ctx, ev)
	}
}

// actionPresenceAll will set the presence of the user list. Because the
// requests to the endpoint are rate limited we implement a timeout here.
func actionSetPresenceAll(ctx *context.AppContext) {
	for _, chn := range ctx.Service.Conversations {
		if chn.IsIM {
                        // TODO: do something with presence??
                        // presence, err := ctx.Service.GetUserPresence(chn.User)
			// if err != nil {
                        //         presence := "away"
			// }
			time.Sleep(1200 * time.Millisecond)
		}
	}
}

func actionScrollUpChat(ctx *context.AppContext) {
	ctx.View.Chat.ScrollUp()
	termui.Render(ctx.View.Chat)
}

func actionScrollDownChat(ctx *context.AppContext) {
	ctx.View.Chat.ScrollDown()
	termui.Render(ctx.View.Chat)
}

func actionHelp(ctx *context.AppContext) {
	ctx.View.Chat.ClearMessages()
	ctx.View.Chat.Help(ctx.Usage, ctx.Config)
	termui.Render(ctx.View.Chat)
}

// GetKeyString will return a string that resembles the key event from
// termbox. This is blatanly copied from termui because it is an unexported
// function.
//
// See:
// - https://github.com/gizak/termui/blob/a7e3aeef4cdf9fa2edb723b1541cb69b7bb089ea/events.go#L31-L72
// - https://github.com/nsf/termbox-go/blob/master/api_common.go
func getKeyString(e termbox.Event) string {
	var ek string

	k := string(e.Ch)
	pre := ""
	mod := ""

	if e.Mod == termbox.ModAlt {
		mod = "M-"
	}
	if e.Ch == 0 {
		if e.Key > 0xFFFF-12 {
			k = "<f" + strconv.Itoa(0xFFFF-int(e.Key)+1) + ">"
		} else if e.Key > 0xFFFF-25 {
			ks := []string{"<insert>", "<delete>", "<home>", "<end>", "<previous>", "<next>", "<up>", "<down>", "<left>", "<right>"}
			k = ks[0xFFFF-int(e.Key)-12]
		}

		if e.Key <= 0x7F {
			pre = "C-"
			k = string('a' - 1 + int(e.Key))
			kmap := map[termbox.Key][2]string{
				termbox.KeyCtrlSpace:     {"C-", "<space>"},
				termbox.KeyBackspace:     {"", "<backspace>"},
				termbox.KeyTab:           {"", "<tab>"},
				termbox.KeyEnter:         {"", "<enter>"},
				termbox.KeyEsc:           {"", "<escape>"},
				termbox.KeyCtrlBackslash: {"C-", "\\"},
				termbox.KeyCtrlSlash:     {"C-", "/"},
				termbox.KeySpace:         {"", "<space>"},
				termbox.KeyCtrl8:         {"C-", "8"},
			}
			if sk, ok := kmap[e.Key]; ok {
				pre = sk[0]
				k = sk[1]
			}
		}
	}

	ek = pre + mod + k
	return ek
}

// isMention check if the message event either contains a
// mention or is posted on an IM channel.
func isMention(ctx *context.AppContext, ev *slack.MessageEvent) bool {

	// Mentions have the following format:
	//	<@U12345|erroneousboat>
	// 	<@U12345>
	r := regexp.MustCompile(`\<@(\w+\|*\w+)\>`)
	matches := r.FindAllString(ev.Text, -1)
	for _, match := range matches {
		if strings.Contains(match, ctx.Service.CurrentUserID) {
			return true
		}
	}

	return false
}

func createNotifyMessage(ctx *context.AppContext, ev *slack.MessageEvent) {
	go func() {
		if notifyTimer != nil {
			notifyTimer.Stop()
		}

		// Only actually notify when time expires
		notifyTimer = time.NewTimer(time.Second * 2)
		<-notifyTimer.C

		var message string
                message = "TODO: what channel am i on"
		ctx.Notify.Push("slack-term", message, "", notificator.UR_NORMAL)
	}()
}
