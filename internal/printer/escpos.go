package printer

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ESCPOS renders the ticket as a PNG via Typst and pushes it to a thermal
// printer as ESC/POS raster data over a TCP raw socket (typically port 9100).
//
// The render pipeline supersamples the Typst output (so each printer dot is
// the area-average of supersample² source pixels) and converts the result to
// 1-bit via a luminance threshold with Floyd-Steinberg dithering, which keeps
// the anti-aliased edges of the template's text and line-art map crisp. The
// template is pure black-and-white, so no color handling is needed.
type ESCPOS struct {
	addr     string // host:port for TCP raw printing
	template string
	width    int // target raster width in dots
}

// Thermal printers in this family are 203 DPI. The Typst page width is
// derived as width / DPI so each rendered pixel maps cleanly to a printer
// dot at supersample=1.
const targetDPI = 203.0

// supersample is how many source pixels per output dot, per axis. 2 captures
// Typst's anti-aliasing without quadrupling rasterization cost; 3+ shows
// diminishing returns on a 203 DPI printer.
const supersample = 2

// NewESCPOS validates the address and returns a printer. width is the printer
// head width in dots; 576 fits the common 80mm/203dpi thermal printer.
func NewESCPOS(addr, template string, width int) (*ESCPOS, error) {
	if addr == "" {
		return nil, fmt.Errorf("printer: ESC/POS requires PRINTER_ESCPOS_ADDR (host:port)")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, fmt.Errorf("printer: invalid ESC/POS address %q: %w", addr, err)
	}
	if width <= 0 {
		width = 576
	}
	return &ESCPOS{addr: addr, template: template, width: width}, nil
}

func (p *ESCPOS) Print(ctx context.Context, t Ticket) error {
	pngPath, err := p.render(ctx, t)
	if err != nil {
		return err
	}
	defer os.Remove(pngPath)

	img, err := loadImage(pngPath)
	if err != nil {
		return fmt.Errorf("printer: load rendered PNG: %w", err)
	}

	packets := encodeESCPOS(img, p.width)

	// When DEBUG_KEEP_PNG is set, drop a copy of both the rendered PNG and
	// the exact bytes we're about to send to the printer into a debug dir.
	// Handy when a ticket looks corrupt on paper — you can diff the artifacts
	// against a known-good ticket to see where things diverged.
	if dir := debugDir(); dir != "" {
		stem := fmt.Sprintf("balloon-%d-%d", t.BalloonID, time.Now().UnixNano())
		if err := copyFile(pngPath, filepath.Join(dir, stem+".png")); err != nil {
			log.Printf("printer: debug PNG copy: %v", err)
		}
		var flat bytes.Buffer
		for _, p := range packets {
			flat.Write(p)
		}
		if err := os.WriteFile(filepath.Join(dir, stem+".bin"), flat.Bytes(), 0o644); err != nil {
			log.Printf("printer: debug payload write: %v", err)
		}
	}

	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", p.addr)
	if err != nil {
		return fmt.Errorf("printer: ESC/POS dial %s: %w", p.addr, err)
	}
	defer conn.Close()
	return writePackets(ctx, conn, packets)
}

// debugDir returns a directory to drop debug artifacts into, or "" if
// debugging is disabled. Set DEBUG_KEEP_PNG=1 to use /tmp/balloons-debug/,
// or DEBUG_KEEP_PNG=/some/path to choose your own. The directory is created
// on demand.
func debugDir() string {
	v := os.Getenv("DEBUG_KEEP_PNG")
	if v == "" || v == "0" {
		return ""
	}
	dir := v
	if v == "1" || v == "true" {
		dir = "/tmp/balloons-debug"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("printer: debug dir %s: %v", dir, err)
		return ""
	}
	return dir
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// writePackets streams pre-split ESC/POS packets to the printer, pausing
// between each so the printer can physically print (and drain from its
// internal buffer) the raster it just received before the next one arrives.
// This is the pattern cheap thermal printers actually need: TCP flow control
// stops us from overrunning the socket buffer, but nothing stops us from
// overrunning the *printer's* buffer once the socket is drained — the paper
// only advances at ~2400 rows/s (300 mm/s on 203 dpi), so a stream of dense
// raster arriving faster than that fills the buffer and the raster receiver
// hangs mid-command. Chunked GS v 0 + inter-packet pause is the fix.
func writePackets(ctx context.Context, conn net.Conn, packets [][]byte) error {
	const (
		perPacketTO = 5 * time.Second
		// packetPause is comfortably larger than the physical print time of
		// one 64-row GS v 0 chunk at 300 mm/s (≈27 ms). 80 ms gives the head
		// time to advance, the buffer to drain, and the mechanism to settle
		// before the next raster header hits.
		packetPause = 80 * time.Millisecond
		// 120s covers the worst realistic ticket: ~25 packets × 80 ms pause
		// (~2 s) + TCP + rendering slack. Anything past that is a stuck
		// printer, not a slow one.
		overallLimit = 120 * time.Second
	)
	deadline := time.Now().Add(overallLimit)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	for i, pkt := range packets {
		wdl := time.Now().Add(perPacketTO)
		if wdl.After(deadline) {
			wdl = deadline
		}
		_ = conn.SetWriteDeadline(wdl)
		if _, err := conn.Write(pkt); err != nil {
			return fmt.Errorf("printer: ESC/POS write packet %d/%d: %w", i, len(packets), err)
		}
		if i < len(packets)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(packetPause):
			}
		}
	}
	return nil
}

// render compiles the Typst template to a PNG that is supersample× wider
// than the printer's dot count. The page width (in mm) is derived from the
// dot width / targetDPI so the rendered pixel grid lines up with printer
// dots after downsampling.
func (p *ESCPOS) render(ctx context.Context, t Ticket) (string, error) {
	pageWidthMM := float64(p.width) * 25.4 / targetDPI
	return renderTypst(ctx, t, typstOpts{
		template: p.template,
		ext:      "png",
		format:   "png",
		ppi:      targetDPI * float64(supersample),
		extra: []string{
			"--input", "page_width_mm=" + strconv.FormatFloat(pageWidthMM, 'f', 3, 64),
		},
	})
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// encodeESCPOS builds a list of ESC/POS packets to stream to the printer:
// one prelude (init + line spacing), one packet per raster chunk, and one
// coda (feed + cut). The caller (writePackets) pauses between packets so
// each GS v 0 has time to physically print before the next arrives — see
// the writePackets doc for why that matters on cheap thermal printers.
func encodeESCPOS(img image.Image, targetWidth int) [][]byte {
	bw, w, h := imageTo1Bit(img, targetWidth)
	rowBytes := (w + 7) / 8

	// chunkRows sized so each GS v 0 packet fits comfortably in the
	// smallest plausible printer input buffer (~4 KB on cheap Chinese
	// clones). 64 rows × 72 bytes ≈ 4.6 KB per packet on a 576-dot head.
	// Overhead of the extra GS v 0 headers is negligible (8 bytes each).
	const chunkRows = 64

	packets := make([][]byte, 0, 2+(h+chunkRows-1)/chunkRows)

	// Prelude: init + line spacing zero so consecutive rasters butt together.
	packets = append(packets, []byte{
		0x1b, 0x40, // ESC @
		0x1b, 0x33, 0x00, // ESC 3 0
	})

	for y0 := 0; y0 < h; y0 += chunkRows {
		rows := chunkRows
		if y0+rows > h {
			rows = h - y0
		}
		pkt := make([]byte, 0, 8+rows*rowBytes)
		// GS v 0 m xL xH yL yH — m=0 is normal (non-doubled) raster
		pkt = append(pkt,
			0x1d, 0x76, 0x30, 0x00,
			byte(rowBytes&0xff), byte(rowBytes>>8),
			byte(rows&0xff), byte(rows>>8),
		)
		for ry := 0; ry < rows; ry++ {
			rowStart := (y0 + ry) * w
			for xb := 0; xb < rowBytes; xb++ {
				var b byte
				base := xb * 8
				for bit := 0; bit < 8; bit++ {
					x := base + bit
					if x < w && bw[rowStart+x] {
						b |= 1 << (7 - bit)
					}
				}
				pkt = append(pkt, b)
			}
		}
		packets = append(packets, pkt)
	}

	// Coda: feed past the cutter and partial-cut. GS V B n feeds n dots then cuts.
	packets = append(packets, []byte{0x1d, 0x56, 0x42, 0x40})
	return packets
}

// imageTo1Bit converts img to a 1-bit raster of width targetWidth in two
// stages: an area-filter downsample (the supersample step) that produces a
// per-dot ink density, then Floyd-Steinberg dither on that density field.
// true = black (printed).
//
// The downsample averages luminance in linear-light space so anti-aliased
// edges don't get the gamma-darkening artefact of averaging sRGB values
// directly. The template is black-and-white, so a single luminance channel is
// all we need.
func imageTo1Bit(img image.Image, targetWidth int) (pixels []bool, width, height int) {
	src := img.Bounds()
	srcW, srcH := src.Dx(), src.Dy()
	width = targetWidth
	if width <= 0 || width > srcW {
		width = srcW
	}

	scale := float64(srcW) / float64(width)
	height = int(math.Round(float64(srcH) / scale))
	if height < 1 {
		height = 1
	}

	density := make([]float64, width*height)
	for oy := 0; oy < height; oy++ {
		sy0 := int(float64(oy) * scale)
		sy1 := int(float64(oy+1) * scale)
		if sy1 > srcH {
			sy1 = srcH
		}
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for ox := 0; ox < width; ox++ {
			sx0 := int(float64(ox) * scale)
			sx1 := int(float64(ox+1) * scale)
			if sx1 > srcW {
				sx1 = srcW
			}
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}

			var lumLinear, count float64
			for y := sy0; y < sy1; y++ {
				for x := sx0; x < sx1; x++ {
					r, g, b, a := img.At(src.Min.X+x, src.Min.Y+y).RGBA()
					af := float64(a) / 65535.0
					// Composite over white in sRGB space so transparent
					// regions print white rather than black.
					sr := float64(r)/65535.0*af + (1 - af)
					sg := float64(g)/65535.0*af + (1 - af)
					sb := float64(b)/65535.0*af + (1 - af)
					// Rec. 709 luminance in linear light.
					lumLinear += 0.2126*srgbToLinear(sr) + 0.7152*srgbToLinear(sg) + 0.0722*srgbToLinear(sb)
					count++
				}
			}

			// Perceived brightness back through the sRGB transfer; ink density
			// is its complement (dark pixel → more ink).
			perceived := linearToSrgb(lumLinear / count)
			density[oy*width+ox] = 1 - perceived
		}
	}

	pixels = make([]bool, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*width + x
			old := density[i]
			var newVal float64
			if old > 0.5 {
				newVal = 1
				pixels[i] = true
			} else {
				newVal = 0
			}
			err := old - newVal
			if x+1 < width {
				density[i+1] += err * 7 / 16
			}
			if y+1 < height {
				if x > 0 {
					density[i+width-1] += err * 3 / 16
				}
				density[i+width] += err * 5 / 16
				if x+1 < width {
					density[i+width+1] += err * 1 / 16
				}
			}
		}
	}
	return pixels, width, height
}

func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func linearToSrgb(c float64) float64 {
	if c <= 0.0031308 {
		return c * 12.92
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}
