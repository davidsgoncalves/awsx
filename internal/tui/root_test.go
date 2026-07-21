package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/profiles"
)

type fakeIDP struct{}

func (fakeIDP) WhoAmI(context.Context) (awsx.Identity, error) {
	return awsx.Identity{Account: "123", Arn: "arn:aws:sts::123:assumed-role/Admin/x"}, nil
}

type fakeEC2 struct{}

func (fakeEC2) RunningInstances(context.Context) ([]awsx.Instance, error) {
	return []awsx.Instance{{ID: "i-1", Name: "api", State: "running", Type: "t3.large", PrivateIP: "10.0.1.15"}}, nil
}

type fakeSSM struct{}

func (fakeSSM) OnlineInstanceIDs(context.Context) (map[string]bool, error) {
	return map[string]bool{"i-1": true}, nil
}

func fakeDeps() Deps {
	return Deps{
		Profiles: []profiles.Profile{{Name: "prod", IsSSO: true, Region: "us-east-1"}},
		Checks: []deps.Dependency{
			{Name: "AWS CLI", Binary: "aws", Found: true},
			{Name: "Session Manager Plugin", Binary: "session-manager-plugin", Found: true},
		},
		NewClients: func(context.Context, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, string, error) {
			return fakeIDP{}, fakeEC2{}, fakeSSM{}, "us-east-1", nil
		},
	}
}

// drive applies a message and returns the concrete rootModel.
func drive(m tea.Model, msg tea.Msg) rootModel {
	next, _ := m.Update(msg)
	return next.(rootModel)
}

func TestRoot_DepsOKStartsAtProfiles(t *testing.T) {
	m := NewRoot(fakeDeps())
	if m.current != screenProfiles {
		t.Fatalf("current = %v, want screenProfiles", m.current)
	}
}

func TestRoot_MissingDepGoesToError(t *testing.T) {
	d := fakeDeps()
	d.Checks[0].Found = false
	m := NewRoot(d)
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
}

type failIDP struct{}

func (failIDP) WhoAmI(context.Context) (awsx.Identity, error) {
	return awsx.Identity{}, context.DeadlineExceeded // stand-in for expired/invalid
}

type fakeLogin struct{ called *bool }

func (f fakeLogin) SSOLogin(context.Context) error { *f.called = true; return nil }

func TestRoot_IdentityFailureWithLoginGoesToLogin(t *testing.T) {
	called := false
	d := fakeDeps()
	d.NewClients = func(context.Context, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, string, error) {
		return failIDP{}, fakeEC2{}, fakeSSM{}, "us-east-1", nil
	}
	d.NewCLI = func(string) (awsx.Login, awsx.Sessioner) { return fakeLogin{called: &called}, nil }

	m := NewRoot(d)
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select profile -> checking
	// identity fails during checking:
	next, cmd := m.Update(errMsg{err: context.DeadlineExceeded})
	m = next.(rootModel)
	if m.current != screenLogin {
		t.Fatalf("current = %v, want screenLogin", m.current)
	}
	if cmd == nil {
		t.Fatal("expected a login command")
	}
	_ = cmd() // execute the login cmd; fake sets called=true
	if !called {
		t.Fatal("login was not invoked")
	}
}

func TestRoot_NoTargetsShowsGuidance(t *testing.T) {
	m := NewRoot(fakeDeps())
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, targetsMsg{targets: nil})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	if !strings.Contains(m.errorScreen.View(), "Nenhuma instância") {
		t.Fatalf("view missing empty message: %q", m.errorScreen.View())
	}
}

func TestRoot_SelectProfileThenIdentityShowsMenu(t *testing.T) {
	m := NewRoot(fakeDeps())
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select "prod" -> checkingScreen
	if m.current != screenChecking {
		t.Fatalf("after select current = %v, want screenChecking", m.current)
	}
	// identity command resolves via fake
	m = drive(m, identityMsg{id: awsx.Identity{Account: "123"}, region: "us-east-1"})
	if m.current != screenMenu {
		t.Fatalf("after identity current = %v, want screenMenu", m.current)
	}
}
