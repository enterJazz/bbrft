package brft

import (
	"fmt"
	"net"
	"sync"

	"go.uber.org/zap"
)

const (
	FPeerKey      = "peer"
	FComponentKey = "component"
)

func FPeer(peer string) zap.Field {
	return zap.String(FPeerKey, peer)
}

func FComponent(component string) zap.Field {
	return zap.String(FComponentKey, component)
}

func FAddr(addr net.Addr) zap.Field {
	return zap.String("addr", addr.String())
}

const minProgressDelta float32 = 0.05

func LogProgress(l *zap.Logger, filename string, info *DownloadInfo) {
	LogMultipleProgresses(l, map[string]*DownloadInfo{filename: info})
}

func LogMultipleProgresses(l *zap.Logger, infos map[string]*DownloadInfo) {
	wg := sync.WaitGroup{}
	for filename, info := range infos {
		filename, ch, info := filename, info.ProgChan, info // https://golang.org/doc/faq#closures_and_goroutines
		wg.Add(1)
		go func() {
			defer wg.Done()
			var prevTransmitted uint64 = info.StartOffset
			var logStep uint64 = 0
			if prog, ok := <-ch; ok {
				logStep := uint64(minProgressDelta * float32(prog.TotalBytes))
				if prog.TransmittedBytes > prevTransmitted+logStep {
					l.Info("initial progress",
						zap.String("file_name", filename),
						zap.Uint64("previously_transmitted_bytes", prog.TransmittedBytes),
						zap.Uint64("total_bytes", prog.TotalBytes),
						zap.String("progress",
							fmt.Sprintf("%.2f%%", float32(prog.TransmittedBytes)/float32(prog.TotalBytes)*100),
						),
					)
					prevTransmitted = prog.TransmittedBytes
				}
			}
			for {
				if prog, ok := <-ch; ok {
					if prog.TransmittedBytes > prevTransmitted+logStep {
						l.Info("current progress",
							zap.String("file_name", filename),
							zap.Uint64("transmitted_bytes", prog.TransmittedBytes),
							zap.Uint64("total_bytes", prog.TotalBytes),
							zap.String("progress",
								fmt.Sprintf("%.2f%%", float32(prog.TransmittedBytes)/float32(prog.TotalBytes)*100),
							),
						)
						prevTransmitted = prog.TransmittedBytes
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
