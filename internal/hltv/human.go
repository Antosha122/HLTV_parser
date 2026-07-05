package hltv

import (
	"context"
	"math/rand"
	"time"

	"psr/internal/config"
	"psr/internal/logx"
)

func HumanPause(ctx context.Context, cfg config.Config, action string) {
	min := cfg.HumanDelayMin
	max := cfg.HumanDelayMax
	if max < min {
		max = min
	}
	if min <= 0 && max <= 0 {
		return
	}
	if min <= 0 {
		min = 1 * time.Second
	}
	if max <= 0 {
		max = min
	}

	var d time.Duration
	if max == min {
		d = min
	} else {
		delta := max - min
		d = min + time.Duration(rand.Int63n(int64(delta)))
	}

	logx.Info("hltv", "пауза %s — %s", d.Round(time.Millisecond), action)

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		logx.Info("hltv", "пауза прервана — остановка")
		return
	case <-timer.C:
	}
}
