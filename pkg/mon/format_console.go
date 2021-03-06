package mon

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/jonboulle/clockwork"
	"strings"
)

func formatterConsole(clock clockwork.Clock, channel string, level string, msg string, logErr error, fields Fields) ([]byte, error) {
	fieldParts := make([]string, 0, len(fields))
	for k, v := range fields {
		fieldParts = append(fieldParts, fmt.Sprintf("%v: %v", k, v))
	}
	fieldString := strings.Join(fieldParts, ", ")

	now := clock.Now().Format("15:04:05.999999")

	errStr := ""
	if logErr != nil {
		errStr = fmt.Sprintf("ERR: %s", logErr.Error())
	}

	now = fmt.Sprintf("%-15v", now)
	level = fmt.Sprintf("%-7v", level)
	channel = fmt.Sprintf("%-7s", channel)

	output := fmt.Sprintf("%s %s %s %-50s %s %s", color.YellowString(now), color.GreenString(channel), color.GreenString(level), msg, color.BlueString(fieldString), color.RedString(errStr))
	serialized := []byte(output)

	return append(serialized, '\n'), nil
}
