package view

import (
	"io"

	"github.com/rivo/tview"
)

// TextViewLogWriter is a writer that writes logs to a TextView and scrolls it down
// so we "tail" the logs.
type textViewLogWriter struct {
	view   *tview.TextView
	writer io.Writer
}

func TextViewLogWriter(t *tview.TextView) io.Writer {
	return &textViewLogWriter{
		view:   t,
		writer: tview.ANSIWriter(t),
	}
}
func (t *textViewLogWriter) Write(b []byte) (int, error) {
	i, err := t.writer.Write(b)
	t.view.ScrollToEnd()
	return i, err
}

// Center creates a Flex at the center of another Primitive
func Center(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}
