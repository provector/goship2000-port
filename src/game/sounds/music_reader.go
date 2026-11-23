package sounds

import (
	"gunship2000-assets/game/assets"
	"gunship2000-assets/game/diagnostics"
	"io"
	"sync/atomic"
)

type musicPage struct {
	clip   *assets.MusicPages
	number int
	pcm    []int16
}
type musicRequest struct {
	clip   *assets.MusicPages
	number int
}
type musicReader struct {
	requests  chan musicRequest
	quit      chan struct{}
	done      chan struct{}
	pages     [4]atomic.Pointer[musicPage]
	current   musicPage
	requested musicRequest
	misses    atomic.Uint64
}

func newMusicReader() *musicReader {
	r := &musicReader{requests: make(chan musicRequest, 4), quit: make(chan struct{}), done: make(chan struct{})}
	go r.run()
	return r
}
func (r *musicReader) run() {
	defer close(r.done)
	var src *assets.MusicPages
	var file io.ReadSeekCloser
	defer func() {
		if file != nil {
			file.Close()
		}
	}()
	for {
		select {
		case <-r.quit:
			return
		case q := <-r.requests:
			var err error
			if src != q.clip || file == nil {
				if file != nil {
					file.Close()
				}
				src = q.clip
				file, err = src.Open()
			}
			if err == nil {
				var pcm []int16
				pcm, err = src.Read(file, q.number)
				if err == nil {
					r.pages[q.number%len(r.pages)].Store(&musicPage{src, q.number, pcm})
				}
			}
			if err != nil {
				diagnostics.Warn("audio", "Cannot read packed music page", "page", q.number, "error", err)
			}
		}
	}
}
func (r *musicReader) request(clip *assets.MusicPages, n int) {
	if n*assets.MusicPageFrames >= clip.Frames || clip.Warm(n) != nil {
		return
	}
	q := musicRequest{clip, n}
	if r.requested == q {
		return
	}
	if p := r.pages[n%len(r.pages)].Load(); p != nil && p.clip == clip && p.number == n {
		return
	}
	select {
	case r.requests <- q:
		r.requested = q
	default:
	}
}
func (r *musicReader) sample(c *assets.AudioClip, pos uint64) int16 {
	if c.Music == nil {
		return c.Samples[pos]
	}
	clip := c.Music
	n := int(pos) / assets.MusicPageFrames
	if r.current.clip != clip || r.current.number != n || r.current.pcm == nil {
		pcm := clip.Warm(n)
		if pcm == nil {
			if p := r.pages[n%len(r.pages)].Load(); p != nil && p.clip == clip && p.number == n {
				pcm = p.pcm
			}
		}
		if pcm == nil {
			r.request(clip, n)
			r.misses.Add(1)
			return 0
		}
		r.current = musicPage{clip, n, pcm}
		r.request(clip, n+1)
		r.request(clip, n+2)
	}
	return r.current.pcm[int(pos)%assets.MusicPageFrames]
}
