package filebuf_test

import (
	"fmt"
	"github.com/malumar/filebuf"
	"github.com/stretchr/testify/assert"
	"io"
	"reflect"
	"testing"
)

const PART1 = "123"
const PART2 = "456"
const PART3 = "789"
const PART4 = "0"
const TEXT = PART1 + PART2 + PART3 + PART4

var mapa map[reflect.Type]string

type structura struct {
	Static  string
	Pointer *string
}

func whatItIs(i interface{}) {
	fmt.Println(reflect.TypeOf(i))
}
func Test1(t *testing.T) {

	st := structura{}
	whatItIs(st.Static)
	whatItIs(st.Pointer)
	assert := assert.New(t)

	buf := filebuf.New(100, true, false)

	b := make([]byte, 200)
	assert.NoError(buf.SwitchToWrite())

	l, e := buf.WriteString(TEXT)
	buf.Flush()
	assert.NoError(e)
	assert.Equal(len(TEXT), l)
	t.Logf("Write %d bytes\n", l)

	assert.NoError(buf.SwitchToRead())
	off, err := buf.Seek(0, io.SeekStart)

	assert.NoError(err)
	assert.Equal(int64(0), off)
	l, e = buf.Read(b)
	assert.NoError(e)
	assert.Equal(len(TEXT), l)

	t.Logf("Read `%s`\n", string(b))

	off, err = buf.Seek(0, io.SeekStart)
	off, err = buf.Seek(0, io.SeekEnd)
	t.Logf("Length `%d`\n", off)

	assert.NoError(buf.Close())
}

func BenchmarkRead(t *testing.B) {
	for i := 0; i < t.N; i++ {
		assert := assert.New(t)

		buf := filebuf.New(100, true, false)

		b := make([]byte, 200)
		assert.NoError(buf.SwitchToWrite())

		l, e := buf.WriteString(TEXT)
		buf.Flush()
		assert.NoError(e)
		assert.Equal(len(TEXT), l)
		t.Logf("Write bytes `%d`\n", l)

		assert.NoError(buf.SwitchToRead())
		off, err := buf.Seek(0, io.SeekStart)

		assert.NoError(err)
		assert.Equal(0, off)
		l, e = buf.Read(b)
		assert.NoError(e)
		assert.Equal(len(TEXT), l)

		t.Logf("Read `%s`\n", string(b))
		assert.NoError(buf.Close())
	}
}
