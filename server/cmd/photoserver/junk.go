package main

import (
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
)

type junkFlag struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Reasons []string `json:"reasons"`
}

type junkState struct {
	Excluded []string            `json:"excluded"`
	Kept     []string            `json:"kept"`
	Flags    map[string]junkFlag `json:"flags"`
}

func detectJunk(path string) (reasons []string) {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))

	nameHints := []string{
		"screenshot", "screen shot", "screen_shot", "screencapture", "screen-capture",
		"screen recording", "screenrecording",
		"receipt", "scan_", "_scan", "document", "docscan", "camscanner",
		"whatsapp image", // often memes/forwards; light signal only with others
	}
	for _, h := range nameHints {
		if strings.Contains(base, h) {
			if strings.Contains(h, "screenshot") || strings.Contains(h, "screen") {
				reasons = appendUnique(reasons, "filename:screenshot")
			} else if strings.Contains(h, "receipt") || strings.Contains(h, "scan") || strings.Contains(h, "document") || strings.Contains(h, "camscanner") {
				reasons = appendUnique(reasons, "filename:document")
			} else {
				reasons = appendUnique(reasons, "filename:forward")
			}
			break
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return reasons
	}
	defer f.Close()

	hasCamera := false
	if x, err := exif.Decode(f); err == nil {
		if makeTag, err := x.Get(exif.Make); err == nil {
			if s, err := makeTag.StringVal(); err == nil && strings.TrimSpace(s) != "" {
				hasCamera = true
			}
		}
		if !hasCamera {
			if modelTag, err := x.Get(exif.Model); err == nil {
				if s, err := modelTag.StringVal(); err == nil && strings.TrimSpace(s) != "" {
					hasCamera = true
				}
			}
		}
	}
	// reset for image decode
	if _, err := f.Seek(0, 0); err != nil {
		return reasons
	}

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return reasons
	}
	w, h := cfg.Width, cfg.Height
	if w < 64 || h < 64 {
		reasons = appendUnique(reasons, "too-small")
	}

	// Phone screenshot-ish aspect ratios (portrait or landscape)
	aspect := float64(w) / float64(h)
	if aspect < 1 {
		aspect = float64(h) / float64(w)
	}
	phoneish := aspect > 1.7 && aspect < 2.4
	if !hasCamera && phoneish && (ext == ".png" || strings.Contains(base, "screen")) {
		reasons = appendUnique(reasons, "screenshot-aspect")
	}
	if !hasCamera && ext == ".png" && phoneish {
		reasons = appendUnique(reasons, "png-no-camera")
	}

	// Document / whiteboard: mostly white/gray
	if _, err := f.Seek(0, 0); err == nil {
		if img, _, err := image.Decode(f); err == nil {
			if whiteFrac(img) > 0.82 && !hasCamera {
				reasons = appendUnique(reasons, "mostly-white")
			}
		}
	}

	// Need at least one solid signal; filename alone or mostly-white+no-camera is enough
	return reasons
}

func whiteFrac(img image.Image) float64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return 0
	}
	// subsample
	stepX := w / 64
	stepY := h / 64
	if stepX < 1 {
		stepX = 1
	}
	if stepY < 1 {
		stepY = 1
	}
	var white, total int
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			total++
			r, g, bl, _ := img.At(x, y).RGBA()
			// 16-bit colors; near white
			if r > 50000 && g > 50000 && bl > 50000 {
				white++
			} else {
				// also count very light gray
				rr, gg, bb := float64(r)/65535, float64(g)/65535, float64(bl)/65535
				if rr > 0.85 && gg > 0.85 && bb > 0.85 {
					white++
				}
			}
		}
	}
	if total == 0 {
		return 0
	}
	_ = color.Gray{}
	return float64(white) / float64(total)
}

func appendUnique(in []string, s string) []string {
	for _, x := range in {
		if x == s {
			return in
		}
	}
	return append(in, s)
}

func junkWorthy(reasons []string) bool {
	if len(reasons) == 0 {
		return false
	}
	// Single weak signal "filename:forward" alone is not enough
	if len(reasons) == 1 && reasons[0] == "filename:forward" {
		return false
	}
	return true
}
