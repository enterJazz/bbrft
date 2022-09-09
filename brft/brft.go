package brft

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

const minProgressDelta float32 = 0.05

func LogProgress(l *zap.Logger, filename string, info *DownloadInfo) {
	LogMultipleProgresses(l, map[FileName]*DownloadInfo{filename: info})
}

func LogMultipleProgresses(l *zap.Logger, infos map[FileName]*DownloadInfo) {
	wg := sync.WaitGroup{}
	for filename, info := range infos {
		filename, ch := filename, info.progChan // https://golang.org/doc/faq#closures_and_goroutines
		wg.Add(1)
		go func() {
			defer wg.Done()
			var prevProgress uint64 = info.startOffset
			logStep := uint64(minProgressDelta * float32(info.totalSize))
			for {
				if prog, ok := <-ch; ok {
					if prog > prevProgress+logStep {
						l.Info("current progress",
							zap.String("file_name", filename),
							zap.Uint64("transmitted_bytes", prog),
							zap.Uint64("total_bytes", info.totalSize),
							zap.String("progress",
								fmt.Sprintf("%.2f%%", float32(prog)/float32(info.totalSize)*100),
							),
						)
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

func ConsumeProgress(chs ...<-chan float32) {
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
