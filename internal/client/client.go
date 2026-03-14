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

// StartTeam starts a team and returns a channel of TeamEvents.
func (c *Client) StartTeam(ctx context.Context, req *pb.StartTeamReq) (<-chan *pb.TeamEvent, error) {
	stream, err := c.daemon.StartTeam(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan *pb.TeamEvent, 64)
	go func() {
		defer close(ch)
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				ch <- &pb.TeamEvent{
					Event: &pb.TeamEvent_Error{
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

// GetTeamStatus returns the status of the active team.
func (c *Client) GetTeamStatus(ctx context.Context, teamID string) (*pb.TeamStatus, error) {
	return c.daemon.GetTeamStatus(ctx, &pb.TeamStatusReq{TeamId: teamID})
}

// CompactSession requests immediate context compression for the given session.
// It sends a special sentinel message that handleChat recognises as a compression
// request rather than a user turn — the daemon compresses history and responds
// with a ContextCompressed event.
func (c *Client) CompactSession(ctx context.Context, sessionID string) (<-chan *pb.ChatEvent, error) {
	return c.SendMessage(ctx, sessionID, "\x00compact\x00")
}

func (c *Client) CreateCron(ctx context.Context, sessionID, schedule, command string) (*pb.CronJob, error) {
	return c.daemon.CreateCron(ctx, &pb.CreateCronReq{
		SessionId: sessionID,
		Schedule:  schedule,
		Command:   command,
	})
}

func (c *Client) ListCrons(ctx context.Context) (*pb.CronJobList, error) {
	return c.daemon.ListCrons(ctx, &pb.Empty{})
}

func (c *Client) PauseCron(ctx context.Context, jobID string) error {
	_, err := c.daemon.PauseCron(ctx, &pb.CronJobReq{JobId: jobID})
	return err
}

func (c *Client) ResumeCron(ctx context.Context, jobID string) error {
	_, err := c.daemon.ResumeCron(ctx, &pb.CronJobReq{JobId: jobID})
	return err
}

func (c *Client) StopCron(ctx context.Context, jobID string) error {
	_, err := c.daemon.StopCron(ctx, &pb.CronJobReq{JobId: jobID})
	return err
}

// StartFleet starts a fleet execution and returns a channel of ChatEvents containing FleetStatus updates.
func (c *Client) StartFleet(ctx context.Context, req *pb.StartFleetReq) (<-chan *pb.ChatEvent, error) {
	stream, err := c.daemon.StartFleet(ctx, req)
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
					Event: &pb.ChatEvent_Error{Error: &pb.ErrorEvent{Message: err.Error()}},
				}
				return
			}
			ch <- event
		}
	}()
	return ch, nil
}

// GetFleetStatus returns the current status of a fleet.
func (c *Client) GetFleetStatus(ctx context.Context, fleetID string) (*pb.FleetStatus, error) {
	return c.daemon.GetFleetStatus(ctx, &pb.FleetStatusReq{FleetId: fleetID})
}

// KillFleetWorker cancels a specific worker within a fleet.
func (c *Client) KillFleetWorker(ctx context.Context, fleetID, workerID string) error {
	_, err := c.daemon.KillFleetWorker(ctx, &pb.KillFleetWorkerReq{
		FleetId:  fleetID,
		WorkerId: workerID,
	})
	return err
}

// ApprovePlan approves a proposed plan and returns a channel of ChatEvents.
func (c *Client) ApprovePlan(ctx context.Context, sessionID, planID string, skipSteps []string) (<-chan *pb.ChatEvent, error) {
	stream, err := c.daemon.ApprovePlan(ctx, &pb.ApprovePlanReq{
		SessionId: sessionID,
		PlanId:    planID,
		SkipSteps: skipSteps,
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan *pb.ChatEvent, 16)
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

// RejectPlan rejects a proposed plan with optional feedback.
func (c *Client) RejectPlan(ctx context.Context, sessionID, planID, feedback string) error {
	_, err := c.daemon.RejectPlan(ctx, &pb.RejectPlanReq{
		SessionId: sessionID,
		PlanId:    planID,
		Feedback:  feedback,
	})
	return err
}

// ListJobs returns all active jobs from the daemon's job registry.
func (c *Client) ListJobs(ctx context.Context) (*pb.JobList, error) {
	return c.daemon.ListJobs(ctx, &pb.Empty{})
}

// PauseJob pauses the job with the given ID.
func (c *Client) PauseJob(ctx context.Context, jobID string) error {
	_, err := c.daemon.PauseJob(ctx, &pb.JobReq{JobId: jobID})
	return err
}

// ResumeJob resumes a paused job.
func (c *Client) ResumeJob(ctx context.Context, jobID string) error {
	_, err := c.daemon.ResumeJob(ctx, &pb.JobReq{JobId: jobID})
	return err
}

// KillJob kills the job with the given ID.
func (c *Client) KillJob(ctx context.Context, jobID string) error {
	_, err := c.daemon.KillJob(ctx, &pb.JobReq{JobId: jobID})
	return err
}

// KillAgent kills a team agent by routing through the job control system.
func (c *Client) KillAgent(ctx context.Context, agentID string) error {
	return c.KillJob(ctx, agentID)
}
