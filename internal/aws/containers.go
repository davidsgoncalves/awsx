package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Errors returned by the remote docker ps run.
var (
	// ErrCommandFailed means the command ran but exited non-zero.
	ErrCommandFailed = errors.New("remote command failed")
	// ErrCommandTimeout means the invocation never reached a terminal state.
	ErrCommandTimeout = errors.New("remote command timed out")
)

// psPollInterval is how often the invocation result is polled. It is a variable
// so tests can shorten it.
var psPollInterval = 500 * time.Millisecond

// psTimeout bounds the whole send-and-poll cycle.
const psTimeout = 20 * time.Second

// ssmRunner is the subset of the SSM API used to run a shell command on an
// instance. *ssm.Client satisfies it.
type ssmRunner interface {
	SendCommand(ctx context.Context, in *ssm.SendCommandInput, opts ...func(*ssm.Options)) (*ssm.SendCommandOutput, error)
	GetCommandInvocation(ctx context.Context, in *ssm.GetCommandInvocationInput, opts ...func(*ssm.Options)) (*ssm.GetCommandInvocationOutput, error)
}

// Containers implements ContainerLister. It runs docker ps on the instance
// through SSM and parses the result. SendCommand runs as root, so no sudo is
// needed here — unlike the interactive docker exec, which runs as ssm-user.
func (c *Clients) Containers(ctx context.Context, instanceID string) ([]Container, error) {
	ctx, cancel := context.WithTimeout(ctx, psTimeout)
	defer cancel()
	return runDockerPS(ctx, c.ssm, instanceID)
}

// runDockerPS sends the docker ps command and polls until the invocation
// reaches a terminal state or ctx expires.
func runDockerPS(ctx context.Context, r ssmRunner, instanceID string) ([]Container, error) {
	out, err := r.SendCommand(ctx, &ssm.SendCommandInput{
		DocumentName: ptr("AWS-RunShellScript"),
		InstanceIds:  []string{instanceID},
		Parameters:   map[string][]string{"commands": {dockerPSCommand}},
	})
	if err != nil {
		return nil, err
	}
	if out.Command == nil || out.Command.CommandId == nil {
		return nil, fmt.Errorf("%w: no command id returned", ErrCommandFailed)
	}
	commandID := *out.Command.CommandId

	for {
		inv, err := r.GetCommandInvocation(ctx, &ssm.GetCommandInvocationInput{
			CommandId:  &commandID,
			InstanceId: &instanceID,
		})
		switch {
		case err == nil:
			switch inv.Status {
			case ssmtypes.CommandInvocationStatusPending, ssmtypes.CommandInvocationStatusInProgress,
				ssmtypes.CommandInvocationStatusDelayed:
				// keep polling
			case ssmtypes.CommandInvocationStatusSuccess:
				return parsePS(deref(inv.StandardOutputContent)), nil
			default:
				detail := deref(inv.StandardErrorContent)
				if detail == "" {
					detail = string(inv.Status)
				}
				return nil, fmt.Errorf("%w: %s", ErrCommandFailed, detail)
			}
		case isInvocationMissing(err):
			// The invocation is not registered yet; retry.
		default:
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ErrCommandTimeout
		case <-time.After(psPollInterval):
		}
	}
}

// isInvocationMissing reports the short window after SendCommand during which
// the invocation is not yet queryable.
func isInvocationMissing(err error) bool {
	var missing *ssmtypes.InvocationDoesNotExist
	return errors.As(err, &missing)
}
