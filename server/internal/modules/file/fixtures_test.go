package file

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
)

// Minimal file headers for content sniffing tests.
var (
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 32)...)
	jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 32)...)
	gifBytes  = []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;")
	webpBytes = []byte("RIFF\x24\x00\x00\x00WEBPVP8 ")
	pdfBytes  = []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	exeBytes  = append([]byte("MZ\x90\x00\x03\x00\x00\x00"), make([]byte, 64)...)
	oleBytes  = append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...)
	htmlBytes = []byte("<!DOCTYPE html><html><body><script>alert(1)</script></body></html>")
	svgBytes  = []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	textBytes = []byte("name,age\nalice,30\n")
	mp3Bytes  = append([]byte("ID3\x03\x00\x00\x00\x00\x00\x00"), make([]byte, 32)...)
)

func zipBytes() []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("word/document.xml")
	_, _ = f.Write([]byte("<w:document/>"))
	_ = w.Close()
	return buf.Bytes()
}

func gzipBytes() []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write([]byte("hello"))
	_ = w.Close()
	return buf.Bytes()
}
