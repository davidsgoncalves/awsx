package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	awsx "github.com/davidsgoncalves/awsx/internal/aws"
	"github.com/davidsgoncalves/awsx/internal/deps"
	"github.com/davidsgoncalves/awsx/internal/profiles"
)

// Deps is the injection seam for the TUI. Real wiring lives in cli/root.go;
// tests supply fakes.
type Deps struct {
	Profiles    []profiles.Profile
	SSOSessions []profiles.SSOSession
	Checks      []deps.Dependency

	// Profile flow.
	NewClients func(ctx context.Context, profile string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, string, error)
	NewCLI     func(profile string) (awsx.Login, awsx.Sessioner)

	// SSO-session flow.
	NewDiscoverer func(ctx context.Context, session profiles.SSOSession) (awsx.SSODiscoverer, error)
	NewSSOLogin   func(session profiles.SSOSession) awsx.Login
	NewEphemeral  func(session profiles.SSOSession, accountID, roleName, region string) (idp awsx.IdentityProvider, ec2 awsx.EC2Lister, ssm awsx.SSMLister, sess awsx.Sessioner, resolvedRegion string, cleanup func(), err error)
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
	errorScreen     errorScreen

	// Profile / resolved-session state.
	profile string
	region  string
	idp     awsx.IdentityProvider
	ec2     awsx.EC2Lister
	ssm     awsx.SSMLister
	login   awsx.Login
	session awsx.Sessioner

	// SSO-session flow state.
	inSSOFlow  bool
	ssoSession profiles.SSOSession
	discoverer awsx.SSODiscoverer
	accountID  string
	roleName   string
	cleanup    func()

	loading       string
	width, height int
	quitting      bool
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
		m.current = screenInstances
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
			return m.startChecking(*prof)
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
			m.regionScreen = newRegionScreen(awsx.Regions())
			m.regionScreen, _, _ = m.regionScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.current = screenRegion
		}
		return m, cmd

	case screenRegion:
		var sel *string
		var cmd tea.Cmd
		m.regionScreen, sel, cmd = m.regionScreen.Update(msg)
		if sel != nil {
			return m.startEphemeral(*sel)
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

// startChecking begins the profile flow: build clients, then resolve identity.
func (m rootModel) startChecking(p profiles.Profile) (tea.Model, tea.Cmd) {
	m.inSSOFlow = false
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
	idp, ec2c, ssmc, sess, region2, cleanup, err := m.deps.NewEphemeral(m.ssoSession, m.accountID, m.roleName, region)
	if err != nil {
		return m.toError("Não foi possível preparar as credenciais.", err.Error(), []errorAction{
			{label: "Escolher outra conta", next: screenAccounts},
			{label: "Sair", next: screenQuit},
		}), nil
	}
	m.idp, m.ec2, m.ssm, m.session = idp, ec2c, ssmc, sess
	m.region = region2
	m.cleanup = cleanup
	m.current = screenChecking
	m.loading = "Verificando sessão..."
	return m, loadIdentityCmd(m.idp, m.region)
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
