package views

import (
	"github.com/erroneousboat/termui"

	"github.com/erroneousboat/slack-term/components"
	"github.com/erroneousboat/slack-term/config"
	"github.com/erroneousboat/slack-term/service"
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

	// Channels: fill the component
	//slackChans, err := svc.GetChannels()
	// if err != nil {
	// 	return nil, err
	// }

	// Threads: create component
	threads := components.CreateThreadsComponent(sideBarHeight)

	// Chat: create the component
	chat := components.CreateChatComponent(input.Par.Height)

	// Chat: fill the component
	// msgs, thr, err := svc.GetMessages(
	msgs, _, err := svc.GetMessages(
                "TODO: get channel, or make a function to get fireshose",
		chat.GetMaxItems(),
	)
	if err != nil {
		return nil, err
	}

	// Chat: set messages in component
	chat.SetMessages(msgs)

	chat.SetBorderLabel(
		"Firehose",
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
