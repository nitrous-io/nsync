package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

var ch = make(chan string, 1000)

func Start() {
	for {
		select {
		case msg := <-ch:
			fmt.Fprint(os.Stdout, msg)
		}
	}
}

func send(msgType string, a ...interface{}) {
	ch <- fmt.Sprintf(fmt.Sprintf("[nsync %s]", msgType), a...)
}

func sendf(msgType, format string, a ...interface{}) {
	ch <- fmt.Sprintf(fmt.Sprintf("[nsync %s] %s", msgType, format), a...)
}

func Info(a ...interface{}) {
	send("info", a...)
}

func Infof(format string, a ...interface{}) {
	sendf("info", format, a...)
}

func Warn(a ...interface{}) {
	send("warn", a...)
}

func Warnf(format string, a ...interface{}) {
	sendf("warn", format, a...)
}

func Error(a ...interface{}) {
	send("error", a...)
}

func Errorf(format string, a ...interface{}) {
	sendf("error", format, a...)
}

func Debug(a ...interface{}) {
	send("debug", a...)
}

func Debugf(format string, a ...interface{}) {
	sendf("debug", format, a...)
}

func Fatal(a ...interface{}) {
	log.Fatal(append([]interface{}{"[nsync fatal]"}, a...))
}

func Fatalf(format string, a ...interface{}) {
	log.Fatalf(fmt.Sprintf("[nsync fatal] %s", format), a...)
}

func Panic(a ...interface{}) {
	log.Panic(append([]interface{}{"[nsync panic]"}, a...))
}

func Panicf(format string, a ...interface{}) {
	log.Panicf(fmt.Sprintf("[nsync panic] %s", format), a...)
}

func Event(name string, data interface{}) {
	message := struct {
		Name string      `json:"name"`
		Data interface{} `json:"data"`
	}{
		Name: name,
		Data: data,
	}

	b, err := json.Marshal(message)
	if err != nil {
		return
	}

	sendf("event", "%s", string(b))
}
