package imgutil

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"sync"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/disintegration/imaging"
)

const maxOutputBytes = 16 * 1024

var bufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 16*1024))
	},
}

func CompressAvatar(src []byte, size, initialQuality int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w != h {
		side := w
		if h < w {
			side = h
		}
		img = imaging.CropCenter(img, side, side)
	}

	img = imaging.Resize(img, size, size, imaging.CatmullRom)

	quality := initialQuality
	if quality <= 0 || quality > 100 {
		quality = 75
	}

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	steps := []int{quality}
	for _, q := range []int{75, 60, 50} {
		if q < quality {
			steps = append(steps, q)
		}
	}

	var out []byte
	for _, q := range steps {
		buf.Reset()
		err = jpeg.Encode(buf, img, &jpeg.Options{Quality: q})
		if err != nil {
			return nil, fmt.Errorf("encode jpeg: %w", err)
		}
		out = make([]byte, buf.Len())
		copy(out, buf.Bytes())
		if buf.Len() <= maxOutputBytes {
			break
		}
	}

	return out, nil
}
