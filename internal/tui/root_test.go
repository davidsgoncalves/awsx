package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/config"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/profiles"
	"github.com/davidsgoncalves/awsx/internal/state"
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

type fakeRDS struct{}

func (fakeRDS) RDSInstances(context.Context) ([]awsx.RDSInstance, error) {
	return []awsx.RDSInstance{{Name: "db1", Engine: "postgres", Endpoint: "db1.rds.local", Port: 5432}}, nil
}

func fakeDeps() Deps {
	return Deps{
		Profiles: []profiles.Profile{{Name: "prod", IsSSO: true, Region: "us-east-1"}},
		Checks: []deps.Dependency{
			{Name: "AWS CLI", Binary: "aws", Found: true},
			{Name: "Session Manager Plugin", Binary: "session-manager-plugin", Found: true},
		},
		NewClients: func(context.Context, string, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, string, error) {
			return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, "us-east-1", nil
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
	d.NewClients = func(context.Context, string, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, string, error) {
		return failIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, "us-east-1", nil
	}
	d.NewCLI = func(string, string) (awsx.Login, awsx.Sessioner) { return fakeLogin{called: &called}, nil }

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

type fakeDiscoverer struct{}

func (fakeDiscoverer) Accounts(context.Context) ([]awsx.Account, error) {
	return []awsx.Account{{ID: "111111111111", Name: "prod"}}, nil
}
func (fakeDiscoverer) Roles(context.Context, string) ([]awsx.Role, error) {
	return []awsx.Role{{Name: "SystemAdministrator"}}, nil
}

func ssoDeps(cleanup func()) Deps {
	return Deps{
		SSOSessions: []profiles.SSOSession{{Name: "vakinha", StartURL: "https://x/start", Region: "us-east-1"}},
		Checks: []deps.Dependency{
			{Name: "AWS CLI", Binary: "aws", Found: true},
			{Name: "Session Manager Plugin", Binary: "session-manager-plugin", Found: true},
		},
		NewDiscoverer: func(context.Context, profiles.SSOSession) (awsx.SSODiscoverer, error) {
			return fakeDiscoverer{}, nil
		},
		NewSSOLogin: func(profiles.SSOSession) awsx.Login { return fakeLogin{called: new(bool)} },
		NewEphemeral: func(profiles.SSOSession, string, string, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.Sessioner, string, func(), error) {
			return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, nil, "sa-east-1", cleanup, nil
		},
	}
}

func TestRoot_SSOFlow_SessionToMenu(t *testing.T) {
	cleaned := false
	m := NewRoot(ssoDeps(func() { cleaned = true }))
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// select the SSO session (first item) -> discoverer init (checking)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenChecking {
		t.Fatalf("after session select current = %v, want screenChecking", m.current)
	}
	if !m.inSSOFlow {
		t.Fatal("expected inSSOFlow true")
	}

	// accounts arrive -> account picker
	m = drive(m, accountsMsg{accounts: []awsx.Account{{ID: "111111111111", Name: "prod"}}})
	if m.current != screenAccounts {
		t.Fatalf("current = %v, want screenAccounts", m.current)
	}

	// pick account -> roles loading (checking)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenChecking || m.accountID != "111111111111" {
		t.Fatalf("after account pick current=%v account=%q", m.current, m.accountID)
	}

	// roles arrive -> role picker
	m = drive(m, rolesMsg{roles: []awsx.Role{{Name: "SystemAdministrator"}}})
	if m.current != screenRoles {
		t.Fatalf("current = %v, want screenRoles", m.current)
	}

	// pick role -> region picker
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenRegion || m.roleName != "SystemAdministrator" {
		t.Fatalf("after role pick current=%v role=%q", m.current, m.roleName)
	}

	// pick region -> ephemeral -> checking
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenChecking {
		t.Fatalf("after region pick current = %v, want screenChecking", m.current)
	}

	// identity -> menu
	m = drive(m, identityMsg{id: awsx.Identity{Account: "111111111111"}, region: "sa-east-1"})
	if m.current != screenMenu {
		t.Fatalf("current = %v, want screenMenu", m.current)
	}

	// quit from menu -> cleanup runs (menu has 3 items; Sair is the third)
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !cleaned {
		t.Fatal("cleanup was not called on quit")
	}
}

func TestRoot_SSOFlow_NeedLoginGoesToLoginThenRetries(t *testing.T) {
	m := NewRoot(ssoDeps(func() {}))
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select session

	// token missing -> needLogin -> login screen
	m = drive(m, needLoginMsg{})
	if m.current != screenLogin {
		t.Fatalf("current = %v, want screenLogin", m.current)
	}

	// login done -> re-init discoverer (checking)
	next, cmd := m.Update(loginDoneMsg{err: nil})
	m = next.(rootModel)
	if m.current != screenChecking {
		t.Fatalf("current = %v, want screenChecking", m.current)
	}
	if cmd == nil {
		t.Fatal("expected discoverer init command after login")
	}
}

func TestRoot_ProfileWithoutRegionShowsRegionPicker(t *testing.T) {
	d := fakeDeps()
	d.NewClients = func(_ context.Context, _, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, string, error) {
		if region == "" {
			return nil, nil, nil, nil, "", config.ErrNoRegion
		}
		return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, region, nil
	}

	m := NewRoot(d)
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select profile -> no region -> region picker
	if m.current != screenRegion {
		t.Fatalf("current = %v, want screenRegion", m.current)
	}

	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // pick first region -> checking
	if m.current != screenChecking {
		t.Fatalf("current = %v, want screenChecking", m.current)
	}

	m = drive(m, identityMsg{id: awsx.Identity{Account: "1"}, region: "af-south-1"})
	if m.current != screenMenu {
		t.Fatalf("current = %v, want screenMenu", m.current)
	}
}

func TestRoot_ProfileRegionIsRemembered(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st := state.Load()
	d := fakeDeps()
	d.State = st
	d.NewClients = func(_ context.Context, _, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, string, error) {
		if region == "" {
			return nil, nil, nil, nil, "", config.ErrNoRegion
		}
		return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, region, nil
	}

	m := NewRoot(d)
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select profile -> region picker
	drive(m, tea.KeyMsg{Type: tea.KeyEnter})     // pick first region (af-south-1)

	if got := st.Region(state.ProfileKey("prod")); got != "af-south-1" {
		t.Fatalf("remembered region = %q, want af-south-1", got)
	}
}

func TestRoot_TunnelFlow_MenuToRDSToInstance(t *testing.T) {
	m := NewRoot(fakeDeps())
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select profile
	m = drive(m, identityMsg{id: awsx.Identity{Account: "1"}, region: "us-east-1"})
	if m.current != screenMenu {
		t.Fatalf("current = %v, want screenMenu", m.current)
	}

	// menu: move to "Acessar banco/serviço (túnel)" and enter
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.tunneling {
		t.Fatal("expected tunneling true")
	}

	// rds list arrives -> rds picker
	m = drive(m, rdsMsg{dbs: []awsx.RDSInstance{{Name: "db1", Endpoint: "db1.rds.local", Port: 5432}}})
	if m.current != screenRDS {
		t.Fatalf("current = %v, want screenRDS", m.current)
	}

	// pick db -> load instances (checking)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenChecking || m.tunnelDB.Name != "db1" {
		t.Fatalf("after db pick current=%v db=%q", m.current, m.tunnelDB.Name)
	}

	// targets arrive -> tunnel instance picker (not the shell instance screen)
	m = drive(m, targetsMsg{targets: []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api", State: "running"}, SSMOnline: true},
	}})
	if m.current != screenTunnelInstance {
		t.Fatalf("current = %v, want screenTunnelInstance", m.current)
	}
	if !strings.Contains(m.instancesScreen.list.Title, "db1") {
		t.Fatalf("tunnel instance title should name the db: %q", m.instancesScreen.list.Title)
	}

	// pick instance -> a port-forward exec command is returned
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an exec command for the tunnel")
	}
}

func TestRoot_NoRDSShowsGuidance(t *testing.T) {
	m := NewRoot(fakeDeps())
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.tunneling = true
	m = drive(m, rdsMsg{dbs: nil})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	if !strings.Contains(m.errorScreen.View(), "Nenhum banco RDS") {
		t.Fatalf("view missing empty-rds message: %q", m.errorScreen.View())
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
