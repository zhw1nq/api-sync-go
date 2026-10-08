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

const maxOutputBytes = 12 * 1024 // 12KB target ceiling

var bufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 16*1024))
	},
}

// CompressAvatar decodes any image, center-crops to square,
// resizes to size×size using CatmullRom (high speed + crisp downsampling),
// and encodes as baseline JPEG with buffer pooling.
func CompressAvatar(src []byte, size, initialQuality int) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	// Center-crop to square if not already
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

	// Resize to target dimensions using CatmullRom (3-4x faster than Lanczos)
	img = imaging.Resize(img, size, size, imaging.CatmullRom)

	quality := initialQuality
	if quality <= 0 || quality > 100 {
		quality = 75
	}

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	err = jpeg.Encode(buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}

	if buf.Len() <= maxOutputBytes {
		out := make([]byte, buf.Len())
		copy(out, buf.Bytes())
		return out, nil
	}

	// Fallback to lower quality if output exceeded 12KB ceiling
	buf.Reset()
	err = jpeg.Encode(buf, img, &jpeg.Options{Quality: 50})
	if err != nil {
		return nil, fmt.Errorf("encode jpeg fallback: %w", err)
	}

	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}
