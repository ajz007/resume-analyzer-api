package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type logEntry struct {
	TS     string         `json:"ts"`
	Level  string         `json:"level"`
	Msg    string         `json:"msg"`
	Fields map[string]any `json:"-"`
}

// Info writes an info-level log line with the given fields.
func Info(msg string, fields map[string]any) {
	write("info", msg, fields)
}

// Error writes an error-level log line with the given fields.
func Error(msg string, fields map[string]any) {
	write("error", msg, fields)
}

func write(level, msg string, fields map[string]any) {
	entry := make(map[string]any, len(fields)+3)
	entry["ts"] = time.Now().UTC().Format(time.RFC3339)
	entry["level"] = level
	entry["msg"] = msg
	for k, v := range fields {
		entry[k] = v
	}
	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stdout, `{"ts":"%s","level":"error","msg":"logger marshal failed","err":%q}`+"\n", time.Now().UTC().Format(time.RFC3339), err.Error())
		return
	}
	fmt.Fprintln(os.Stdout, string(data))
}
