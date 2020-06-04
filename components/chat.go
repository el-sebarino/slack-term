package components

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"github.com/erroneousboat/termui"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/erroneousboat/slack-term/config"
        "github.com/slack-go/slack"
)

// Chat is the definition of a Chat component
type Chat struct {
	List     *termui.List
	Messages map[string]Message
	Offset   int

        // map "abbrev"s / short names of channels, to channels
        AbbrevCache     map[string]slack.Channel
        CacheCounter    int

        ThreadAbbrevCache map[string](map[string]string)
        ThreadAbbrevCacheCounter map[string]int

        CurrentAbbrev   string
}

// CreateChatComponent is the constructor for the Chat struct
func CreateChatComponent(inputHeight int) *Chat {
	chat := &Chat{
		List:     termui.NewList(),
		Messages: make(map[string]Message),
		Offset:   0,
                AbbrevCache: make(map[string]slack.Channel),
                CacheCounter: 0,
                ThreadAbbrevCache: make(map[string](map[string]string)),
                ThreadAbbrevCacheCounter: make(map[string]int),
	}

	chat.List.Height = termui.TermHeight() - inputHeight
	chat.List.Overflow = "wrap"

	return chat
}

func getChr(id int) string {
        if (id < 26) {
                return fmt.Sprintf("%c", 'a' + id)
        } else {
                x := id % 26
                return fmt.Sprintf("%c%s", 'a' + x, getChr(id / 26))
        }
}

func (c* Chat) ThreadAndChanToThreadAbbrev (ab string, th string) string {
        threadsForThisChannel, found := c.ThreadAbbrevCache[ab]
        if found {
                for k := range threadsForThisChannel {
                        if threadsForThisChannel[k] == th {
                                return k
                        }
                }
                newKey := fmt.Sprintf("%s", getChr(c.ThreadAbbrevCacheCounter[ab]))
                c.ThreadAbbrevCacheCounter[ab]++ 
                c.ThreadAbbrevCache[ab][newKey] = th
                return newKey
        } else {
                newKey := fmt.Sprintf("%s", getChr(0))
                c.ThreadAbbrevCacheCounter[ab] = 1
                c.ThreadAbbrevCache[ab][newKey] = th
                return newKey
        }
}

// map a channel to unique identifier,"hash"
func (c* Chat) ChanToAbbrev(ch slack.Channel, th string) string {
        for k := range c.AbbrevCache {
                if c.AbbrevCache[k].ID == ch.ID {
                        return k
                }
        }
        newKey := fmt.Sprintf("%d", c.CacheCounter)
        c.AbbrevCache[newKey] = ch
        c.CacheCounter++

        return newKey
}

func (c* Chat) AbbrevToChan(a string) (slack.Channel, string, error) {
        ch := c.AbbrevCache[a] 
        // TODO: error handle
        return ch, nil
}

func (c* Chat) GetChannelsList() []string {
        var keys []string
        var chanStrings []string

        // TODO better way to sort? more concise?
        for k := range c.AbbrevCache {
                keys = append(keys, k)
        }
        // TODO represent the "abbrev" chans as numbers? This is sorting "numericallly"
        sort.Slice(keys, func(i, j int) bool { return (len(keys[i]) == len(keys[j]) && keys[i] < keys[j]) || len(keys[i]) < len(keys[j])  } )
        for _, k := range keys {
                chanStrings = append(chanStrings, fmt.Sprintf("%s", c.GetChannelString(k)))
        }
        return chanStrings
}

func (c* Chat) SetChannel(ch string) error {
        _, err := c.AbbrevToChan(ch, "") 
        if err != nil {
                return err
        }
        c.CurrentAbbrev = ch
        return nil
}

// TODO: make this same as wowstring in messages
func (c* Chat) GetChannelString(ch string) string {
        channel, err := c.AbbrevToChan(ch)
        if err != nil {
                return "???"
        }
        return fmt.Sprintf("(%s) %s", ch, channel.Name)
}

func (c* Chat) GetCurrentChannelString() string {
        return c.GetChannelString(c.CurrentAbbrev)
}

func (c* Chat) GetCurrentChannel() slack.Channel {
        // TODO error?
        channel, _ := c.AbbrevToChan(c.CurrentAbbrev)
        return channel
}

func (c* Chat) GetAbbrev(m Message) string {
        return fmt.Sprintf("(%s) ", c.ChanToAbbrev(m.Chan))
}
// Buffer implements interface termui.Bufferer
func (c *Chat) Buffer() termui.Buffer {
	// Convert Messages into termui.Cell
	cells := c.MessagesToCells(c.Messages)

	// We will create an array of Line structs, this allows us
	// to more easily render the items in a list. We will range
	// over the cells we've created and create a Line within
	// the bounds of the Chat pane
	type Line struct {
		cells []termui.Cell
	}

	lines := []Line{}
	line := Line{}

	// When we encounter a newline or, are at the bounds of the chat view we
	// stop iterating over the cells and add the line to the line array
	x := 0
	for _, cell := range cells {

		// When we encounter a newline we add the line to the array
		if cell.Ch == '\n' {
			lines = append(lines, line)

			// Reset for new line
			line = Line{}
			x = 0
			continue
		}

		if x+cell.Width() > c.List.InnerBounds().Dx() {
			lines = append(lines, line)

			// Reset for new line
			line = Line{}
			x = 0
		}

		line.cells = append(line.cells, cell)
		x += cell.Width()
	}

	// Append the last line to the array when we didn't encounter any
	// newlines or were at the bounds of the chat view
	lines = append(lines, line)

	// We will print lines bottom up, it will loop over the lines
	// backwards and for every line it'll set the cell in that line.
	// Offset is the number which allows us to begin printing the
	// line above the last line.
	buf := c.List.Buffer()
	linesHeight := len(lines)
	paneMinY := c.List.InnerBounds().Min.Y
	paneMaxY := c.List.InnerBounds().Max.Y

	currentY := paneMaxY - 1
	for i := (linesHeight - 1) - c.Offset; i >= 0; i-- {

		if currentY < paneMinY {
			break
		}

		x := c.List.InnerBounds().Min.X
		for _, cell := range lines[i].cells {
			buf.Set(x, currentY, cell)
			x += cell.Width()
		}

		// When we're not at the end of the pane, fill it up
		// with empty characters
		for x < c.List.InnerBounds().Max.X {
			buf.Set(
				x, currentY,
				termui.Cell{
					Ch: ' ',
					Fg: c.List.ItemFgColor,
					Bg: c.List.ItemBgColor,
				},
			)
			x += runewidth.RuneWidth(' ')
		}
		currentY--
	}

	// If the space above currentY is empty we need to fill
	// it up with blank lines, otherwise the List object will
	// render the items top down, and the result will mix.
	for currentY >= paneMinY {
		x := c.List.InnerBounds().Min.X
		for x < c.List.InnerBounds().Max.X {
			buf.Set(
				x, currentY,
				termui.Cell{
					Ch: ' ',
					Fg: c.List.ItemFgColor,
					Bg: c.List.ItemBgColor,
				},
			)
			x += runewidth.RuneWidth(' ')
		}
		currentY--
	}

	return buf
}

// GetHeight implements interface termui.GridBufferer
func (c *Chat) GetHeight() int {
	return c.List.Block.GetHeight()
}

// SetWidth implements interface termui.GridBufferer
func (c *Chat) SetWidth(w int) {
	c.List.SetWidth(w)
}

// SetX implements interface termui.GridBufferer
func (c *Chat) SetX(x int) {
	c.List.SetX(x)
}

// SetY implements interface termui.GridBufferer
func (c *Chat) SetY(y int) {
	c.List.SetY(y)
}

// GetMaxItems return the maximal amount of items can fit in the Chat
// component
func (c *Chat) GetMaxItems() int {
	return c.List.InnerBounds().Max.Y - c.List.InnerBounds().Min.Y
}

// SetMessages will put the provided messages into the Messages field of the
// Chat view
func (c *Chat) SetMessages(messages []Message) {
        // sets the channel to last added message
	// Reset offset first, when scrolling in view and changing channels we
	// want the offset to be 0 when loading new messages
	c.Offset = 0
        var last_message Message
	for _, msg := range messages {
		c.AddMessage(msg)
                last_message = msg
	}
        // Sets the channel to last added message
        c.CurrentAbbrev=c.GetAbbrev(last_message)
}

// AddMessage adds a single message to Messages
func (c *Chat) AddMessage(message Message) {
	c.Messages[message.ID] = message
}

// AddReply adds a single reply to a parent thread, it also sets
// the thread separator
func (c *Chat) AddReply(parentID string, message Message) {
	// It is possible that a message is received but the parent is not
	// present in the chat view
	if _, ok := c.Messages[parentID]; ok {
		message.Thread = "  "
		c.Messages[parentID].Messages[message.ID] = message
	} else {
		c.AddMessage(message)
	}
}

// IsNewThread check whether a message that is going to be added as
// a child to a parent message, is the first one or not
func (c *Chat) IsNewThread(parentID string) bool {
	if parent, ok := c.Messages[parentID]; ok {
		if len(parent.Messages) > 0 {
			return true
		}
	}
	return false
}

// ClearMessages clear the c.Messages
func (c *Chat) ClearMessages() {
	c.Messages = make(map[string]Message)
}

// ScrollUp will render the chat messages based on the Offset of the Chat
// pane.
//
// Offset is 0 when scrolled down. (we loop backwards over the array, so we
// start with rendering last item in the list at the maximum y of the Chat
// pane). Increasing the Offset will thus result in substracting the offset
// from the len(Chat.Messages).
func (c *Chat) ScrollUp() {
	c.Offset = c.Offset + 10

	// Protect overscrolling
	if c.Offset > len(c.Messages) {
		c.Offset = len(c.Messages)
	}
}

// ScrollDown will render the chat messages based on the Offset of the Chat
// pane.
//
// Offset is 0 when scrolled down. (we loop backwards over the array, so we
// start with rendering last item in the list at the maximum y of the Chat
// pane). Increasing the Offset will thus result in substracting the offset
// from the len(Chat.Messages).
func (c *Chat) ScrollDown() {
	c.Offset = c.Offset - 10

	// Protect overscrolling
	if c.Offset < 0 {
		c.Offset = 0
	}
}

// SetBorderLabel will set Label of the Chat pane to the specified string
func (c *Chat) SetBorderLabel(channelName string) {
	c.List.BorderLabel = channelName
}

// MessagesToCells is a wrapper around MessageToCells to use for a slice of
// of type Message
func (c *Chat) MessagesToCells(msgs map[string]Message) []termui.Cell {
	cells := make([]termui.Cell, 0)
	sortedMessages := SortMessages(msgs)

	for i, msg := range sortedMessages {
		cells = append(cells, c.MessageToCells(msg)...)

		if len(msg.Messages) > 0 {
			cells = append(cells, termui.Cell{Ch: '\n'})
			cells = append(cells, c.MessagesToCells(msg.Messages)...)
		}

		// Add a newline after every message
		if i < len(sortedMessages)-1 {
			cells = append(cells, termui.Cell{Ch: '\n'})
		}
	}

	return cells
}


// Get an IRC style string from a slack.Channel
// Could also go in slack.channel
func (m Message) GetWOWString() string {
        var wowString string
        var c slack.Channel
        var channelName string

        c = m.Chan

        if m.Thread != "" {
                channelName = fmt.Sprintf("%s/%s", c.Name, m.Thread)
        } else {
                channelName = fmt.Sprintf("%s", c.Name)
        }
        
        // Find out the type of the channel
        if c.IsChannel {
                // [random] joe:
                wowString = fmt.Sprintf("[%s] %s", channelName, m.Name)
        } else if c.IsGroup {
                if c.IsMpIM {
                        // ??
                        // [joe-fred-lisa] fred:
                        wowString = fmt.Sprintf("[%s] %s", channelName, m.Name)
                } else {
                        wowString = fmt.Sprintf("[%s] %s", channelName, m.Name)
                }
        } else if c.IsIM {
                // joe:
                wowString = fmt.Sprintf("%s", m.Name)
        }
        return fmt.Sprintf("%s: ", wowString)
}


// MessageToCells will convert a Message struct to termui.Cell
//
// We're building parts of the message individually, or else DefaultTxBuilder
// will interpret potential markdown usage in a message as well.
func (c *Chat) MessageToCells(msg Message) []termui.Cell {
	cells := make([]termui.Cell, 0)

	// When msg.Time and msg.Name are empty (in the case of attachments)
	// don't add the time and name parts.
	if (msg.Time != time.Time{} && msg.Name != "") {
		// Time
		cells = append(cells, termui.DefaultTxBuilder.Build(
			msg.GetTime(),
			termui.ColorDefault, termui.ColorDefault)...,
		)

                // Abbrev
		cells = append(cells, termui.DefaultTxBuilder.Build(
			c.GetAbbrev(msg),
			termui.ColorDefault, termui.ColorDefault)...,
		)

                // WOW style name
		cells = append(cells, termui.DefaultTxBuilder.Build(
			msg.GetWOWString(),
			termui.ColorDefault, termui.ColorDefault)...,
		)

		// Thread
		cells = append(cells, termui.DefaultTxBuilder.Build(
			msg.GetThread(),
			termui.ColorDefault, termui.ColorDefault)...,
		)

		// Name
		//cells = append(cells, termui.DefaultTxBuilder.Build(
		//	msg.GetName(),
		//	termui.ColorDefault, termui.ColorDefault)...,
		//)
	}

	// Hack, in order to get the correct fg and bg attributes. This is
	// because the readAttr function in termui is unexported.
	txCells := termui.DefaultTxBuilder.Build(
		msg.GetContent(),
		termui.ColorDefault, termui.ColorDefault,
	)

	// Text
	for _, r := range msg.Content {
		cells = append(
			cells,
			termui.Cell{
				Ch: r,
				Fg: txCells[0].Fg,
				Bg: txCells[0].Bg,
			},
		)
	}

	return cells
}

// Help shows the usage and key bindings in the chat pane
func (c *Chat) Help(usage string, cfg *config.Config) {
	msgUsage := Message{
		ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
		Content: usage,
	}

	c.Messages[msgUsage.ID] = msgUsage

	for mode, mapping := range cfg.KeyMap {
		msgMode := Message{
			ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
			Content: fmt.Sprintf("%s", strings.ToUpper(mode)),
		}
		c.Messages[msgMode.ID] = msgMode

		msgNewline := Message{
			ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
			Content: "",
		}
		c.Messages[msgNewline.ID] = msgNewline

		var keys []string
		for k := range mapping {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			msgKey := Message{
				ID:      fmt.Sprintf("%d", time.Now().UnixNano()),
				Content: fmt.Sprintf("    %-12s%-15s", k, mapping[k]),
			}
			c.Messages[msgKey.ID] = msgKey
		}

		msgNewline.ID = fmt.Sprintf("%d", time.Now().UnixNano())
		c.Messages[msgNewline.ID] = msgNewline
	}
}
