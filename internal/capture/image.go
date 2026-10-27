package capture

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"syscall"
)

// done is a finished picture on disk.
type done struct {
	width, height int
	bytes         int64
	sum           string
	shadowed      bool
}

// finish reads the PNG the backend wrote at src, enforces the limits on it,
// applies the caller's options and leaves the result at dst. src is removed
// either way. dst must not exist.
//
// monitors, for a screen picture, holds each monitor's rectangle relative to
// the picture's origin in desktop pixels. Whatever lies outside all of them
// is not on any display, so it is cleared: the compositor leaves stage
// chrome there that is not part of what the user sees.
func finish(src string, raw Raw, kind Kind, o Options, lim Limits, monitors []Rect, dst string) (done, error) {
	defer os.Remove(src)

	decoded, cfg, err := readPicture(src, lim)
	if err != nil {
		return done{}, err
	}

	if kind == KindWindow && blank(decoded) {
		return done{}, fmt.Errorf("%w: the compositor painted nothing for this window (an X11 window that is minimized or unmapped has no picture to take)", ErrNoTarget)
	}

	frame := Rect{}
	if kind == KindWindow && !o.KeepShadow {
		frame = clip(raw.Frame, cfg.Width, cfg.Height)
	}
	shrink := o.MaxDim > 0 && (cfg.Width > o.MaxDim || cfg.Height > o.MaxDim)
	voids := len(monitors) > 0 && !covered(monitors, cfg.Width, cfg.Height)
	if o.Format != formatJPEG && frame.Empty() && o.Crop.Empty() && !shrink && !voids {
		return keepAsIs(src, dst, cfg.Width, cfg.Height, kind == KindWindow)
	}
	img := toRGBA(decoded)
	if voids {
		clearOutside(img, monitors)
	}
	shadowed := kind == KindWindow
	if !frame.Empty() {
		img = crop(img, frame)
		shadowed = false
	}
	if !o.Crop.Empty() {
		r := clip(o.Crop, img.Bounds().Dx(), img.Bounds().Dy())
		if r.Empty() {
			return done{}, fmt.Errorf("%w: the crop %d,%d %dx%d lies outside the %dx%d picture",
				ErrInvalid, o.Crop.X, o.Crop.Y, o.Crop.W, o.Crop.H, img.Bounds().Dx(), img.Bounds().Dy())
		}
		img = crop(img, r)
	}
	if o.MaxDim > 0 {
		img = fit(img, o.MaxDim)
	}
	return encode(dst, img, o, shadowed)
}

// readPicture opens the backend's file without following a link, enforces
// the byte and pixel limits, and decodes it whole.
func readPicture(src string, lim Limits) (image.Image, image.Config, error) {
	f, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, image.Config{}, fmt.Errorf("%w: %w", ErrBadImage, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, image.Config{}, fmt.Errorf("%w: the backend's file is not a regular file", ErrBadImage)
	}
	if st.Size() == 0 {
		return nil, image.Config{}, fmt.Errorf("%w: the backend's file is empty", ErrBadImage)
	}
	if st.Size() > lim.MaxBytes {
		return nil, image.Config{}, fmt.Errorf("%w: %d bytes against a limit of %d", ErrTooLarge, st.Size(), lim.MaxBytes)
	}
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return nil, image.Config{}, fmt.Errorf("%w: %w", ErrBadImage, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > lim.MaxPixels {
		return nil, image.Config{}, fmt.Errorf("%w: %dx%d pixels against a limit of %d", ErrTooLarge, cfg.Width, cfg.Height, lim.MaxPixels)
	}

	// The whole picture is decoded even when it will be kept as it is: the
	// header alone says nothing about the pixel data, and a truncated or
	// corrupt file must not be filed as a screenshot. The pixel limit above
	// bounds what this costs.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, image.Config{}, fmt.Errorf("%w: %w", ErrBadImage, err)
	}
	decoded, err := png.Decode(bufio.NewReader(f))
	if err != nil {
		return nil, image.Config{}, fmt.Errorf("%w: %w", ErrBadImage, err)
	}
	return decoded, cfg, nil
}

// keepAsIs moves an already validated, untouched PNG into place.
func keepAsIs(src, dst string, w, h int, shadowed bool) (done, error) {
	if err := os.Rename(src, dst); err != nil {
		return done{}, fmt.Errorf("capture: filing the picture: %w", err)
	}
	n, sum, err := hashFile(dst)
	if err != nil {
		return done{}, err
	}
	return done{width: w, height: h, bytes: n, sum: sum, shadowed: shadowed}, nil
}

func encode(dst string, img *image.RGBA, o Options, shadowed bool) (done, error) {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return done{}, fmt.Errorf("capture: filing the picture: %w", err)
	}
	w := bufio.NewWriter(out)
	switch o.Format {
	case formatJPEG:
		q := o.Quality
		if q < 1 || q > 100 {
			q = 85
		}
		flat := image.NewRGBA(img.Bounds())
		draw.Draw(flat, flat.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
		draw.Draw(flat, flat.Bounds(), img, img.Bounds().Min, draw.Over)
		err = jpeg.Encode(w, flat, &jpeg.Options{Quality: q})
	default:
		err = (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(w, img)
	}
	if err == nil {
		err = w.Flush()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
		return done{}, fmt.Errorf("capture: encoding the picture: %w", err)
	}
	n, sum, err := hashFile(dst)
	if err != nil {
		os.Remove(dst)
		return done{}, err
	}
	b := img.Bounds()
	return done{width: b.Dx(), height: b.Dy(), bytes: n, sum: sum, shadowed: shadowed}, nil
}

func hashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// clip returns r limited to a w by h picture, empty if nothing is left.
func clip(r Rect, w, h int) Rect {
	if r.W <= 0 || r.H <= 0 {
		return Rect{}
	}
	x0, y0 := max(r.X, 0), max(r.Y, 0)
	x1, y1 := min(r.X+r.W, w), min(r.Y+r.H, h)
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

func toRGBA(src image.Image) *image.RGBA {
	if p, ok := src.(*image.RGBA); ok && p.Rect.Min == (image.Point{}) {
		return p
	}
	dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst
}

// covered reports whether the monitors, scaled to a w by h picture, leave no
// pixel of it uncovered. The picture is the union's bounding box, so its
// size over the unscaled box gives the scale.
func covered(monitors []Rect, w, h int) bool {
	var area int64
	bw, bh := 0, 0
	for _, m := range monitors {
		bw, bh = max(bw, m.X+m.W), max(bh, m.Y+m.H)
	}
	if bw <= 0 || bh <= 0 {
		return true
	}
	sx, sy := float64(w)/float64(bw), float64(h)/float64(bh)
	for _, m := range monitors {
		area += int64(float64(m.W)*sx) * int64(float64(m.H)*sy)
	}
	// Monitors do not overlap, so their areas add up to the picture's when
	// nothing is left over.
	return area >= int64(w)*int64(h)
}

// clearOutside makes every pixel that is inside none of the monitors
// transparent. The monitors are in desktop pixels and are scaled to the
// picture the same way covered does.
func clearOutside(img *image.RGBA, monitors []Rect) {
	b := img.Bounds()
	bw, bh := 0, 0
	for _, m := range monitors {
		bw, bh = max(bw, m.X+m.W), max(bh, m.Y+m.H)
	}
	if bw <= 0 || bh <= 0 {
		return
	}
	sx, sy := float64(b.Dx())/float64(bw), float64(b.Dy())/float64(bh)
	keep := make([]image.Rectangle, 0, len(monitors))
	for _, m := range monitors {
		keep = append(keep, image.Rect(
			int(float64(m.X)*sx), int(float64(m.Y)*sy),
			int(float64(m.X+m.W)*sx), int(float64(m.Y+m.H)*sy)))
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[(y-b.Min.Y)*img.Stride:]
		for x := b.Min.X; x < b.Max.X; x++ {
			in := false
			for _, r := range keep {
				if image.Pt(x, y).In(r) {
					in = true
					break
				}
			}
			if !in {
				copy(row[(x-b.Min.X)*4:(x-b.Min.X)*4+4], []byte{0, 0, 0, 0})
			}
		}
	}
}

// blank reports a picture with no visible pixel at all.
func blank(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				return false
			}
		}
	}
	return true
}

func crop(src *image.RGBA, r Rect) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, r.W, r.H))
	draw.Draw(dst, dst.Bounds(), src, image.Pt(r.X, r.Y), draw.Src)
	return dst
}

// fit shrinks src, keeping its proportions, until neither side exceeds limit.
// It averages the source pixels under each target pixel (premultiplied, so a
// transparent edge does not bleed), and never enlarges.
func fit(src *image.RGBA, limit int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw <= limit && sh <= limit {
		return src
	}
	dw, dh := limit, limit
	if sw >= sh {
		dh = (sh*limit + sw/2) / sw
	} else {
		dw = (sw*limit + sh/2) / sh
	}
	dw, dh = clampMin(dw), clampMin(dh)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		y0, y1 := y*sh/dh, (y+1)*sh/dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := range dw {
			x0, x1 := x*sw/dw, (x+1)*sw/dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, a, n uint64
			for yy := y0; yy < y1; yy++ {
				row := src.Pix[yy*src.Stride+x0*4 : yy*src.Stride+x1*4]
				for i := 0; i < len(row); i += 4 {
					r += uint64(row[i])
					g += uint64(row[i+1])
					b += uint64(row[i+2])
					a += uint64(row[i+3])
					n++
				}
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] =
				uint8(r/n), uint8(g/n), uint8(b/n), uint8(a/n) //nolint:gosec // a mean of bytes is a byte
		}
	}
	return dst
}

func clampMin(n int) int { return max(n, 1) }
