package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/profiles"
)

// Deps is the injection seam for the TUI. Real wiring lives in Task 15; tests
// supply fakes.
type Deps struct {
	Profiles   []profiles.Profile
	Checks     []deps.Dependency
	NewClients func(ctx context.Context, profile string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, string, error)
	NewCLI     func(profile string) (awsx.Login, awsx.Sessioner)
}

type rootModel struct {
	deps    Deps
	current screen

	depsScreen      depsScreen
	profileScreen   profileScreen
	menuScreen      menuScreen
	instancesScreen instancesScreen
	errorScreen     errorScreen

	profile string
	region  string
	idp     awsx.IdentityProvider
	ec2     awsx.EC2Lister
	ssm     awsx.SSMLister
	login   awsx.Login
	session awsx.Sessioner

	width, height int
	quitting      bool
}

// NewRoot builds the initial model. The dependency check is evaluated eagerly:
// missing deps route straight to an error screen, otherwise the profile
// selector is shown.
func NewRoot(d Deps) rootModel {
	m := rootModel{deps: d}
	m.depsScreen = newDepsScreen(d.Checks)
	if !m.depsScreen.allFound() {
		m.current = screenError
		miss := m.depsScreen.missing()
		detail := deps.InstallHint(miss[0].Binary, deps.GOOS(), deps.BrewAvailable())
		m.errorScreen = newErrorScreen(
			miss[0].Name+" não encontrado.",
			detail,
			[]errorAction{{label: "Sair", next: screenQuit}},
		)
		return m
	}
	m.current = screenProfiles
	m.profileScreen = newProfileScreen(d.Profiles)
	return m
}

func (m rootModel) Init() tea.Cmd { return nil }

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}
	case identityMsg:
		m.region = msg.region
		m.menuScreen = newMenuScreen(m.profile, msg.region, msg.id)
		m.current = screenMenu
		return m, nil
	case targetsMsg:
		m.instancesScreen = newInstancesScreen(msg.targets)
		m.instancesScreen, _, _ = m.instancesScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.current = screenInstances
		return m, nil
	case loginDoneMsg:
		if msg.err != nil {
			return m.toError("O login foi cancelado ou não foi concluído.", "", []errorAction{
				{label: "Tentar novamente", next: screenChecking},
				{label: "Escolher outro perfil", next: screenProfiles},
				{label: "Sair", next: screenQuit},
			}), nil
		}
		m.current = screenChecking
		return m, loadIdentityCmd(m.idp, m.region)
	case sessionEndedMsg:
		return m.toError("Sessão encerrada.", "", []errorAction{
			{label: "Voltar para as instâncias", next: screenInstances},
			{label: "Voltar para o menu principal", next: screenMenu},
			{label: "Sair", next: screenQuit},
		}), nil
	case errMsg:
		// During the checking phase, an identity failure means the session is
		// expired/invalid: route to SSO login instead of a generic error.
		if m.current == screenChecking && m.login != nil {
			m.current = screenLogin
			return m, loginCmd(m.login)
		}
		detail := ""
		if msg.action != "" {
			detail = "Permissão necessária: " + msg.action
		} else if msg.err != nil {
			detail = msg.err.Error()
		}
		return m.toError("Ocorreu um erro.", detail, nil), nil
	}

	return m.routeToScreen(msg)
}

// routeToScreen dispatches a message to the active screen and applies the
// screen transition it requests.
func (m rootModel) routeToScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current {
	case screenProfiles:
		var sel *profiles.Profile
		var cmd tea.Cmd
		m.profileScreen, sel, cmd = m.profileScreen.Update(msg)
		if sel != nil {
			return m.startChecking(*sel)
		}
		return m, cmd

	case screenMenu:
		var next screen
		m.menuScreen, next = m.menuScreen.Update(msg)
		switch next {
		case screenInstances:
			m.current = screenInstances
			return m, loadTargetsCmd(m.ec2, m.ssm)
		case screenQuit:
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case screenInstances:
		var sel *awsx.Target
		var cmd tea.Cmd
		m.instancesScreen, sel, cmd = m.instancesScreen.Update(msg)
		if sel != nil {
			id := sel.ID
			name := awsx.DisplayName(sel.Instance)
			return m, tea.ExecProcess(sessionExec(m.session, id, name), func(err error) tea.Msg {
				return sessionEndedMsg{err: err}
			})
		}
		return m, cmd

	case screenError:
		var next screen
		m.errorScreen, next = m.errorScreen.Update(msg)
		return m.applyErrorNext(next)
	}
	return m, nil
}

func (m rootModel) startChecking(p profiles.Profile) (tea.Model, tea.Cmd) {
	m.profile = p.Name
	idp, ec2c, ssmc, region, err := m.deps.NewClients(context.Background(), p.Name)
	if err != nil {
		return m.toError("Não foi possível preparar o perfil.", err.Error(), []errorAction{
			{label: "Escolher outro perfil", next: screenProfiles},
			{label: "Sair", next: screenQuit},
		}), nil
	}
	m.idp, m.ec2, m.ssm, m.region = idp, ec2c, ssmc, region
	if m.deps.NewCLI != nil {
		m.login, m.session = m.deps.NewCLI(p.Name)
	}
	m.current = screenChecking
	// Try identity first; on failure the error path can route to login.
	return m, loadIdentityCmd(m.idp, m.region)
}

func (m rootModel) applyErrorNext(next screen) (tea.Model, tea.Cmd) {
	switch next {
	case screenError:
		return m, nil
	case screenQuit:
		m.quitting = true
		return m, tea.Quit
	case screenChecking:
		if m.login != nil {
			m.current = screenLogin
			return m, loginCmd(m.login)
		}
		m.current = screenChecking
		return m, loadIdentityCmd(m.idp, m.region)
	case screenProfiles:
		m.current = screenProfiles
		return m, nil
	case screenInstances:
		m.current = screenInstances
		return m, loadTargetsCmd(m.ec2, m.ssm)
	case screenMenu:
		m.current = screenMenu
		return m, nil
	}
	return m, nil
}

func (m rootModel) toError(title, detail string, actions []errorAction) rootModel {
	m.errorScreen = newErrorScreen(title, detail, actions)
	m.current = screenError
	return m
}

func (m rootModel) View() string {
	if m.quitting {
		return ""
	}
	switch m.current {
	case screenDeps:
		return m.depsScreen.View()
	case screenProfiles:
		return m.profileScreen.View()
	case screenChecking:
		return styleFaint.Render("Verificando sessão...")
	case screenLogin:
		return styleFaint.Render("Abrindo autenticação AWS...")
	case screenMenu:
		return m.menuScreen.View()
	case screenInstances:
		return m.instancesScreen.View()
	case screenError:
		return m.errorScreen.View()
	}
	return ""
}

// Run starts the Bubble Tea program with the given dependencies.
func Run(d Deps) error {
	_, err := tea.NewProgram(NewRoot(d), tea.WithAltScreen()).Run()
	return err
}
