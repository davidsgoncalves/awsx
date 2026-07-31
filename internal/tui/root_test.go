package tui

import (
	"context"
	"errors"
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
	return []awsx.Instance{{ID: "i-1", Name: "api", State: "running", Type: "t3.large", PrivateIP: "10.0.1.15", VpcID: "vpc-1"}}, nil
}

type fakeSSM struct{}

func (fakeSSM) OnlineInstanceIDs(context.Context) (map[string]bool, error) {
	return map[string]bool{"i-1": true}, nil
}

type fakeRDS struct{}

func (fakeRDS) RDSInstances(context.Context) ([]awsx.RDSInstance, error) {
	return []awsx.RDSInstance{
		{Name: "db1", Engine: "postgres", Endpoint: "db1.rds.local", Port: 5432, VpcID: "vpc-1"},
		{Name: "db2", Engine: "mysql", Endpoint: "db2.rds.local", Port: 3306, VpcID: "vpc-2"},
	}, nil
}

func fakeDeps() Deps {
	return Deps{
		Profiles: []profiles.Profile{{Name: "prod", IsSSO: true, Region: "us-east-1"}},
		Checks: []deps.Dependency{
			{Name: "AWS CLI", Binary: "aws", Found: true},
			{Name: "Session Manager Plugin", Binary: "session-manager-plugin", Found: true},
		},
		NewClients: func(context.Context, string, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, string, error) {
			return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, fakeContainers{}, "us-east-1", nil
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
	d.NewClients = func(context.Context, string, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, string, error) {
		return failIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, fakeContainers{}, "us-east-1", nil
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
		NewEphemeral: func(profiles.SSOSession, string, string, string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, awsx.Sessioner, string, func(), error) {
			return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, fakeContainers{}, nil, "sa-east-1", cleanup, nil
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

	// quit from menu -> cleanup runs (menu has 4 items; Sair is the fourth)
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown})
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
	d.NewClients = func(_ context.Context, _, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, string, error) {
		if region == "" {
			return nil, nil, nil, nil, nil, "", config.ErrNoRegion
		}
		return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, fakeContainers{}, region, nil
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
	d.NewClients = func(_ context.Context, _, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, string, error) {
		if region == "" {
			return nil, nil, nil, nil, nil, "", config.ErrNoRegion
		}
		return fakeIDP{}, fakeEC2{}, fakeSSM{}, fakeRDS{}, fakeContainers{}, region, nil
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
	if m.flow != flowTunnel {
		t.Fatalf("flow = %v, want flowTunnel", m.flow)
	}

	// instances arrive first -> tunnel instance picker
	m = drive(m, targetsMsg{targets: []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api", State: "running", VpcID: "vpc-1"}, SSMOnline: true},
	}})
	if m.current != screenTunnelInstance {
		t.Fatalf("current = %v, want screenTunnelInstance", m.current)
	}

	// pick instance -> load databases (checking)
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenChecking || m.tunnelInstance.ID != "i-1" {
		t.Fatalf("after instance pick current=%v instance=%q", m.current, m.tunnelInstance.ID)
	}

	// rds list arrives -> filtered to same VPC (vpc-1) -> rds picker
	m = drive(m, rdsMsg{dbs: []awsx.RDSInstance{
		{Name: "db1", Endpoint: "db1.rds.local", Port: 5432, VpcID: "vpc-1"},
		{Name: "db2", Endpoint: "db2.rds.local", Port: 3306, VpcID: "vpc-2"},
	}})
	if m.current != screenRDS {
		t.Fatalf("current = %v, want screenRDS", m.current)
	}
	if len(m.rdsScreen.list.Items()) != 1 {
		t.Fatalf("expected only same-VPC db (1), got %d", len(m.rdsScreen.list.Items()))
	}

	// pick db -> a port-forward exec command is returned
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an exec command for the tunnel")
	}
}

func TestRoot_NoRDSShowsGuidance(t *testing.T) {
	m := NewRoot(fakeDeps())
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.flow = flowTunnel
	m.tunnelInstance = awsx.Target{Instance: awsx.Instance{ID: "i-9", VpcID: "vpc-empty"}}
	m = drive(m, rdsMsg{dbs: []awsx.RDSInstance{{Name: "db1", VpcID: "vpc-other"}}})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	if !strings.Contains(m.errorScreen.View(), "Nenhum banco alcançável") {
		t.Fatalf("view missing empty message: %q", m.errorScreen.View())
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

type fakeContainers struct{}

func (fakeContainers) Containers(context.Context, string) ([]awsx.Container, error) {
	return []awsx.Container{
		{ID: "abc", Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days"},
	}, nil
}

// menuToRunCommand drives a fresh model to the instance picker of the
// "Rodar comando" flow.
func menuToRunCommand(t *testing.T, d Deps) rootModel {
	t.Helper()
	m := NewRoot(d)
	m = drive(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // select profile
	m = drive(m, identityMsg{id: awsx.Identity{Account: "1"}, region: "us-east-1"})

	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}) // túnel
	m = drive(m, tea.KeyMsg{Type: tea.KeyDown}) // Rodar comando
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.flow != flowExec {
		t.Fatalf("flow = %v, want flowExec", m.flow)
	}
	return m
}

func TestRoot_ExecFlow_InstanceToContainerToCommand(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())

	// instances arrive -> exec instance picker
	m = drive(m, targetsMsg{targets: []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api", State: "running", VpcID: "vpc-1"}, SSMOnline: true},
	}})
	if m.current != screenExecInstance {
		t.Fatalf("current = %v, want screenExecInstance", m.current)
	}

	// pick instance -> loading containers
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenChecking || m.execInstance.ID != "i-1" {
		t.Fatalf("current=%v instance=%q", m.current, m.execInstance.ID)
	}

	// containers arrive -> container picker
	m = drive(m, containersMsg{containers: []awsx.Container{
		{ID: "abc", Name: "myapp-web-1", Service: "web", Image: "ruby:3.2", Status: "Up 3 days"},
	}})
	if m.current != screenContainers {
		t.Fatalf("current = %v, want screenContainers", m.current)
	}

	// pick container -> command screen
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenCommand {
		t.Fatalf("current = %v, want screenCommand", m.current)
	}
	if m.execContainer.Name != "myapp-web-1" {
		t.Fatalf("container = %q", m.execContainer.Name)
	}

	// type a command and submit -> an exec command is returned
	for _, r := range "bash" {
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected an exec command")
	}
}

func TestRoot_ExecFlow_RemembersCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	st := state.Load()
	d := fakeDeps()
	d.State = st

	m := menuToRunCommand(t, d)
	m = drive(m, targetsMsg{targets: []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api", VpcID: "vpc-1"}, SSMOnline: true},
	}})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(m, containersMsg{containers: []awsx.Container{
		{ID: "abc", Name: "myapp-web-1", Service: "web"},
	}})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter}) // pick container
	for _, r := range "rails c" {
		m = drive(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	got := st.CommandHistory(state.ContainerKey("web"))
	if len(got) != 1 || got[0] != "rails c" {
		t.Fatalf("history = %v, want [rails c]", got)
	}
}

func TestRoot_ExecFlow_NoContainersShowsGuidance(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m.execInstance = awsx.Target{Instance: awsx.Instance{ID: "i-1", Name: "api"}}

	m = drive(m, containersMsg{containers: nil})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	if !strings.Contains(m.errorScreen.View(), "Nenhum container") {
		t.Fatalf("view missing empty message: %q", m.errorScreen.View())
	}
}

func TestRoot_ExecFlow_EscFromCommandGoesBackToContainers(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m = drive(m, targetsMsg{targets: []awsx.Target{
		{Instance: awsx.Instance{ID: "i-1", Name: "api", VpcID: "vpc-1"}, SSMOnline: true},
	}})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(m, containersMsg{containers: []awsx.Container{{ID: "abc", Name: "myapp-web-1", Service: "web"}}})
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})

	m = drive(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.current != screenContainers {
		t.Fatalf("current = %v, want screenContainers", m.current)
	}
}

func TestRoot_ExecFlow_SessionEndReturnsToCommandScreen(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m.execContainer = awsx.Container{Name: "myapp-web-1", Service: "web"}

	m = drive(m, sessionEndedMsg{})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	v := m.errorScreen.View()
	if !strings.Contains(v, "Voltar para o comando") {
		t.Fatalf("missing back-to-command action: %q", v)
	}
}

func TestRoot_ExecFlow_ListingDeniedOffersManualCommand(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m.execInstance = awsx.Target{Instance: awsx.Instance{ID: "i-1", Name: "api"}}

	m = drive(m, errMsg{err: errors.New("denied"), action: "ssm:SendCommand"})
	if m.current != screenError {
		t.Fatalf("current = %v, want screenError", m.current)
	}
	v := m.errorScreen.View()
	if !strings.Contains(v, "ssm:SendCommand") {
		t.Fatalf("error does not name the permission: %q", v)
	}
	if !strings.Contains(v, "Digitar o comando à mão") {
		t.Fatalf("error does not offer the manual fallback: %q", v)
	}
}

func TestRoot_ExecFlow_ManualFallbackOpensFullLineCommandScreen(t *testing.T) {
	m := menuToRunCommand(t, fakeDeps())
	m.execInstance = awsx.Target{Instance: awsx.Instance{ID: "i-1", Name: "api"}}
	m = drive(m, errMsg{err: errors.New("denied"), action: "ssm:SendCommand"})

	// The manual fallback is the first action on the error screen.
	m = drive(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.current != screenCommand {
		t.Fatalf("current = %v, want screenCommand", m.current)
	}
	if !m.commandScreen.fullLineMode() {
		t.Fatal("expected the manual screen to start in full-line mode")
	}
	if !strings.Contains(m.commandScreen.View(), "api") {
		t.Fatalf("manual screen does not name the instance: %q", m.commandScreen.View())
	}
}
