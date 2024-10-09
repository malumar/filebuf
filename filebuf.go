package filebuf

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	opNone = iota
	opRead
	opWrite
)

var pagesize = os.Getpagesize() * 2

type ProcessorReadHandler func(r io.Reader) (io.ReadCloser, error)
type ProcessorWriteHandler func(w io.Writer) (io.Writer, error)

type Reader struct {
	*bufio.Reader
	rdr io.Reader
	// gz  io.ReadCloser
	processor io.ReadCloser
}

func min(v1, v2 int) int {
	if v1 < v2 {
		return v1
	}
	return v2
}
func max(v1, v2 int) int {
	if v1 > v2 {
		return v1
	}
	return v2
}

func min64(v1, v2 int64) int64 {
	if v1 < v2 {
		return v1
	}
	return v2
}
func max64(v1, v2 int64) int64 {
	if v1 > v2 {
		return v1
	}
	return v2
}

func (r *Reader) Read(p []byte) (n int, err error) {
	return r.rdr.Read(p)
}

// Close the associated files.
func (r *Reader) Close() error {
	if r.processor != nil {
		r.processor.Close()
	}
	if c, ok := r.rdr.(io.ReadCloser); ok {
		c.Close()
	}
	return nil
}

// Writer is returned by Wopen
type Writer struct {
	*bufio.Writer
	wtr *os.File
	// gz  *gzip.Writer
	processor io.Writer
}

func BufferReader(r io.Reader, processor ProcessorReadHandler) (*Reader, error) {
	b := bufio.NewReaderSize(r, pagesize)

	if rdr, err := processor(b); err != nil {
		return nil, err
	} else {
		b = bufio.NewReaderSize(rdr, pagesize)
		return &Reader{b, r, rdr}, nil
	}

	/*
		if is, err := IsGzip(b); err != nil && err != io.EOF {
			log.Fatal(err)
		} else if is {
			rdr, err = gzip.NewReader(b)
			if err != nil {
				log.Fatal(err)
			}
			b = bufio.NewReaderSize(rdr, pageSize)
		}
		return &Reader{b, r, rdr}
	*/
}

func DefaultBufferSize() int {
	return pagesize
}

func New(bufferSize int, useMemory bool, removeAfterClose bool) Buf {
	return Buf{
		useMemory:        useMemory,
		bufferSize:       bufferSize,
		removeAfterClose: removeAfterClose,
		lastOp:           opNone,
	}
}

type Buf struct {
	buf        []byte
	f          *os.File
	useMemory  bool
	filename   string
	r          *bufio.Reader
	w          *bufio.Writer
	writeMode  bool
	bufferSize int
	//dataInMemoryLen  int
	index            int
	flushAfter       int64
	writtenCount     int64
	removeAfterClose bool
	lastOp           int
	totalWritten     int64
}

var ErrTooLarge = errors.New("filebuf: Out of free memory")

// makeSlice allocates a slice of size n. If the allocation fails, it panics
// with ErrTooLarge.
func makeSlice(n int) []byte {
	// If the make fails, give a known error.
	defer func() {
		if recover() != nil {
			panic(ErrTooLarge)
		}
	}()
	return make([]byte, n)
}

func (b *Buf) Offset() int64 {
	return int64(b.index)
}

func (b *Buf) openFile() (err error) {

	if b.useMemory {
		return errors.New("filebuf: The file cannot be opened in RAM mode")
	}

	if b.f == nil {
		b.f, err = os.CreateTemp("", "filebuf")
	}
	return
}

func (b *Buf) Flush() (err error) {
	if b.lastOp != opWrite {
		return nil
	}

	if b.w != nil {
		err = b.w.Flush()
	}

	if err != nil {
		return err
	}

	b.writtenCount = 0

	return nil
}

func (b *Buf) SwitchToRead() (err error) {

	if b.lastOp == opRead {
		return
	}

	if !b.useMemory {

		if err = b.Flush(); err != nil {
			return err
		}

		if b.w != nil {
			b.w.Reset(nil)
		}

		b.lastOp = opNone

		b.w = nil

		if b.f == nil {

			if err = b.openFile(); err != nil {
				return err
			}

		}

		b.r = bufio.NewReaderSize(b.f, b.bufferSize)

	}

	b.lastOp = opRead

	return nil
}

func (b *Buf) SwitchToWrite() (err error) {

	if b.lastOp == opWrite {
		return
	}

	b.lastOp = opNone

	if b.useMemory {
		b.lastOp = opWrite
		return
	}

	if b.r != nil {
		b.r.Reset(nil)
	}
	b.r = nil

	b.lastOp = opWrite

	if b.f == nil {
		if err = b.openFile(); err != nil {
			return err
		}
	}

	if b.w == nil {
		b.w = bufio.NewWriterSize(b.f, b.bufferSize)
	}
	return nil
}

func (b *Buf) Truncate(n int64) (err error) {
	if b.useMemory {

		switch {
		case n < 0 || n > int64(b.len()):
			return errors.New("bytes.Buffer: truncation out of range")
		case n == 0:
			// Reuse buffer space.
			b.index = 0
		}
		b.buf = b.buf[0 : b.index+int(n)]

		return nil
	}

	if err = b.f.Truncate(n); err != nil {
		return
	}
	if b.lastOp == opRead {
		b.r.Reset(b.f)
	} else {
		b.w.Reset(b.w)
	}

	return nil
}

// Len for io.Reader compatibility
func (b *Buf) Len() int {
	s, _ := b.Size()
	return int(s)
}

func (b *Buf) LenInt64() int64 {
	s, _ := b.Size()
	return s
}

func (b *Buf) Size() (int64, error) {
	if b.useMemory {
		return int64(b.len()), nil
	} else {
		if b.f == nil {
			return 0, nil
		} else {
			if s, err := b.f.Stat(); err != nil {
				return 0, err
			} else {
				return s.Size(), nil
			}
		}
	}
}

func (b *Buf) Seek(offset int64, whence int) (int64, error) {

	if b.useMemory {
		var off int

		switch whence {
		case io.SeekStart:
			off = int(offset)
			break
		case io.SeekCurrent:
			off = b.index + int(offset)
			break
		case io.SeekEnd:
			off = b.len() + int(offset)
			break
		default:
			return 0, fmt.Errorf("filebuf: whence = %d not supported", whence)
		}

		if off < 0 || off > b.len() {
			return 0, errors.New("filebuf: seek after offset")
		}

		b.index = int(offset)

		return int64(off), nil
	}

	if b.lastOp == opNone {
		return 0, errors.New("filebuf: no specific mode")
	}

	off, err := b.f.Seek(offset, whence)
	if err != nil {
		return off, err
	}

	if b.lastOp == opWrite {
		if err = b.Flush(); err != nil {
			return 0, err
		}
		b.w.Reset(b.f)
	} else {
		b.r.Reset(b.f)
	}

	return off, nil
}

func (b *Buf) Read(data []byte) (n int, err error) {
	// fmt.Println("Read")

	if b.lastOp != opRead {
		return 0, errors.New("filebuf: we are not in read mode")
	}

	b.lastOp = opRead

	if b.useMemory {
		if b.index >= len(b.buf) {
			if len(data) == 0 {
				return
			}
			return 0, io.EOF
		}
		n = copy(data, b.buf[b.index:])
		b.index += n
		return
	}

	if b.f == nil {
		return 0, nil
	}

	return b.r.Read(data)
}

func (b *Buf) WriteString(data string) (int, error) {
	return b.Write([]byte(data))
}

// only in memory mode
func (b *Buf) len() int {
	return len(b.buf) - b.index
}

func (b *Buf) grow(n int) int {

	difference := n - cap(b.buf)
	if difference >= 0 {
		// if we are at the meeting point
		if difference == 0 {
			//n+=512
		}
	}

	m := b.len()
	afterWrite := b.index + n
	// If buffer is empty, reset to recover space.
	if m == 0 && b.index != 0 {
		// b.Truncate(0)
	}
	currentCap := cap(b.buf)

	// 05102023:
	// There was > currentCap but I think it still makes the buffo too small
	// so I changed it to >= or we still have something wrong with crossing the free space
	if currentCap <= afterWrite {
		missingSpace := b.index + n - currentCap
		growCapacity := 0
		if currentCap < 1024 && missingSpace < 1024 {
			growCapacity = 1024
		} else {
			growCapacity = max(missingSpace+(n/4), b.index/4)
		}
		newSlice := make([]byte, currentCap+growCapacity)
		copy(newSlice, b.buf[0:])
		b.buf = newSlice
	}
	b.buf = b.buf[0:afterWrite]
	return b.index + m
}

func (b *Buf) Write(data []byte) (int, error) {

	if b.lastOp != opWrite {
		return 0, errors.New("filebuf: we are not in write mode")
	}

	b.lastOp = opWrite
	if b.useMemory {

		b.grow(len(data))
		wl := copy(b.buf[b.index:], data)
		b.index += wl
		b.totalWritten += int64(wl)
		//if int64(cap(b.buf)) < b.totalWritten {
		//	log.Printf("We saved more than the capacity of the buffer %d - although we wanted to have a minimum capacity of %d\n", cap(b.buf), b.bufferSize)
		//}
		return wl, nil
	}

	count, err := b.w.Write(data)
	b.writtenCount += int64(count)
	b.totalWritten += int64(count)

	if err == nil {
		if b.flushAfter > 0 && b.writtenCount >= b.flushAfter {
			return count, b.Flush()
		}
	}

	return count, err
}

func (b *Buf) Close() (err error) {
	if b.useMemory {
		b.buf = b.buf[0:0]
		b.buf = nil
		b.index = 0
		b.lastOp = opNone
		return nil
	}

	if b.writeMode {
		err = b.Flush()
	}

	if b.r != nil {
		b.r.Reset(nil)
		b.r = nil
	}

	if b.w != nil {
		b.w.Reset(nil)
		b.w = nil
	}

	if b.f != nil {
		if err2 := b.f.Close(); err2 != nil {
			err = fmt.Errorf("err: [filebuf]: %v, flush state: %v", err2, err)
		}
		if b.removeAfterClose {
			if b.f != nil {
				fn := b.f.Name()
				os.Remove(fn)
			}
		}
		b.f = nil
	}

	return
}
