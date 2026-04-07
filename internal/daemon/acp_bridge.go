package daemon

import (
	"context"
	"log"

	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"google.golang.org/grpc/metadata"
)

// SendMessageChan sends a message to a session and returns ChatEvents on a channel.
// This is used by the ACP bridge to avoid implementing the full gRPC stream interface.
func (s *Service) SendMessageChan(ctx context.Context, sessionID, content string) (<-chan *pb.ChatEvent, error) {
	ch := make(chan *pb.ChatEvent, 64)
	cs := &chanSendServer{ctx: ctx, ch: ch}

	go func() {
		defer close(ch)
		if err := s.handleChat(ctx, sessionID, content, cs); err != nil {
			log.Printf("acp bridge: handleChat session=%s: %v", sessionID, err)
		}
	}()

	return ch, nil
}

// chanSendServer implements pb.RatchetDaemon_SendMessageServer by forwarding to a channel.
type chanSendServer struct {
	ctx context.Context
	ch  chan<- *pb.ChatEvent
}

func (c *chanSendServer) Send(ev *pb.ChatEvent) error {
	select {
	case c.ch <- ev:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *chanSendServer) Context() context.Context        { return c.ctx }
func (c *chanSendServer) SetHeader(metadata.MD) error     { return nil }
func (c *chanSendServer) SendHeader(metadata.MD) error    { return nil }
func (c *chanSendServer) SetTrailer(metadata.MD)          {}
func (c *chanSendServer) SendMsg(any) error               { return nil }
func (c *chanSendServer) RecvMsg(any) error               { return nil }
