package log

import (
	"sync"

	"go.uber.org/zap"
)

const minProgressDelta float32 = 0.05

func LogProgress(l *zap.Logger, filename string, ch <-chan float32) {
	LogMultipleProgresses(l, map[string]<-chan float32{filename: ch})
}

func LogMultipleProgresses(l *zap.Logger, chs map[string]<-chan float32) {
	wg := sync.WaitGroup{}
	for filename, ch := range chs {
		filename, ch := filename, ch // https://golang.org/doc/faq#closures_and_goroutines
		wg.Add(1)
		go func() {
			defer wg.Done()
			var prevProgress float32 = 0.0
			for {
				if prog, ok := <-ch; ok {
					//if prog > prevProgress {
					if prog > prevProgress+minProgressDelta {
						l.Info("current progress", zap.String("file_name", filename), zap.Float32("progress", prog))
						prevProgress = prog
					}
				} else {
					return
				}
			}
		}()
	}

	wg.Wait()
}

func consumeProgress(chs ...<-chan float32) {
	wg := sync.WaitGroup{}
	for _, ch := range chs {
		wg.Add(1)
		ch := ch // https://golang.org/doc/faq#closures_and_goroutines
		go func() {
			defer wg.Done()
			for {
				if _, ok := <-ch; !ok {
					return
				}
			}
		}()
	}

	wg.Wait()
}
