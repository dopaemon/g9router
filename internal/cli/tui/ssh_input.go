package tui

import (
	"io"
	"sync"
)

type sshInput struct {
	chunks chan []byte
	done   chan struct{}
	once   sync.Once
}

type sshInputReader struct {
	parent  *sshInput
	closed  chan struct{}
	pending []byte
}

func newSSHInput(input io.Reader) *sshInput {
	result := &sshInput{chunks: make(chan []byte, 32), done: make(chan struct{})}
	go result.readLoop(input)
	return result
}

func (input *sshInput) readLoop(source io.Reader) {
	defer close(input.chunks)
	buffer := make([]byte, 1024)
	for {
		count, err := source.Read(buffer)
		if count > 0 {
			chunk := append([]byte(nil), buffer[:count]...)
			select {
			case input.chunks <- chunk:
			case <-input.done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (input *sshInput) reader() (*sshInputReader, func()) {
	reader := &sshInputReader{parent: input, closed: make(chan struct{})}
	return reader, func() { close(reader.closed) }
}

func (input *sshInput) close() {
	input.once.Do(func() { close(input.done) })
}

func (reader *sshInputReader) Read(buffer []byte) (int, error) {
	if len(reader.pending) == 0 {
		select {
		case <-reader.closed:
			return 0, io.EOF
		case chunk, ok := <-reader.parent.chunks:
			if !ok {
				return 0, io.EOF
			}
			reader.pending = chunk
		}
	}
	count := copy(buffer, reader.pending)
	reader.pending = reader.pending[count:]
	return count, nil
}

var _ io.Reader = (*sshInputReader)(nil)
