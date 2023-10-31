package svc

import (
	"context"
	"log"
	"time"

	"github.com/mei-rune/autoupdate"
)

func RunUpdate(updater *autoupdate.Updater, restart, exit chan struct{}) {
	ctx := context.Background()
	for {
		timer := time.NewTimer(10 * time.Minute)
		select {
		case <-exit:
			return
		case <-timer.C:
		}

		hasUpdateOk, err := updater.DoUpdate(ctx)
		if err != nil {
			log.Println(err)
		}
		if hasUpdateOk && restart != nil {
			select {
			case restart <- struct{}{}:
			default:
			}
		}
	}
}
