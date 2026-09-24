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
	"github.com/davidsgoncalves/awsx/internal/update"
	"github.com/davidsgoncalves/awsx/internal/version"
)

// Deps is the injection seam for the TUI. Real wiring lives in cli/root.go;
// tests supply fakes.
type Deps struct {
	Profiles    []profiles.Profile
	SSOSessions []profiles.SSOSession
	Checks      []deps.Dependency

	// Profile flow. region is a region override; "" means resolve from the
	// profile/env (config.ErrNoRegion when none is configured).
	NewClients func(ctx context.Context, profile, region string) (awsx.IdentityProvider, awsx.EC2Lister, awsx.SSMLister, awsx.RDSLister, awsx.ContainerLister, awsx.ECSLister, string, error)
	NewCLI     func(profile, region string) (awsx.Login, awsx.Sessioner)

	// Optional cross-cutting services (nil-safe).
	Log   *logging.Logger
	State *state.State

	// SSO-session flow.
	NewDiscoverer func(ctx context.Context, session profiles.SSOSession) (awsx.SSODiscoverer, error)
	NewSSOLogin   func(session profiles.SSOSession) awsx.Login
	NewEphemeral  func(session profiles.SSOSession, accountID, roleName, region string) (idp awsx.IdentityProvider, ec2 awsx.EC2Lister, ssm awsx.SSMLister, rds awsx.RDSLister, containers awsx.ContainerLister, ecs awsx.ECSLister, sess awsx.Sessioner, resolvedRegion string, cleanup func(), err error)
}

// flow is which action is in progress. It decides where the flow goes next
// and how a finished or failed session is reported.
type flow int

const (
	flowSession flow = iota // EC2 > Sessão na instância
	flowTunnel              // EC2 > Túnel para banco RDS
	flowExec                // EC2 > Comando em container Docker
	flowECSExec             // ECS
)

type rootModel struct {
	deps    Deps
	current screen

	depsScreen      depsScreen
	selectionScreen selectionScreen
	accountScreen   accountScreen
	roleScreen      roleScreen
	regionScreen    regionScreen
	menuScreen       menuScreen
	instancesScreen  instancesScreen
	ec2ActionScreen  ec2ActionScreen
	rdsScreen        rdsScreen
	containersScreen containersScreen
	ecsClusterScreen ecsClusterScreen
	ecsTaskScreen    ecsTaskScreen
	ecsActionScreen  ecsActionScreen
	commandScreen    commandScreen
	updateScreen     updateScreen
	errorScreen      errorScreen

	// Profile / resolved-session state.
	profile string
	region  string
	idp        awsx.IdentityProvider
	ec2        awsx.EC2Lister
	ssm        awsx.SSMLister
	rds        awsx.RDSLister
	containers awsx.ContainerLister
	ecs        awsx.ECSLister
	login      awsx.Login
	session    awsx.Sessioner

	// Flow state.
	flow           flow
	tunnelInstance awsx.Target
	tunnelDB       awsx.RDSInstance

	// Exec (run-command) flow state.
	execInstance  awsx.Target
	execContainer awsx.Container

	// ECS exec flow state.
	ecsCluster string
	ecsTask    awsx.ECSTask

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
		m.current = screenInstances
		return m, nil
	case rdsMsg:
		dbs := filterDBsByVPC(msg.dbs, m.tunnelInstance.VpcID)
		if len(dbs) == 0 {
			return m.toError(
				"Nenhum banco alcançável a partir dessa instância.",
				fmt.Sprintf("Não há RDS na mesma VPC (%s) da instância escolhida, ou falta permissão rds:DescribeDBInstances.", m.tunnelInstance.VpcID),
				[]errorAction{
					{label: "Escolher outra instância", next: screenInstances},
					{label: "Voltar para o menu principal", next: screenMenu},
					{label: "Sair", next: screenQuit},
				},
			), nil
		}
		m.rdsScreen = newRDSScreen(dbs)
		m.rdsScreen, _, _ = m.rdsScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.rdsScreen.list.Title = fmt.Sprintf("Bancos alcançáveis por %s", awsx.DisplayName(m.tunnelInstance.Instance))
		m.current = screenRDS
		return m, nil
	case containersMsg:
		if len(msg.containers) == 0 {
			return m.toError(
				"Nenhum container em execução nessa instância.",
				fmt.Sprintf("`docker ps` não retornou nada em %s (%s). A instância pode não rodar containers, ou o Docker pode estar parado.",
					awsx.DisplayName(m.execInstance.Instance), m.execInstance.ID),
				[]errorAction{
					{label: "Escolher outra instância", next: screenInstances},
					{label: "Voltar para o menu principal", next: screenMenu},
					{label: "Sair", next: screenQuit},
				},
			), nil
		}
		m.containersScreen = newContainersScreen(msg.containers)
		m.containersScreen, _, _ = m.containersScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.containersScreen.list.Title = fmt.Sprintf("Containers em %s", awsx.DisplayName(m.execInstance.Instance))
		m.current = screenContainers
		return m, nil
	case ecsClustersMsg:
		if len(msg.clusters) == 0 {
			return m.toError(
				"Nenhum cluster ECS encontrado nessa região.",
				fmt.Sprintf("Não há clusters em %s, ou falta permissão ecs:ListClusters.", m.region),
				[]errorAction{
					{label: "Voltar para o menu principal", next: screenMenu},
					{label: "Sair", next: screenQuit},
				},
			), nil
		}
		if len(msg.clusters) == 1 {
			m.ecsCluster = msg.clusters[0]
			m.current = screenChecking
			m.loading = "Carregando containers..."
			return m, loadECSTasksCmd(m.ecs, m.ecsCluster)
		}
		m.ecsClusterScreen = newECSClusterScreen(msg.clusters)
		m.ecsClusterScreen, _, _ = m.ecsClusterScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.current = screenECSCluster
		return m, nil
	case ecsTasksMsg:
		if len(msg.tasks) == 0 {
			return m.toError(
				"Nenhum container em execução nesse cluster.",
				fmt.Sprintf("O cluster %s não tem tasks RUNNING.", msg.cluster),
				[]errorAction{
					{label: "Escolher outro cluster", next: screenECSCluster},
					{label: "Voltar para o menu principal", next: screenMenu},
					{label: "Sair", next: screenQuit},
				},
			), nil
		}
		m.ecsTaskScreen = newECSTaskScreen(msg.tasks)
		m.ecsTaskScreen, _, _ = m.ecsTaskScreen.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.ecsTaskScreen.list.Title = fmt.Sprintf("Containers em %s", msg.cluster)
		m.current = screenECSTask
		return m, nil
	case updateCheckMsg:
		m.updateScreen = m.updateScreen.applied(msg)
		if msg.err != nil {
			m.deps.Log.Error("update check failed: %v", msg.err)
		}
		return m, nil
	case updateDoneMsg:
		m.updateScreen.stage = updateDone
		m.updateScreen.err = msg.err
		if msg.err != nil {
			m.deps.Log.Error("update failed (method=%v path=%s): %v",
				m.updateScreen.method, m.updateScreen.path, msg.err)
		}
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
		back := errorAction{label: "Voltar para a instância", next: screenEC2Action}
		switch m.flow {
		case flowTunnel:
			back = errorAction{label: "Voltar para os bancos", next: screenRDS}
		case flowExec:
			back = errorAction{label: "Voltar para o comando", next: screenCommand}
		case flowECSExec:
			back = errorAction{label: "Voltar para o container", next: screenECSAction}
		}
		actions := []errorAction{
			back,
			{label: "Voltar para o menu principal", next: screenMenu},
			{label: "Sair", next: screenQuit},
		}
		if msg.err != nil {
			m.deps.Log.Error("ssm session failed for %s (%s) region=%s: %v | %s",
				m.connectingName, m.connectingID, m.region, msg.err, msg.stderr)
			awsErr := msg.stderr
			if awsErr == "" {
				awsErr = msg.err.Error()
			}
			detail := fmt.Sprintf("Alvo: %s (%s)\nRegião: %s\nErro: %s\n\nDetalhes no log: %s",
				m.connectingName, m.connectingID, m.region, awsErr, logging.Path())
			title := "Não foi possível abrir a sessão SSM."
			switch m.flow {
			case flowTunnel:
				title = "Não foi possível abrir o túnel."
			case flowExec:
				title = "Não foi possível rodar o comando."
			}
			return m.toError(title, detail, actions), nil
		}
		title := "Sessão encerrada."
		switch m.flow {
		case flowTunnel:
			title = "Túnel encerrado."
		case flowExec:
			title = "Comando encerrado."
		}
		return m.toError(title, "", actions), nil
	case errMsg:
		// A failure while listing containers is recoverable: the command can
		// still be typed by hand. This must come before the login check below,
		// which would otherwise treat it as an expired session.
		if m.flow == flowExec && m.current == screenChecking && m.execContainer.Name == "" {
			m.deps.Log.Error("listing containers on %s failed: %v", m.execInstance.ID, msg.err)
			var detail string
			if msg.action != "" {
				detail = "Permissão necessária: " + msg.action + "\n\n"
			}
			if msg.err != nil {
				detail += msg.err.Error()
			}
			detail += "\n\nDetalhes no log: " + logging.Path()
			m.commandScreen = newManualCommandScreen(awsx.DisplayName(m.execInstance.Instance))
			return m.toError("Não foi possível listar os containers.", detail, []errorAction{
				{label: "Digitar o comando à mão", next: screenCommand},
				{label: "Escolher outra instância", next: screenInstances},
				{label: "Voltar para o menu principal", next: screenMenu},
				{label: "Sair", next: screenQuit},
			}), nil
		}
		// During the checking phase, an identity failure means the session is
		// expired/invalid: route to SSO login instead of a generic error.
		if m.current == screenChecking && m.login != nil {
			m.current = screenLogin
			return m, loginCmd(m.login)
		}
		m.deps.Log.Error("operation failed (action=%q): %v", msg.action, msg.err)
		var detail string
		if msg.action != "" {
			detail = "Permissão necessária: " + msg.action + "\n\n"
		}
		if msg.err != nil {
			detail += msg.err.Error()
		}
		detail += "\n\nDetalhes no log: " + logging.Path()
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
			m.flow = flowSession
			m.current = screenChecking
			m.loading = "Carregando instâncias..."
			return m, loadTargetsCmd(m.ec2, m.ssm)
		case screenECSCluster:
			m.flow = flowECSExec
			m.current = screenChecking
			m.loading = "Carregando clusters..."
			return m, loadECSClustersCmd(m.ecs)
		case screenUpdate:
			m.updateScreen = newUpdateScreen(version.Current())
			m.current = screenUpdate
			return m, checkUpdateCmd()
		case screenQuit:
			return m.quit()
		}
		return m, nil

	case screenInstances:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc && !m.instancesScreen.list.SettingFilter() {
			m.current = screenMenu
			return m, nil
		}
		var sel *awsx.Target
		var cmd tea.Cmd
		m.instancesScreen, sel, cmd = m.instancesScreen.Update(msg)
		if sel != nil {
			m.ec2ActionScreen = newEC2ActionScreen(*sel)
			m.current = screenEC2Action
		}
		return m, cmd

	case screenEC2Action:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc {
			m.current = screenInstances
			return m, nil
		}
		var action ec2Action
		m.ec2ActionScreen, action = m.ec2ActionScreen.Update(msg)
		t := m.ec2ActionScreen.target
		switch action {
		case ec2ActionSession:
			m.flow = flowSession
			name := awsx.DisplayName(t.Instance)
			m.connectingName, m.connectingID = name, t.ID
			m.deps.Log.Debug("opening ssm session: instance=%s (%s) region=%s", name, t.ID, m.region)
			return m, execWithCapture(sessionExec(m.session, t.ID, name))
		case ec2ActionExec:
			m.flow = flowExec
			m.execInstance = t
			// Clear any container from a previous round so a listing failure
			// is recognised as such.
			m.execContainer = awsx.Container{}
			m.current = screenChecking
			m.loading = "Carregando containers..."
			return m, loadContainersCmd(m.containers, t.ID)
		case ec2ActionTunnel:
			m.flow = flowTunnel
			m.tunnelInstance = t
			m.current = screenChecking
			m.loading = "Carregando bancos..."
			return m, loadRDSCmd(m.rds)
		}
		return m, nil

	case screenRDS:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc && !m.rdsScreen.list.SettingFilter() {
			m.current = screenEC2Action
			return m, nil
		}
		var sel *awsx.RDSInstance
		var cmd tea.Cmd
		m.rdsScreen, sel, cmd = m.rdsScreen.Update(msg)
		if sel != nil {
			return m.startTunnel(*sel)
		}
		return m, cmd

	case screenContainers:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc && !m.containersScreen.list.SettingFilter() {
			m.current = screenEC2Action
			return m, nil
		}
		var sel *awsx.Container
		var cmd tea.Cmd
		m.containersScreen, sel, cmd = m.containersScreen.Update(msg)
		if sel != nil {
			m.execContainer = *sel
			m.commandScreen = newCommandScreen(*sel, m.commandHistory(*sel))
			m.current = screenCommand
		}
		return m, cmd

	case screenECSCluster:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc && !m.ecsClusterScreen.list.SettingFilter() {
			m.current = screenMenu
			return m, nil
		}
		var sel *string
		var cmd tea.Cmd
		m.ecsClusterScreen, sel, cmd = m.ecsClusterScreen.Update(msg)
		if sel != nil {
			m.ecsCluster = *sel
			m.current = screenChecking
			m.loading = "Carregando containers..."
			return m, loadECSTasksCmd(m.ecs, *sel)
		}
		return m, cmd

	case screenECSTask:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc && !m.ecsTaskScreen.list.SettingFilter() {
			m.current = screenMenu
			return m, nil
		}
		var sel *awsx.ECSTask
		var cmd tea.Cmd
		m.ecsTaskScreen, sel, cmd = m.ecsTaskScreen.Update(msg)
		if sel != nil {
			m.ecsTask = *sel
			m.ecsActionScreen = newECSActionScreen(*sel)
			m.current = screenECSAction
		}
		return m, cmd

	case screenECSAction:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc {
			m.current = screenECSTask
			return m, nil
		}
		var action ecsAction
		m.ecsActionScreen, action = m.ecsActionScreen.Update(msg)
		switch action {
		case ecsActionCommand:
			if !m.ecsTask.ExecEnabled {
				return m.execDisabled(), nil
			}
			m.commandScreen = newECSCommandScreen(m.ecsTask, m.ecsCommandHistory(m.ecsTask))
			m.current = screenCommand
		case ecsActionShell:
			if !m.ecsTask.ExecEnabled {
				return m.execDisabled(), nil
			}
			return m.startECSShell()
		case ecsActionHost:
			return m.startECSHost()
		}
		return m, nil

	case screenCommand:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc {
			if m.flow == flowECSExec {
				m.current = screenECSAction
				return m, nil
			}
			m.current = screenContainers
			return m, nil
		}
		var sub *commandSubmit
		var cmd tea.Cmd
		m.commandScreen, sub, cmd = m.commandScreen.Update(msg)
		if sub != nil {
			if m.flow == flowECSExec {
				return m.startECSExec(*sub)
			}
			return m.startExec(*sub)
		}
		return m, cmd

	case screenUpdate:
		if km, ok := msg.(tea.KeyMsg); ok && km.Type == tea.KeyEsc {
			m.current = screenMenu
			return m, nil
		}
		var apply bool
		m.updateScreen, apply = m.updateScreen.Update(msg)
		if !apply {
			return m, nil
		}
		if m.updateScreen.method == update.MethodBrew {
			return m, brewUpgradeCmd()
		}
		return m, applyUpdateCmd(m.updateScreen.latest, m.updateScreen.path)

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
	idp, ec2c, ssmc, rdsc, contc, ecsc, resolved, err := m.deps.NewClients(context.Background(), p.Name, region)
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
	m.idp, m.ec2, m.ssm, m.rds, m.containers, m.ecs, m.region = idp, ec2c, ssmc, rdsc, contc, ecsc, resolved
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
	idp, ec2c, ssmc, rdsc, contc, ecsc, sess, region2, cleanup, err := m.deps.NewEphemeral(m.ssoSession, m.accountID, m.roleName, region)
	if err != nil {
		m.deps.Log.Error("prepare ephemeral creds failed (session=%s account=%s role=%s region=%s): %v",
			m.ssoSession.Name, m.accountID, m.roleName, region, err)
		return m.toError("Não foi possível preparar as credenciais.", err.Error(), []errorAction{
			{label: "Escolher outra conta", next: screenAccounts},
			{label: "Sair", next: screenQuit},
		}), nil
	}
	m.remember(state.SSOKey(m.ssoSession.Name, m.accountID, m.roleName), region)
	m.rds, m.containers, m.ecs = rdsc, contc, ecsc
	m.idp, m.ec2, m.ssm, m.session = idp, ec2c, ssmc, sess
	m.region = region2
	m.cleanup = cleanup
	m.current = screenChecking
	m.loading = "Verificando sessão..."
	return m, loadIdentityCmd(m.idp, m.region)
}

// startTunnel opens an SSM port-forward from a free local port to the chosen
// RDS endpoint, through the already-selected tunnel instance.
func (m rootModel) startTunnel(db awsx.RDSInstance) (tea.Model, tea.Cmd) {
	m.tunnelDB = db
	localPort, err := awsx.FreeLocalPort()
	if err != nil {
		m.deps.Log.Error("could not find a free local port: %v", err)
		return m.toError("Não foi possível abrir uma porta local.", err.Error(), []errorAction{
			{label: "Voltar para o menu principal", next: screenMenu},
			{label: "Sair", next: screenQuit},
		}), nil
	}
	id := m.tunnelInstance.ID
	m.connectingName = fmt.Sprintf("%s via %s", db.Name, awsx.DisplayName(m.tunnelInstance.Instance))
	m.connectingID = id
	m.deps.Log.Debug("opening ssm tunnel: db=%s (%s:%d) via instance=%s local=%d region=%s",
		db.Name, db.Endpoint, db.Port, id, localPort, m.region)

	return m, execWithCapture(portForwardExec(m.session, id, db.Endpoint, db.Port, localPort))
}

// commandHistory returns the remembered commands for a container.
func (m rootModel) commandHistory(c awsx.Container) []string {
	if m.deps.State == nil {
		return nil
	}
	return m.deps.State.CommandHistory(state.ContainerKey(awsx.DisplayContainer(c)))
}

// startExec runs the confirmed command inside the chosen container, recording
// it in the per-container history first.
func (m rootModel) startExec(sub commandSubmit) (tea.Model, tea.Cmd) {
	if sub.Inner != "" && m.deps.State != nil {
		m.deps.State.PushCommand(state.ContainerKey(awsx.DisplayContainer(m.execContainer)), sub.Inner)
		if err := m.deps.State.Save(); err != nil {
			m.deps.Log.Error("could not save state: %v", err)
		}
	}
	id := m.execInstance.ID
	m.connectingName = fmt.Sprintf("%s em %s", awsx.DisplayContainer(m.execContainer), awsx.DisplayName(m.execInstance.Instance))
	m.connectingID = id
	m.deps.Log.Debug("running command: instance=%s container=%s region=%s line=%q",
		id, m.execContainer.Name, m.region, sub.Line)

	return m, execWithCapture(interactiveExec(m.session, id, sub.Line))
}

// ecsCommandHistory returns the remembered commands for an ECS service.
func (m rootModel) ecsCommandHistory(t awsx.ECSTask) []string {
	if m.deps.State == nil {
		return nil
	}
	return m.deps.State.CommandHistory(state.ContainerKey(awsx.DisplayTask(t)))
}

// startECSExec opens an ECS Exec session inside the chosen container. The task
// carries its own placement, so no instance is selected along the way.
func (m rootModel) startECSExec(sub commandSubmit) (tea.Model, tea.Cmd) {
	if sub.Inner != "" && m.deps.State != nil {
		m.deps.State.PushCommand(state.ContainerKey(awsx.DisplayTask(m.ecsTask)), sub.Inner)
		if err := m.deps.State.Save(); err != nil {
			m.deps.Log.Error("could not save state: %v", err)
		}
	}
	t := m.ecsTask
	m.connectingName = fmt.Sprintf("%s em %s", awsx.DisplayTask(t), t.Cluster)
	m.connectingID = awsx.TaskID(t.TaskARN)
	m.deps.Log.Debug("running ecs exec: cluster=%s task=%s container=%s instance=%s region=%s line=%q",
		t.Cluster, t.TaskARN, t.Container, t.InstanceID, m.region, awsx.ECSShellLine(sub.Line))

	return m, execWithCapture(ecsExec(m.session, t.Cluster, t.TaskARN, t.Container, awsx.ECSShellLine(sub.Line)))
}

// startECSShell opens a shell inside the chosen container. Nothing was typed,
// so there is no command to remember.
func (m rootModel) startECSShell() (tea.Model, tea.Cmd) {
	t := m.ecsTask
	m.connectingName = fmt.Sprintf("shell em %s", awsx.DisplayTask(t))
	m.connectingID = awsx.TaskID(t.TaskARN)
	m.deps.Log.Debug("opening ecs shell: cluster=%s task=%s container=%s region=%s",
		t.Cluster, t.TaskARN, t.Container, m.region)

	return m, execWithCapture(ecsExec(m.session, t.Cluster, t.TaskARN, t.Container,
		awsx.ECSShellLine(awsx.ECSInteractiveShell())))
}

// startECSHost opens an SSM session on the instance the task landed on. This
// reaches the node, not the container, and does not need ECS Exec.
func (m rootModel) startECSHost() (tea.Model, tea.Cmd) {
	t := m.ecsTask
	m.connectingName = fmt.Sprintf("host de %s", awsx.DisplayTask(t))
	m.connectingID = t.InstanceID
	m.deps.Log.Debug("opening ssm session on ecs host: instance=%s task=%s region=%s",
		t.InstanceID, awsx.TaskID(t.TaskARN), m.region)

	return m, execWithCapture(sessionExec(m.session, t.InstanceID, m.connectingName))
}

// execDisabled explains that the task was started without ECS Exec, pointing
// at the host session as the way in that does not depend on it.
func (m rootModel) execDisabled() rootModel {
	t := m.ecsTask
	detail := fmt.Sprintf("A task %s roda com enableExecuteCommand desligado. Habilite no serviço e faça um novo deploy.",
		awsx.TaskID(t.TaskARN))
	actions := []errorAction{}
	if t.InstanceID != "" {
		detail += fmt.Sprintf(" O host %s continua acessível por SSM.", t.InstanceID)
		actions = append(actions, errorAction{label: "Voltar para o container", next: screenECSAction})
	}
	actions = append(actions,
		errorAction{label: "Escolher outro container", next: screenECSTask},
		errorAction{label: "Voltar para o menu principal", next: screenMenu},
		errorAction{label: "Sair", next: screenQuit},
	)
	return m.toError("Esse container não aceita ECS Exec.", detail, actions)
}

// filterDBsByVPC keeps the RDS instances in the same VPC as the tunnel instance.
// If the instance has no VPC (unusual), all databases are returned.
func filterDBsByVPC(dbs []awsx.RDSInstance, vpcID string) []awsx.RDSInstance {
	if vpcID == "" {
		return dbs
	}
	var out []awsx.RDSInstance
	for _, db := range dbs {
		if db.VpcID == vpcID {
			out = append(out, db)
		}
	}
	return out
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
	case screenEC2Action:
		m.current = screenEC2Action
		return m, nil
	case screenContainers:
		m.current = screenContainers
		return m, nil
	case screenCommand:
		m.current = screenCommand
		return m, nil
	case screenECSCluster:
		m.current = screenChecking
		m.loading = "Carregando clusters..."
		return m, loadECSClustersCmd(m.ecs)
	case screenECSTask:
		m.current = screenChecking
		m.loading = "Carregando containers..."
		return m, loadECSTasksCmd(m.ecs, m.ecsCluster)
	case screenECSAction:
		m.current = screenECSAction
		return m, nil
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
	case screenEC2Action:
		return m.ec2ActionScreen.View()
	case screenRDS:
		return m.rdsScreen.View()
	case screenContainers:
		return m.containersScreen.View()
	case screenECSCluster:
		return m.ecsClusterScreen.View()
	case screenECSTask:
		return m.ecsTaskScreen.View()
	case screenECSAction:
		return m.ecsActionScreen.View()
	case screenUpdate:
		return m.updateScreen.View()
	case screenCommand:
		return m.commandScreen.View()
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
