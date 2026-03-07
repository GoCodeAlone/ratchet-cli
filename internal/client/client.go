package client

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/GoCodeAlone/ratchet-cli/internal/daemon"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
)

type Client struct {
	conn   *grpc.ClientConn
	daemon pb.RatchetDaemonClient
}

// Connect creates a gRPC client connection to the daemon Unix socket.
func Connect() (*Client, error) {
	sock := daemon.SocketPath()
	conn, err := grpc.NewClient(
		"unix://"+sock,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon at %s: %w", sock, err)
	}
	return &Client{
		conn:   conn,
		daemon: pb.NewRatchetDaemonClient(conn),
	}, nil
}

// EnsureDaemon starts the daemon if not running, then connects.
func EnsureDaemon() (*Client, error) {
	if !daemon.IsRunning() {
		if err := daemon.StartBackground(); err != nil {
			return nil, fmt.Errorf("start daemon: %w", err)
		}
	}
	return Connect()
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Health(ctx context.Context) (*pb.HealthResponse, error) {
	return c.daemon.Health(ctx, &pb.Empty{})
}

func (c *Client) CreateSession(ctx context.Context, req *pb.CreateSessionReq) (*pb.Session, error) {
	return c.daemon.CreateSession(ctx, req)
}

func (c *Client) ListSessions(ctx context.Context) (*pb.SessionList, error) {
	return c.daemon.ListSessions(ctx, &pb.Empty{})
}

func (c *Client) KillSession(ctx context.Context, id string) error {
	_, err := c.daemon.KillSession(ctx, &pb.KillReq{SessionId: id})
	return err
}

// SendMessage sends a chat message and returns a channel of ChatEvents.
func (c *Client) SendMessage(ctx context.Context, sessionID, content string) (<-chan *pb.ChatEvent, error) {
	stream, err := c.daemon.SendMessage(ctx, &pb.SendMessageReq{
		SessionId: sessionID,
		Content:   content,
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan *pb.ChatEvent, 64)
	go func() {
		defer close(ch)
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- &pb.ChatEvent{
					Event: &pb.ChatEvent_Error{
						Error: &pb.ErrorEvent{Message: err.Error()},
					},
				}
				return
			}
			ch <- event
		}
	}()
	return ch, nil
}

// AttachSession streams events from an existing session.
func (c *Client) AttachSession(ctx context.Context, sessionID string) (<-chan *pb.ChatEvent, error) {
	stream, err := c.daemon.AttachSession(ctx, &pb.AttachReq{SessionId: sessionID})
	if err != nil {
		return nil, err
	}

	ch := make(chan *pb.ChatEvent, 64)
	go func() {
		defer close(ch)
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- &pb.ChatEvent{
					Event: &pb.ChatEvent_Error{
						Error: &pb.ErrorEvent{Message: err.Error()},
					},
				}
				return
			}
			ch <- event
		}
	}()
	return ch, nil
}

func (c *Client) RespondToPermission(ctx context.Context, requestID string, allowed bool, scope string) error {
	_, err := c.daemon.RespondToPermission(ctx, &pb.PermissionResponse{
		RequestId: requestID,
		Allowed:   allowed,
		Scope:     scope,
	})
	return err
}

func (c *Client) AddProvider(ctx context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
	return c.daemon.AddProvider(ctx, req)
}

func (c *Client) ListProviders(ctx context.Context) (*pb.ProviderList, error) {
	return c.daemon.ListProviders(ctx, &pb.Empty{})
}

func (c *Client) TestProvider(ctx context.Context, alias string) (*pb.TestProviderResult, error) {
	return c.daemon.TestProvider(ctx, &pb.TestProviderReq{Alias: alias})
}

func (c *Client) RemoveProvider(ctx context.Context, alias string) error {
	_, err := c.daemon.RemoveProvider(ctx, &pb.RemoveProviderReq{Alias: alias})
	return err
}

func (c *Client) SetDefaultProvider(ctx context.Context, alias string) error {
	_, err := c.daemon.SetDefaultProvider(ctx, &pb.SetDefaultProviderReq{Alias: alias})
	return err
}

func (c *Client) ListAgents(ctx context.Context) (*pb.AgentList, error) {
	return c.daemon.ListAgents(ctx, &pb.Empty{})
}

func (c *Client) Shutdown(ctx context.Context) error {
	_, err := c.daemon.Shutdown(ctx, &pb.Empty{})
	return err
}
