package tui

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/config"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/logging"
	"github.com/davidsgoncalves/awsx/internal/profiles"
	"github.com/davidsgoncalves/awsx/internal/state"
)

// Deps is the injection seam for the TUI. Real wiring lives in cli/root.go;
// tests supply fakes.
type Deps struct {
	Profiles    []profiles.Profile
	SSOSessions []profiles.SSOSession
	Checks      []deps.Dependency

	// Profile flow. region is a region override; "" means resolve from the
	// profile/env (config.ErrNoRegion when none is configured).
	NewClients func(ctx context.Context, profile, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, string, error)
	NewCLI     func(profile, region string) (awsx.Login, awsx.Sessioner)

	// Optional cross-cutting services (nil-safe).
	Log   *logging.Logger
	State *state.State

	// SSO-session flow.
	NewDiscoverer func(ctx context.Context, session profiles.SSOSession) (awsx.SSODiscoverer, error)
	NewSSOLogin   func(session profiles.SSOSession) awsx.Login
	NewEphemeral  func(session profiles.SSOSession, accountID, roleName, region string) (idp awsx.IdentityProvider, ec2 awsx.EC2Lister, ssm awsx.SSMLister, rds awsx.RDSLister, sess awsx.Sessioner, resolvedRegion string, cleanup func(), err error)
}

type rootModel struct {
	deps    Deps
	current screen

	depsScreen      depsScreen
	selectionScreen selectionScreen
	accountScreen   accountScreen
	roleScreen      roleScreen
	regionScreen    regionScreen
	menuScreen      menuScreen
	instancesScreen instancesScreen
	rdsScreen       rdsScreen
	errorScreen     errorScreen

	// Profile / resolved-session state.
	profile string
	region  string
	idp     awsx.IdentityProvider
	ec2     awsx.EC2Lister
	ssm     awsx.SSMLister
	rds     awsx.RDSLister
	login   awsx.Login
	session awsx.Sessioner

	// Tunnel (port-forward) flow state.
	tunneling bool
	tunnelDB  awsx.RDSInstance

	// pendingProfile is a profile awaiting a region choice (profile flow).
	pendingProfile profiles.Profile

	// SSO-session flow state.
	inSSOFlow  bool
	ssoSession profiles.SSOSession
	discoverer awsx.SSODiscoverer
	accountID  string
	roleName   string
	cleanup    func()

	// Instance currently being connected (for error reporting).
	connectingName string
	connectingID   string

	loading       string
	width, height int
	quitting      bool
}

// remember stores a chosen region for key and persists it (best-effort).
func (m rootModel) remember(key, region string) {
	if m.deps.State == nil {
		return
	}
	m.deps.State.SetRegion(key, region)
	if err := m.deps.State.Save(); err != nil {
		m.deps.Log.Error("could not save state: %v", err)
	}
}

// savedRegion returns the remembered region for key, or "".
func (m rootModel) savedRegion(key string) string {
	if m.deps.State == nil {
		return ""
	}
	return m.deps.State.Region(key)
}

// NewRoot builds the initial model. Missing deps route straight to an error
// screen; otherwise the selection screen (profiles + SSO sessions) is shown.
func NewRoot(d Deps) rootModel {
	m := rootModel{deps: d, loading: "Carregando..."}
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
	m.selectionScreen = newSelectionScreen(d.Profiles, d.SSOSessions)
	return m
}

func (m rootModel) Init() tea.Cmd { return nil }

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m.quit()
		}
	case identityMsg:
		m.region = msg.region
		m.menuScreen = newMenuScreen(m.displayProfile(), msg.region, msg.id)
		m.current = screenMenu
		return m, nil
	case targetsMsg:
		if len(msg.targets) == 0 {
			return m.toError(
				"Nenhuma instância EC2 disponível via SSM foi encontrada.",
				"Possíveis motivos: nenhuma instância em execução; SSM Agent desconectado; instância sem IAM Role para SSM; perfil sem permissão; região sem instâncias.",
				[]errorAction{
					{label: "Voltar para o menu principal", next: screenMenu},
					{label: "Sair", next: screenQuit},
				},
			), nil
		}
		m.instancesScreen = newInstancesScreen(msg.targets)
		m.instancesScreen, _, _ = m.instancesScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		if m.tunneling {
			m.current = screenTunnelInstance
		} else {
			m.current = screenInstances
		}
		return m, nil
	case rdsMsg:
		if len(msg.dbs) == 0 {
			return m.toError(
				"Nenhum banco RDS encontrado.",
				"A região selecionada não tem instâncias RDS, ou o perfil não tem permissão rds:DescribeDBInstances.",
				[]errorAction{
					{label: "Voltar para o menu principal", next: screenMenu},
					{label: "Sair", next: screenQuit},
				},
			), nil
		}
		m.rdsScreen = newRDSScreen(msg.dbs)
		m.rdsScreen, _, _ = m.rdsScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.current = screenRDS
		return m, nil
	case needLoginMsg:
		m.current = screenLogin
		return m, loginCmd(m.deps.NewSSOLogin(m.ssoSession))
	case discovererReadyMsg:
		m.discoverer = msg.d
		m.current = screenChecking
		m.loading = "Carregando contas..."
		return m, loadAccountsCmd(msg.d)
	case accountsMsg:
		m.accountScreen = newAccountScreen(msg.accounts)
		m.accountScreen, _, _ = m.accountScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.current = screenAccounts
		return m, nil
	case rolesMsg:
		m.roleScreen = newRoleScreen(msg.roles)
		m.roleScreen, _, _ = m.roleScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.current = screenRoles
		return m, nil
	case loginDoneMsg:
		if msg.err != nil {
			return m.toError("O login foi cancelado ou não foi concluído.", "", []errorAction{
				{label: "Escolher outro perfil", next: screenProfiles},
				{label: "Sair", next: screenQuit},
			}), nil
		}
		if m.inSSOFlow && m.discoverer == nil {
			m.current = screenChecking
			m.loading = "Verificando sessão..."
			return m, initDiscovererCmd(m.discovererFactory())
		}
		m.current = screenChecking
		m.loading = "Verificando sessão..."
		return m, loadIdentityCmd(m.idp, m.region)
	case sessionEndedMsg:
		back := errorAction{label: "Voltar para as instâncias", next: screenInstances}
		if m.tunneling {
			back = errorAction{label: "Voltar para os bancos", next: screenRDS}
		}
		actions := []errorAction{
			back,
			{label: "Voltar para o menu principal", next: screenMenu},
			{label: "Sair", next: screenQuit},
		}
		if msg.err != nil {
			m.deps.Log.Error("ssm session failed for %s (%s) region=%s: %v",
				m.connectingName, m.connectingID, m.region, msg.err)
			detail := fmt.Sprintf("Alvo: %s (%s)\nRegião: %s\nErro: %v\n\nDetalhes no log: %s",
				m.connectingName, m.connectingID, m.region, msg.err, logging.Path())
			title := "Não foi possível abrir a sessão SSM."
			if m.tunneling {
				title = "Não foi possível abrir o túnel."
			}
			return m.toError(title, detail, actions), nil
		}
		title := "Sessão encerrada."
		if m.tunneling {
			title = "Túnel encerrado."
		}
		return m.toError(title, "", actions), nil
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
// transition it requests.
func (m rootModel) routeToScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current {
	case screenProfiles:
		var prof *profiles.Profile
		var sess *profiles.SSOSession
		var cmd tea.Cmd
		m.selectionScreen, prof, sess, cmd = m.selectionScreen.Update(msg)
		if sess != nil {
			return m.startSSOSession(*sess)
		}
		if prof != nil {
			return m.startChecking(*prof, "")
		}
		return m, cmd

	case screenAccounts:
		var sel *awsx.Account
		var cmd tea.Cmd
		m.accountScreen, sel, cmd = m.accountScreen.Update(msg)
		if sel != nil {
			m.accountID = sel.ID
			m.current = screenChecking
			m.loading = "Carregando roles..."
			return m, loadRolesCmd(m.discoverer, sel.ID)
		}
		return m, cmd

	case screenRoles:
		var sel *awsx.Role
		var cmd tea.Cmd
		m.roleScreen, sel, cmd = m.roleScreen.Update(msg)
		if sel != nil {
			m.roleName = sel.Name
			preselect := m.savedRegion(state.SSOKey(m.ssoSession.Name, m.accountID, m.roleName))
			m.regionScreen = newRegionScreen(awsx.Regions(), preselect)
			m.regionScreen, _, _ = m.regionScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.current = screenRegion
		}
		return m, cmd

	case screenRegion:
		var sel *string
		var cmd tea.Cmd
		m.regionScreen, sel, cmd = m.regionScreen.Update(msg)
		if sel != nil {
			if m.inSSOFlow {
				return m.startEphemeral(*sel)
			}
			return m.startChecking(m.pendingProfile, *sel)
		}
		return m, cmd

	case screenMenu:
		var next screen
		m.menuScreen, next = m.menuScreen.Update(msg)
		switch next {
		case screenInstances:
			m.tunneling = false
			m.current = screenChecking
			m.loading = "Carregando instâncias..."
			return m, loadTargetsCmd(m.ec2, m.ssm)
		case screenRDS:
			m.tunneling = true
			m.current = screenChecking
			m.loading = "Carregando bancos..."
			return m, loadRDSCmd(m.rds)
		case screenQuit:
			return m.quit()
		}
		return m, nil

	case screenInstances:
		var sel *awsx.Target
		var cmd tea.Cmd
		m.instancesScreen, sel, cmd = m.instancesScreen.Update(msg)
		if sel != nil {
			id := sel.ID
			name := awsx.DisplayName(sel.Instance)
			m.connectingName, m.connectingID = name, id
			m.deps.Log.Debug("opening ssm session: instance=%s (%s) region=%s", name, id, m.region)
			return m, tea.ExecProcess(sessionExec(m.session, id, name), func(err error) tea.Msg {
				return sessionEndedMsg{err: err}
			})
		}
		return m, cmd

	case screenRDS:
		var sel *awsx.RDSInstance
		var cmd tea.Cmd
		m.rdsScreen, sel, cmd = m.rdsScreen.Update(msg)
		if sel != nil {
			m.tunnelDB = *sel
			m.current = screenChecking
			m.loading = "Carregando instâncias..."
			return m, loadTargetsCmd(m.ec2, m.ssm)
		}
		return m, cmd

	case screenTunnelInstance:
		var sel *awsx.Target
		var cmd tea.Cmd
		m.instancesScreen, sel, cmd = m.instancesScreen.Update(msg)
		if sel != nil {
			return m.startTunnel(*sel)
		}
		return m, cmd

	case screenError:
		var next screen
		m.errorScreen, next = m.errorScreen.Update(msg)
		return m.applyErrorNext(next)
	}
	return m, nil
}

// startChecking begins the profile flow: build clients, then resolve identity.
// region is a region override; when empty and the profile has no configured
// region, the region picker is shown instead of erroring.
func (m rootModel) startChecking(p profiles.Profile, region string) (tea.Model, tea.Cmd) {
	m.inSSOFlow = false
	m.profile = p.Name
	idp, ec2c, ssmc, rdsc, resolved, err := m.deps.NewClients(context.Background(), p.Name, region)
	if err != nil {
		if errors.Is(err, config.ErrNoRegion) {
			m.pendingProfile = p
			m.current = screenRegion
			m.regionScreen = newRegionScreen(awsx.Regions(), m.savedRegion(state.ProfileKey(p.Name)))
			m.regionScreen, _, _ = m.regionScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, nil
		}
		m.deps.Log.Error("prepare profile %q failed: %v", p.Name, err)
		return m.toError("Não foi possível preparar o perfil.", err.Error(), []errorAction{
			{label: "Escolher outro perfil", next: screenProfiles},
			{label: "Sair", next: screenQuit},
		}), nil
	}
	if region != "" {
		m.remember(state.ProfileKey(p.Name), region)
	}
	m.idp, m.ec2, m.ssm, m.rds, m.region = idp, ec2c, ssmc, rdsc, resolved
	if m.deps.NewCLI != nil {
		m.login, m.session = m.deps.NewCLI(p.Name, resolved)
	}
	m.current = screenChecking
	m.loading = "Verificando sessão..."
	return m, loadIdentityCmd(m.idp, m.region)
}

// startSSOSession begins the SSO-session flow: build the discoverer (logging in
// first if the token is missing/expired).
func (m rootModel) startSSOSession(s profiles.SSOSession) (tea.Model, tea.Cmd) {
	m.inSSOFlow = true
	m.ssoSession = s
	m.discoverer = nil
	m.login = m.deps.NewSSOLogin(s)
	m.current = screenChecking
	m.loading = "Verificando sessão..."
	return m, initDiscovererCmd(m.discovererFactory())
}

// startEphemeral builds the ephemeral clients for the chosen account/role/region
// and resolves identity.
func (m rootModel) startEphemeral(region string) (tea.Model, tea.Cmd) {
	idp, ec2c, ssmc, rdsc, sess, region2, cleanup, err := m.deps.NewEphemeral(m.ssoSession, m.accountID, m.roleName, region)
	if err != nil {
		m.deps.Log.Error("prepare ephemeral creds failed (session=%s account=%s role=%s region=%s): %v",
			m.ssoSession.Name, m.accountID, m.roleName, region, err)
		return m.toError("Não foi possível preparar as credenciais.", err.Error(), []errorAction{
			{label: "Escolher outra conta", next: screenAccounts},
			{label: "Sair", next: screenQuit},
		}), nil
	}
	m.remember(state.SSOKey(m.ssoSession.Name, m.accountID, m.roleName), region)
	m.rds = rdsc
	m.idp, m.ec2, m.ssm, m.session = idp, ec2c, ssmc, sess
	m.region = region2
	m.cleanup = cleanup
	m.current = screenChecking
	m.loading = "Verificando sessão..."
	return m, loadIdentityCmd(m.idp, m.region)
}

// startTunnel opens an SSM port-forward from a free local port to the chosen
// RDS endpoint, through the selected instance.
func (m rootModel) startTunnel(t awsx.Target) (tea.Model, tea.Cmd) {
	localPort, err := awsx.FreeLocalPort()
	if err != nil {
		m.deps.Log.Error("could not find a free local port: %v", err)
		return m.toError("Não foi possível abrir uma porta local.", err.Error(), []errorAction{
			{label: "Voltar para o menu principal", next: screenMenu},
			{label: "Sair", next: screenQuit},
		}), nil
	}
	id := t.ID
	m.connectingName = fmt.Sprintf("%s via %s", m.tunnelDB.Name, awsx.DisplayName(t.Instance))
	m.connectingID = id
	m.deps.Log.Debug("opening ssm tunnel: db=%s (%s:%d) via instance=%s local=%d region=%s",
		m.tunnelDB.Name, m.tunnelDB.Endpoint, m.tunnelDB.Port, id, localPort, m.region)
	host, rport := m.tunnelDB.Endpoint, m.tunnelDB.Port
	return m, tea.ExecProcess(portForwardExec(m.session, id, host, rport, localPort), func(err error) tea.Msg {
		return sessionEndedMsg{err: err}
	})
}

func (m rootModel) discovererFactory() func(ctx context.Context) (awsx.SSODiscoverer, error) {
	s := m.ssoSession
	return func(ctx context.Context) (awsx.SSODiscoverer, error) {
		return m.deps.NewDiscoverer(ctx, s)
	}
}

func (m rootModel) applyErrorNext(next screen) (tea.Model, tea.Cmd) {
	switch next {
	case screenError:
		return m, nil
	case screenQuit:
		return m.quit()
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
	case screenAccounts:
		m.current = screenAccounts
		return m, nil
	case screenRDS:
		m.current = screenRDS
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

// quit runs cleanup (removing any ephemeral temp dir) and quits.
func (m rootModel) quit() (tea.Model, tea.Cmd) {
	if m.cleanup != nil {
		m.cleanup()
	}
	m.quitting = true
	return m, tea.Quit
}

// displayProfile is the label for the menu header: the profile name, or the
// account/role for the SSO-session flow.
func (m rootModel) displayProfile() string {
	if m.inSSOFlow {
		return m.ssoSession.Name + " / " + m.accountID + " / " + m.roleName
	}
	return m.profile
}

func (m rootModel) View() string {
	if m.quitting {
		return ""
	}
	switch m.current {
	case screenDeps:
		return m.depsScreen.View()
	case screenProfiles:
		return m.selectionScreen.View()
	case screenAccounts:
		return m.accountScreen.View()
	case screenRoles:
		return m.roleScreen.View()
	case screenRegion:
		return m.regionScreen.View()
	case screenChecking:
		return styleFaint.Render(m.loading)
	case screenLogin:
		return styleFaint.Render("Abrindo autenticação AWS...")
	case screenMenu:
		return m.menuScreen.View()
	case screenInstances, screenTunnelInstance:
		return m.instancesScreen.View()
	case screenRDS:
		return m.rdsScreen.View()
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
