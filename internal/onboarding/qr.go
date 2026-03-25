package onboarding

import (
	"fmt"
	"io"

	qrterminal "github.com/mdp/qrterminal/v3"
)

func PrintTerminalQRCode(w io.Writer, content string) {
	if w == nil || content == "" {
		return
	}
	qrterminal.GenerateWithConfig(content, qrterminal.Config{
		Level:      qrterminal.M,
		Writer:     w,
		HalfBlocks: false,
		BlackChar:  "██",
		WhiteChar:  "  ",
		QuietZone:  2,
	})
	_, _ = fmt.Fprintln(w)
}
