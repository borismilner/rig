package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const shmSize = 1 << 20

func mapFile(path string) []byte {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil { panic(err) }
	f.Truncate(shmSize)
	b, err := syscall.Mmap(int(f.Fd()), 0, shmSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil { panic(err) }
	return b
}
func seq(b []byte, off int) *uint64 { return (*uint64)(unsafe.Pointer(&b[off])) }

// ---------- child roles ----------
func childEcho(sock string) {
	c, err := net.Dial("unix", sock)
	if err != nil { panic(err) }
	hdr := make([]byte, 4)
	buf := make([]byte, 1<<20)
	for {
		if _, err := io.ReadFull(c, hdr); err != nil { return }
		n := binary.LittleEndian.Uint32(hdr)
		if _, err := io.ReadFull(c, buf[:n]); err != nil { return }
		c.Write(hdr)
		c.Write(buf[:n])
	}
}
func childSink(sock string) {
	c, _ := net.Dial("unix", sock)
	buf := make([]byte, 1<<16)
	total := 0
	for { n, err := c.Read(buf); if err != nil { break }; total += n }
	fmt.Fprintln(os.Stderr, "sink got", total)
}
func childShm(path string) {
	b := mapFile(path)
	a, r := seq(b, 0), seq(b, 64)
	last := uint64(0)
	for {
		for atomic.LoadUint64(a) == last { }
		last = atomic.LoadUint64(a)
		if last == ^uint64(0) { return }
		copy(b[256:256+64], b[128:128+64]) // simulate touching the payload
		atomic.StoreUint64(r, last)
	}
}

// ---------- helpers ----------
func serve(sock string) net.Listener {
	os.Remove(sock)
	l, err := net.Listen("unix", sock)
	if err != nil { panic(err) }
	return l
}
func spawn(mode, arg string) *exec.Cmd {
	c := exec.Command(os.Args[0], mode, arg)
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil { panic(err) }
	return c
}
func p(label string, d time.Duration, n int) {
	per := d.Nanoseconds() / int64(n)
	unit := fmt.Sprintf("%d ns", per)
	if per > 10000 { unit = fmt.Sprintf("%.1f µs", float64(per)/1000) } else if per > 1000 { unit = fmt.Sprintf("%.2f µs", float64(per)/1000) }
	fmt.Printf("  %-46s %12s   (%s ops/s)\n", label, unit, human(float64(n)/d.Seconds()))
}
func human(f float64) string {
	switch { case f > 1e6: return fmt.Sprintf("%.1fM", f/1e6); case f > 1e3: return fmt.Sprintf("%.0fk", f/1e3) }
	return fmt.Sprintf("%.0f", f)
}

//go:noinline
func directCall(x int) int { return x + 1 }

func main() {
	if len(os.Args) > 2 {
		switch os.Args[1] {
		case "echo": childEcho(os.Args[2]); return
		case "sink": childSink(os.Args[2]); return
		case "shm":  childShm(os.Args[2]);  return
		}
	}
	dir := os.TempDir() + "/rigbench"
	os.MkdirAll(dir, 0700)
	sock := dir + "/s.sock"

	fmt.Println("\nIPC transports, measured on this laptop")
	fmt.Println("Go", "1.27.1", "| two real processes | median of the run\n")

	// A. in-process baseline
	{
		n := 50_000_000
		t := time.Now(); s := 0
		for i := 0; i < n; i++ { s = directCall(s) }
		_ = s
		fmt.Println("BASELINE  what an imported library costs")
		p("direct Go function call", time.Since(t), n)
	}

	// B. unix socket round trip
	fmt.Println("\nCONTROL PLANE  request/response over a unix socket")
	for _, sz := range []int{64, 4096, 65536} {
		l := serve(sock); c := spawn("echo", sock)
		conn, _ := l.Accept()
		req := make([]byte, sz); hdr := make([]byte, 4); rb := make([]byte, sz)
		binary.LittleEndian.PutUint32(hdr, uint32(sz))
		n := 20000
		if sz == 65536 { n = 5000 }
		for i := 0; i < 200; i++ { conn.Write(hdr); conn.Write(req); io.ReadFull(conn, hdr); io.ReadFull(conn, rb) }
		t := time.Now()
		for i := 0; i < n; i++ { conn.Write(hdr); conn.Write(req); io.ReadFull(conn, hdr); io.ReadFull(conn, rb) }
		d := time.Since(t)
		conn.Close(); l.Close(); c.Process.Kill(); c.Wait()
		p(fmt.Sprintf("raw bytes, %d B payload", sz), d, n)
	}
	// JSON over socket
	{
		l := serve(sock); c := spawn("echo", sock)
		conn, _ := l.Accept()
		type msg struct{ Method string; Key string; Val string; N int }
		m := msg{"config.get", "shelf.index.path", "/home/b/me/library", 7}
		body, _ := json.Marshal(m)
		hdr := make([]byte, 4); binary.LittleEndian.PutUint32(hdr, uint32(len(body)))
		rb := make([]byte, len(body))
		n := 20000
		t := time.Now()
		for i := 0; i < n; i++ {
			b, _ := json.Marshal(m)
			conn.Write(hdr); conn.Write(b)
			io.ReadFull(conn, hdr); io.ReadFull(conn, rb)
			var out msg; json.Unmarshal(rb, &out)
		}
		d := time.Since(t)
		conn.Close(); l.Close(); c.Process.Kill(); c.Wait()
		p(fmt.Sprintf("JSON encode+decode, %d B", len(body)), d, n)
	}

	// C. one-way firehose
	fmt.Println("\nDATA PLANE  one-way, no reply (logs, traces, metrics)")
	{
		l := serve(sock); c := spawn("sink", sock)
		conn, _ := l.Accept()
		rec := make([]byte, 256)
		n := 1_000_000
		t := time.Now()
		for i := 0; i < n; i++ { conn.Write(rec) }
		d := time.Since(t)
		conn.Close(); l.Close(); c.Process.Kill(); c.Wait()
		p("socket write, 256 B log record", d, n)
	}

	// D. shared memory ping-pong
	{
		path := dir + "/shm.bin"
		os.Remove(path)
		b := mapFile(path)
		c := spawn("shm", path)
		a, r := seq(b, 0), seq(b, 64)
		time.Sleep(150 * time.Millisecond)
		n := 200000
		for i := 1; i <= 500; i++ { atomic.StoreUint64(a, uint64(i)); for atomic.LoadUint64(r) != uint64(i) {} }
		t := time.Now()
		for i := 501; i <= 500+n; i++ {
			atomic.StoreUint64(a, uint64(i))
			for atomic.LoadUint64(r) != uint64(i) { }
		}
		d := time.Since(t)
		atomic.StoreUint64(a, ^uint64(0))
		c.Wait()
		fmt.Println()
		fmt.Println("HOT PATH  shared memory, spin-wait, no syscall")
		p("shm round trip, 64 B", d, n)
	}
	fmt.Println()
}
