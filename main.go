package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"hash"
	"hash/crc32"
	"io"
	"log"
	"os"
	"runtime"

	"golang.org/x/sync/errgroup"
)

// see https://golang.org/src/hash/crc32/crc32_amd64.go
const castagnoliK2 = 1344

func newIOBuffer() []byte  { return make([]byte, castagnoliK2*3*512) }
func newHash() hash.Hash32 { return crc32.New(crc32.MakeTable(crc32.Castagnoli)) }

func write(b []byte) {
	_, err := os.Stdout.Write(b)
	if err != nil {
		log.Fatalf("stdout: %v", err)
	}
}

func formatSum(dst []byte, sum uint32) {
	enc := [4]byte{}
	binary.BigEndian.PutUint32(enc[:], sum)
	hex.Encode(dst, enc[:])
}

func sumStdin() error {
	h := newHash()
	_, err := io.CopyBuffer(h, os.Stdin, newIOBuffer())
	if err != nil {
		return err
	}
	b := [9]byte{}
	formatSum(b[:8], h.Sum32())
	b[8] = '\n'
	write(b[:])
	return nil
}

func sumPath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := newHash()
	_, err = io.CopyBuffer(h, f, newIOBuffer())
	if err != nil {
		return err
	}
	b := make([]byte, 8, 64)
	formatSum(b[:8], h.Sum32())
	b = append(b, "  "...)
	b = append(b, path...)
	b = append(b, '\n')
	write(b)
	return nil
}

type result struct {
	Path string
	Sum  uint32
}

type parallelSummer struct {
	N uint
}

func (s *parallelSummer) Run(paths []string) error {
	n := s.N
	if n < 1 {
		n = 1
	}
	if ncpu := uint(runtime.NumCPU()); n > ncpu {
		n = ncpu
	}
	var (
		pathq   = make(chan string, n)
		resultq = make(chan result, n)
	)

	eg, ctx := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		defer close(pathq)
		for _, p := range paths {
			select {
			case pathq <- p:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	eg.Go(func() error {
		defer close(resultq)
		weg, wctx := errgroup.WithContext(ctx)
		for i := uint(0); i < n; i++ {
			weg.Go(func() error {
				return s.workSum(wctx, pathq, resultq)
			})
		}
		return weg.Wait()
	})
	eg.Go(func() error {
		return s.workOutput(ctx, resultq)
	})
	return eg.Wait()
}

func (s *parallelSummer) workSum(ctx context.Context, pathq <-chan string, resultq chan<- result) error {
	var p string
	var ok bool
	h := newHash()
	b := newIOBuffer()
loop:
	for {
		select {
		case p, ok = <-pathq:
		case <-ctx.Done():
			return ctx.Err()
		}
		if !ok {
			break loop
		}

		f, err := os.Open(p)
		if err != nil {
			return err
		}
		h.Reset()
		_, err = io.CopyBuffer(h, f, b)
		f.Close()
		if err != nil {
			return err
		}

		select {
		case resultq <- result{Path: p, Sum: h.Sum32()}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (s *parallelSummer) workOutput(ctx context.Context, resultq <-chan result) error {
	b := make([]byte, 64)
	for {
		r, ok := <-resultq
		if !ok {
			break
		}
		b = b[:8]
		formatSum(b, r.Sum)
		b = append(b, "  "...)
		b = append(b, r.Path...)
		b = append(b, '\n')
		write(b)
	}
	return nil
}

func main() {
	log.SetFlags(0)
	var parallel = flag.Uint("parallel", 1, "number of checksum computations to run in parallel")
	flag.Parse()
	args := flag.Args()

	var err error
	switch len(args) {
	case 0:
		err = sumStdin()
	case 1:
		err = sumPath(args[0])
	default:
		s := parallelSummer{N: *parallel}
		err = s.Run(args)
	}
	if err != nil {
		log.Fatal(err)
	}
}
