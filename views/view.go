package views

import (
	"github.com/erroneousboat/termui"

	"github.com/erroneousboat/slack-term/components"
	"github.com/erroneousboat/slack-term/config"
	"github.com/erroneousboat/slack-term/service"

        "errors"
        "fmt"
)

type View struct {
	Config   *config.Config
	Input    *components.Input
	Chat     *components.Chat
	Threads  *components.Threads
	Mode     *components.Mode
	Debug    *components.Debug
}

func CreateView(config *config.Config, svc *service.SlackService) (*View, error) {
	// Create Input component
	input := components.CreateInputComponent()
        sideBarHeight := termui.TermHeight() - input.Par.Height

	// Threads: create component
	threads := components.CreateThreadsComponent(sideBarHeight)

	// Chat: create the component
	chat := components.CreateChatComponent(input.Par.Height)

        // Chat: let the chat know about the channels
        // Also sets up the svc for below calls
	slackchans, err := svc.InitializeChannels()
	if err == nil {
                for _, c := range slackchans {
                        // TODO: rename this
                        chat.ChanToAbbrev(c, "")
                }
	} else {
                return nil, errors.New(fmt.Sprintf("oops %s", slackchans))
        }

	// Chat: fill the component
	// msgs, thr, err := svc.GetMessages(
	msgs, _, err := svc.GetInitialMessages(
		chat.GetMaxItems(),
	)
	if err != nil {
		return nil, err
	}

	// Chat: set messages in component
	chat.SetMessages(msgs)


	chat.SetBorderLabel(
		chat.GetCurrentChannelString(),
	)

	// Threads: set threads in component
        // TODO
	// if len(thr) > 0 {

	// 	// Make the first thread the current Channel
	// 	threads.SetChannels(
	// 		append( channels,
	// 			thr...,
	// 		),
	// 	)
	// }

	// Debug: create the component
	debug := components.CreateDebugComponent(input.Par.Height)

	// Mode: create the component
	mode := components.CreateModeComponent()

	view := &View{
		Config:   config,
		Input:    input,
		Threads:  threads,
		Chat:     chat,
		Mode:     mode,
		Debug:    debug,
	}

	return view, nil
}

func (v *View) Refresh() {
	termui.Render(
		v.Input,
		v.Chat,
		v.Threads,
		v.Mode,
	)
}
