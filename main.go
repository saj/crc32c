package main

import (
	"encoding/binary"
	"encoding/hex"
	"hash"
	"hash/crc32"
	"io"
	"log"
	"os"
	"path/filepath"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix(prog + ": ")

	args := os.Args[1:]

	if len(args) < 1 {
		err := sumStdin()
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	for i := range args {
		err := sumPath(args[i])
		if err != nil {
			log.Fatal(err)
		}
	}
}

func sumStdin() error {
	h := newHash()
	_, err := io.CopyBuffer(h, os.Stdin, copybuf)
	if err != nil {
		return err
	}
	b := [9]byte{}
	formatSum(b[:8], h.Sum32())
	b[8] = '\n'
	_, err = os.Stdout.Write(b[:])
	return err
}

func sumPath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := newHash()
	_, err = io.CopyBuffer(h, f, copybuf)
	if err != nil {
		return err
	}

	b := outbuf[:8] // reuse prior allocation
	formatSum(b[:8], h.Sum32())
	b = append(b, "  "...)
	b = append(b, path...)
	b = append(b, '\n')
	_, err = os.Stdout.Write(b)
	outbuf = b
	return err
}

func formatSum(dst []byte, sum uint32) {
	enc := [4]byte{}
	binary.BigEndian.PutUint32(enc[:], sum)
	hex.Encode(dst, enc[:])
}

var prog = "crc32c"

func init() {
	if len(os.Args) < 1 {
		return
	}
	prog = filepath.Base(os.Args[0])
}

// https://golang.org/src/hash/crc32/crc32_amd64.go
const castagnoliK2 = 1344

var (
	copybuf = make([]byte, castagnoliK2*3*512)
	outbuf  = make([]byte, 64)
)

func newHash() hash.Hash32 { return crc32.New(crc32.MakeTable(crc32.Castagnoli)) }
