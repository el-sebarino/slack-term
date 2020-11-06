package context

import (
	"errors"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/0xAX/notificator"
	"github.com/erroneousboat/termui"
	termbox "github.com/nsf/termbox-go"

	"github.com/erroneousboat/slack-term/config"
	"github.com/erroneousboat/slack-term/service"
	"github.com/erroneousboat/slack-term/views"
)

const (
	CommandMode = "command"
	InsertMode  = "insert"
	SearchMode  = "search"

	ChatFocus = iota
	ThreadFocus
)

type AppContext struct {
	Version    string
	Usage      string
	EventQueue chan termbox.Event
	Service    *service.SlackService
	Body       *termui.Grid
	View       *views.View
	Config     *config.Config
	Debug      bool
	Focus      int
	Notify     *notificator.Notificator
}

// CreateAppContext creates an application context which can be passed
// and referenced througout the application
func CreateAppContext(flgConfig string, flgToken string, flgDebug bool, version string, usage string) (*AppContext, error) {
	if flgDebug {
		go func() {
			http.ListenAndServe(":6060", nil)
		}()
	}

	// Loading screen
	views.Loading()

	// Load config
	config, err := config.NewConfig(flgConfig)
	if err != nil {
		return nil, err
	}

	// When slack token isn't set in the config file, we'll check
	// the command-line flag or the environment variable
	if config.SlackToken == "" {
		if flgToken != "" {
			config.SlackToken = flgToken
		} else {
			config.SlackToken = os.Getenv("SLACK_TOKEN")
		}
	}

	// Create desktop notifier
	var notify *notificator.Notificator
	if config.Notify != "" {
		notify = notificator.New(notificator.Options{AppName: "slack-term"})
		if notify == nil {
			return nil, errors.New(
				"desktop notifications are not supported for your OS",
			)
		}
	}

	// Create Service
	svc, err := service.NewSlackService(config)
	if err != nil {
		return nil, err
	}

	// Create the main view
	view, err := views.CreateView(config, svc)
	if err != nil {
		return nil, err
	}

	 columns := []*termui.Row{}

	// Setup the interface
	if flgDebug {
		columns = append(
			columns,
			[]*termui.Row{
				termui.NewCol(config.MainWidth-5, 0, view.Chat),
				// Don't know why -7
				termui.NewCol(config.MainWidth-7, 0, view.Debug),
			}...,
		)
	} else {
		columns = append(
			columns,
			[]*termui.Row{
				termui.NewCol(config.MainWidth, 0, view.Chat),
			}...,
		)
	}

	termui.Body.AddRows(
		termui.NewRow(columns...),
		termui.NewRow(

			termui.NewCol(config.MainWidth, 0, view.Input),
		),
	)

	termui.Body.Align()
	termui.Render(termui.Body)

	return &AppContext{
		Version:    version,
		Usage:      usage,
		EventQueue: make(chan termbox.Event, 20),
		Service:    svc,
		Body:       termui.Body,
		View:       view,
		Config:     config,
		Debug:      flgDebug,
		Focus:      ChatFocus,
		Notify:     notify,
	}, nil
}
