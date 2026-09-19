// postest is a small ESC/POS debugging tool. It opens a raw TCP connection to
// a thermal printer's port 9100 (or wherever) and lets you fire canned
// commands, raw hex, plain text, or a synthetic stress raster. Handy when the
// real ticket path is misbehaving and you want to isolate whether the problem
// is the printer, the network, or the encode step.
//
// Usage:
//
//	postest -addr HOST:PORT <subcommand> [args]
//
// Subcommands:
//
//	init            ESC @   printer reset
//	feed [n]        LF*n    feed n lines (default 4)
//	cut             GS V B  feed 64 dots + partial cut
//	text "..."      print text, feed, cut
//	hex "1b 40 ..." send arbitrary bytes (whitespace-separated hex)
//	status          query printer status via DLE EOT 1..4 and print responses
//	raster [rows]   send a synthetic checkerboard raster (default 1200 rows,
//	                576 dots wide) to stress-test the transport
//	unstick         attempt to un-hang a printer stuck mid-raster: pumps a
//	                large sink of harmless bytes to satisfy any pending
//	                declared byte count, then re-inits. Try this before
//	                power-cycling.
//
// Global flags:
//
//	-addr string    printer host:port (required)
//	-paced          split writes into 4KB chunks with 15ms pauses (matches
//	                the pacing the server uses)
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "", "printer host:port (e.g. 10.0.0.5:9100)")
	paced := flag.Bool("paced", false, "write in small chunks with 15ms pauses (mirror server pacing)")
	flag.Parse()

	if *addr == "" {
		fmt.Fprintln(os.Stderr, "postest: -addr is required")
		flag.Usage()
		os.Exit(2)
	}
	if _, _, err := net.SplitHostPort(*addr); err != nil {
		fmt.Fprintf(os.Stderr, "postest: invalid addr %q: %v\n", *addr, err)
		os.Exit(2)
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "postest: missing subcommand (init|feed|cut|text|hex|status|raster)")
		os.Exit(2)
	}
	sub, rest := args[0], args[1:]

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var (
		payload  []byte
		readBack bool
		err      error
	)
	switch sub {
	case "init":
		payload = []byte{0x1b, 0x40}
	case "feed":
		n := 4
		if len(rest) > 0 {
			n, err = strconv.Atoi(rest[0])
			if err != nil || n < 1 {
				fatalf("feed: bad count %q", rest[0])
			}
		}
		payload = append([]byte{0x1b, 0x40}, bytesRepeat('\n', n)...)
	case "cut":
		payload = []byte{0x1b, 0x40, 0x1d, 0x56, 0x42, 0x40}
	case "text":
		if len(rest) == 0 {
			fatalf("text: missing string argument")
		}
		body := strings.Join(rest, " ")
		payload = append([]byte{0x1b, 0x40}, []byte(body+"\n\n\n\n")...)
		payload = append(payload, 0x1d, 0x56, 0x42, 0x40)
	case "hex":
		if len(rest) == 0 {
			fatalf("hex: missing bytes")
		}
		payload, err = parseHex(strings.Join(rest, " "))
		if err != nil {
			fatalf("hex: %v", err)
		}
	case "status":
		payload = []byte{
			0x10, 0x04, 0x01, // printer status
			0x10, 0x04, 0x02, // offline status
			0x10, 0x04, 0x03, // error status
			0x10, 0x04, 0x04, // paper roll status
		}
		readBack = true
	case "raster":
		rows := 1200
		if len(rest) > 0 {
			rows, err = strconv.Atoi(rest[0])
			if err != nil || rows < 1 {
				fatalf("raster: bad rows %q", rest[0])
			}
		}
		payload = buildTestRaster(rows)
	case "unstick":
		// If the printer is hung expecting up to ~64 KB of raster from a
		// previous truncated GS v 0, feed it that many white bytes so the
		// receiver finishes, then ESC @ to reset. Followed by a short
		// text line + cut so success is visible on paper.
		payload = append(payload, bytesRepeat(0x00, 65536)...)
		payload = append(payload, 0x1b, 0x40)
		payload = append(payload, []byte("unstick ok\n\n\n")...)
		payload = append(payload, 0x1d, 0x56, 0x42, 0x40)
	default:
		fatalf("unknown subcommand %q", sub)
	}

	fmt.Fprintf(os.Stderr, "postest: sending %d bytes to %s (paced=%v)\n", len(payload), *addr, *paced)

	if err := send(ctx, *addr, payload, *paced, readBack); err != nil {
		fatalf("%v", err)
	}
}

func send(ctx context.Context, addr string, payload []byte, paced, readBack bool) error {
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	if paced {
		if err := writePaced(ctx, conn, payload); err != nil {
			return err
		}
	} else {
		_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if _, err := conn.Write(payload); err != nil {
			return fmt.Errorf("write: %w", err)
		}
	}

	if readBack {
		// Real-time status responses come back within a handful of ms.
		// A longer read window doesn't help — printers that don't answer
		// won't answer.
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 64)
		n, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			// Timeout is the common "printer doesn't support DLE EOT"
			// case; report it but don't treat as fatal.
			fmt.Fprintf(os.Stderr, "read: %v\n", err)
		}
		if n > 0 {
			fmt.Printf("response: % x\n", buf[:n])
			decodeStatus(buf[:n])
		} else {
			fmt.Println("response: (none)")
		}
	}
	return nil
}

// writePaced mirrors internal/printer/escpos.go's pacing so `-paced` here
// tests the same transport behavior as the real server.
func writePaced(ctx context.Context, conn net.Conn, payload []byte) error {
	const (
		chunkSize  = 4096
		chunkPause = 15 * time.Millisecond
		perChunkTO = 5 * time.Second
	)
	for off := 0; off < len(payload); off += chunkSize {
		end := off + chunkSize
		if end > len(payload) {
			end = len(payload)
		}
		_ = conn.SetWriteDeadline(time.Now().Add(perChunkTO))
		if _, err := conn.Write(payload[off:end]); err != nil {
			return fmt.Errorf("write at %d/%d: %w", off, len(payload), err)
		}
		if end < len(payload) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(chunkPause):
			}
		}
	}
	return nil
}

func parseHex(s string) ([]byte, error) {
	compact := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == ',' {
			return -1
		}
		return r
	}, s)
	if strings.HasPrefix(compact, "0x") || strings.HasPrefix(compact, "0X") {
		compact = compact[2:]
	}
	return hex.DecodeString(compact)
}

// buildTestRaster emits a checkerboard the full 576-dot width at rows tall.
// Uses the same GS v 0 chunking + partial-cut sequence as the server so the
// stress test exercises the same code path shape without the Typst render.
func buildTestRaster(rows int) []byte {
	const dotWidth = 576
	rowBytes := dotWidth / 8

	var out []byte
	out = append(out, 0x1b, 0x40)       // ESC @
	out = append(out, 0x1b, 0x33, 0x00) // ESC 3 0 (line spacing 0)

	const chunkRows = 1024
	for y0 := 0; y0 < rows; y0 += chunkRows {
		n := chunkRows
		if y0+n > rows {
			n = rows - y0
		}
		out = append(out,
			0x1d, 0x76, 0x30, 0x00,
			byte(rowBytes&0xff), byte(rowBytes>>8),
			byte(n&0xff), byte(n>>8),
		)
		for ry := 0; ry < n; ry++ {
			// 8-dot-wide vertical stripes; every 8 rows also flip so the
			// pattern is a coarse checkerboard easy to eyeball for missing
			// or shifted bytes.
			pat := byte(0xaa)
			if ((y0+ry)/8)%2 == 1 {
				pat = 0x55
			}
			for x := 0; x < rowBytes; x++ {
				out = append(out, pat)
			}
		}
	}
	out = append(out, 0x1d, 0x56, 0x42, 0x40) // feed + partial cut
	return out
}

// decodeStatus turns the DLE EOT response bytes into a short human-readable
// summary. Real-time status bytes have bit 4 = 1 and bits 0/1 = 0 as a sync
// pattern; if it doesn't match, we just skip.
func decodeStatus(resp []byte) {
	names := []string{"printer", "offline", "error", "paper"}
	for i, b := range resp {
		if i >= len(names) {
			break
		}
		if b&0x93 != 0x12 {
			fmt.Printf("  %s: 0x%02x (not a real-time status byte)\n", names[i], b)
			continue
		}
		fmt.Printf("  %s: 0x%02x %s\n", names[i], b, describeStatus(names[i], b))
	}
}

func describeStatus(kind string, b byte) string {
	switch kind {
	case "printer":
		var s []string
		if b&0x04 != 0 {
			s = append(s, "drawer-open")
		}
		if b&0x08 != 0 {
			s = append(s, "offline")
		}
		if b&0x20 != 0 {
			s = append(s, "wait-online-recovery")
		}
		if b&0x40 != 0 {
			s = append(s, "paper-feed-btn")
		}
		if len(s) == 0 {
			return "ok"
		}
		return "[" + strings.Join(s, ",") + "]"
	case "offline":
		var s []string
		if b&0x04 != 0 {
			s = append(s, "cover-open")
		}
		if b&0x08 != 0 {
			s = append(s, "paper-fed-by-btn")
		}
		if b&0x20 != 0 {
			s = append(s, "paper-end")
		}
		if b&0x40 != 0 {
			s = append(s, "error")
		}
		if len(s) == 0 {
			return "ok"
		}
		return "[" + strings.Join(s, ",") + "]"
	case "error":
		var s []string
		if b&0x04 != 0 {
			s = append(s, "mechanical-error")
		}
		if b&0x08 != 0 {
			s = append(s, "auto-cutter-error")
		}
		if b&0x20 != 0 {
			s = append(s, "unrecoverable-error")
		}
		if b&0x40 != 0 {
			s = append(s, "auto-recoverable-error")
		}
		if len(s) == 0 {
			return "ok"
		}
		return "[" + strings.Join(s, ",") + "]"
	case "paper":
		var s []string
		if b&0x0c != 0 {
			s = append(s, "paper-near-end")
		}
		if b&0x60 != 0 {
			s = append(s, "paper-end")
		}
		if len(s) == 0 {
			return "ok"
		}
		return "[" + strings.Join(s, ",") + "]"
	}
	return ""
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "postest: "+format+"\n", a...)
	os.Exit(1)
}
