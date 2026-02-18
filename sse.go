package main

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

var errSSEStreamDone = errors.New("sse stream done")

func readSSEEvents(r io.Reader, onEvent func(eventName, data string) error) error {
	reader := bufio.NewReader(r)

	eventName := ""
	dataLines := make([]string, 0, 1)

	dispatch := func() error {
		if len(dataLines) == 0 {
			eventName = ""
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		err := onEvent(eventName, data)
		eventName = ""
		return err
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if err := dispatch(); err != nil {
				if errors.Is(err, errSSEStreamDone) {
					return nil
				}
				return err
			}
		} else if !strings.HasPrefix(line, ":") {
			i := strings.IndexByte(line, ':')
			if i > 0 {
				field := line[:i]
				value := line[i+1:]
				if strings.HasPrefix(value, " ") {
					value = value[1:]
				}

				switch field {
				case "event":
					eventName = value
				case "data":
					dataLines = append(dataLines, value)
				}
			}
		}

		if errors.Is(err, io.EOF) {
			break
		}
	}

	if err := dispatch(); err != nil {
		if errors.Is(err, errSSEStreamDone) {
			return nil
		}
		return err
	}
	return nil
}
