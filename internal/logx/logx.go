package logx

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

const historySize = 500

var (
	mu       sync.RWMutex
	history  []string
	subs     = make(map[chan string]struct{})
	std      = log.New(os.Stdout, "", 0)
)

func Info(category, format string, args ...any) {
	write("INFO", category, format, args...)
}

func Warn(category, format string, args ...any) {
	write("WARN", category, format, args...)
}

func Error(category, format string, args ...any) {
	write("ERROR", category, format, args...)
}

func write(level, category, format string, args ...any) {
	line := fmt.Sprintf("%s [%s] [%s] %s",
		time.Now().Format("2006-01-02 15:04:05"),
		level,
		category,
		fmt.Sprintf(format, args...),
	)

	mu.Lock()
	history = append(history, line)
	if len(history) > historySize {
		history = history[len(history)-historySize:]
	}
	chans := make([]chan string, 0, len(subs))
	for ch := range subs {
		chans = append(chans, ch)
	}
	mu.Unlock()

	std.Println(line)

	for _, ch := range chans {
		select {
		case ch <- line:
		default:
		}
	}
}

func History() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, len(history))
	copy(out, history)
	return out
}

func Subscribe() (chan string, func()) {
	ch := make(chan string, 64)
	mu.Lock()
	subs[ch] = struct{}{}
	snap := append([]string(nil), history...)
	mu.Unlock()

	for _, line := range snap {
		ch <- line
	}

	cancel := func() {
		mu.Lock()
		delete(subs, ch)
		close(ch)
		mu.Unlock()
	}
	return ch, cancel
}
