// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package netqueue

import (
	"bytes"
	"encoding/gob"
	. "launchpad.net/gocheck"
	"net"
	"sync"
	"testing"
	"time"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(&S{})

// SafeBuffer is a thread safe buffer.
type SafeBuffer struct {
	buf bytes.Buffer
	sync.Mutex
}

func (sb *SafeBuffer) Read(p []byte) (int, error) {
	sb.Lock()
	defer sb.Unlock()
	return sb.buf.Read(p)
}

func (sb *SafeBuffer) Write(p []byte) (int, error) {
	sb.Lock()
	defer sb.Unlock()
	return sb.buf.Write(p)
}

func (s *S) TestChannelFromWriter(c *C) {
	var buf SafeBuffer
	message := Message{
		Action: "delete",
		Args:   []string{"everything"},
	}
	var wg sync.WaitGroup
	wg.Add(1)
	ch, _ := ChannelFromWriter(&buf)
	go func() {
		ch <- message
		wg.Done()
	}()
	wg.Wait()
	var decodedMessage Message
	decoder := gob.NewDecoder(&buf)
	err := decoder.Decode(&decodedMessage)
	c.Assert(err, IsNil)
	c.Assert(decodedMessage, DeepEquals, message)
}

func (s *S) TestClosesErrChanIfClientCloseMessageChannel(c *C) {
	var buf SafeBuffer
	ch, errCh := ChannelFromWriter(&buf)
	close(ch)
	_, ok := <-errCh
	c.Assert(ok, Equals, false)
}

func (s *S) TestWriteSendErrorsInTheErrorChannel(c *C) {
	messages := make(chan Message, 1)
	errCh := make(chan error, 1)
	conn := NewFakeConn("127.0.0.1:2345", "127.0.0.1:12345")
	conn.Close()
	go write(conn, messages, errCh)
	messages <- Message{}
	close(messages)
	err, ok := <-errCh
	c.Assert(ok, Equals, true)
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Closed connection.")
}

func (s *S) TestChannelFromReader(c *C) {
	var buf SafeBuffer
	messages := []Message{
		{Action: "delete", Args: []string{"everything"}},
		{Action: "rename", Args: []string{"old", "new"}},
		{Action: "destroy", Args: []string{"anything", "something", "otherthing"}},
	}
	encoder := gob.NewEncoder(&buf)
	for _, message := range messages {
		err := encoder.Encode(message)
		c.Assert(err, IsNil)
	}
	gotMessages := make([]Message, len(messages))
	ch, errCh := ChannelFromReader(&buf)
	for i := 0; i < len(messages); i++ {
		gotMessages[i] = <-ch
	}
	c.Assert(gotMessages, DeepEquals, messages)
	err := <-errCh
	c.Assert(err, IsNil)
	_, ok := <-ch
	c.Assert(ok, Equals, false)
	_, ok = <-errCh
	c.Assert(ok, Equals, false)
}

func (s *S) TestReadSendErrorsInTheErrorChannel(c *C) {
	messages := make(chan Message, 1)
	errChan := make(chan error, 1)
	conn := NewFakeConn("127.0.0.1:5055", "127.0.0.1:8080")
	conn.Close()
	go read(conn, messages, errChan)
	err := <-errChan
	c.Assert(err, NotNil)
	c.Assert(err.Error(), Equals, "Closed connection.")
}

func (s *S) TestServerAddr(c *C) {
	listener := NewFakeListener("0.0.0.0:8000")
	server := Server{listener: listener}
	c.Assert(server.Addr(), Equals, listener.Addr().String())
}

func (s *S) TestStartServerAndReadMessage(c *C) {
	message := Message{
		Action: "delete",
		Args:   []string{"something"},
	}
	server, err := StartServer("127.0.0.1:0")
	c.Assert(err, IsNil)
	defer server.Close()
	conn, err := net.Dial("tcp", server.Addr())
	c.Assert(err, IsNil)
	defer conn.Close()
	encoder := gob.NewEncoder(conn)
	err = encoder.Encode(message)
	c.Assert(err, IsNil)
	gotMessage, err := server.Message(2e9)
	c.Assert(err, IsNil)
	c.Assert(gotMessage, DeepEquals, message)
}

func (s *S) TestMessageNegativeTimeout(c *C) {
	server := Server{
		messages: make(chan Message, 1),
		errors:   make(chan error, 1),
	}
	var (
		got, want Message
		err       error
		wg        sync.WaitGroup
	)
	want = Message{Action: "create"}
	wg.Add(1)
	go func() {
		got, err = server.Message(-1)
		wg.Done()
	}()
	time.Sleep(1e6)
	server.messages <- want
	wg.Wait()
	c.Assert(err, IsNil)
	c.Assert(got, DeepEquals, want)
}
