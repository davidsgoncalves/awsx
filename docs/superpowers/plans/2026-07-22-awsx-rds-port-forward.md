# AWSX RDS Port Forwarding — Plan

**Goal:** From the main menu, "Acessar banco/serviço (túnel)" discovers RDS
instances, lets the user pick one and an SSM-managed tunnel instance, then opens
an SSM port-forward session (`AWS-StartPortForwardingSessionToRemoteHost`) on an
auto-picked free local port. Works in both the profile and SSO flows.

**Decisions (confirmed):** auto-discover RDS (`rds:DescribeDBInstances`); auto
free local port; menu label "Acessar banco/serviço (túnel)". Deliver locally;
do NOT publish until the user tests.

## Flow
```
menu -> "Acessar banco/serviço (túnel)"
  -> rds:DescribeDBInstances -> RDS picker (name / engine / endpoint:port)
  -> pick RDS -> instance picker (SSM targets = tunnel host)
  -> pick instance -> pick free local port
  -> aws ssm start-session --target <i> \
       --document-name AWS-StartPortForwardingSessionToRemoteHost \
       --parameters host=<endpoint>,portNumber=<rport>,localPortNumber=<lport>
  -> "Túnel aberto: localhost:<lport> -> <endpoint>:<rport> (Ctrl+C encerra)"
  -> on end: return menu
```

## Tasks
1. `aws/rds.go`: `RDSInstance{Name, Engine, Endpoint, Port}`, `RDSLister` iface,
   `Clients.RDSInstances` (SDK, skips instances without an endpoint). Add rds
   client to `Clients`. Test: none live (thin wrapper); covered by build + smoke.
2. `aws/netutil.go`: `FreeLocalPort() (int, error)` via `net.Listen(127.0.0.1:0)`.
   Test: returns a port in range, and it is usable.
3. `aws/exec.go`: `portForwardArgs` + `CLI.PortForwardCommand(instanceID, host
   string, remotePort, localPort int) *exec.Cmd`. Test argv (with/without region).
4. `tui/screen_pick.go`: `rdsScreen` picker over `[]aws.RDSInstance`. Test select.
5. `tui/session.go`: `portForwarder` iface + `portForwardExec`.
6. `tui/root.go`: menu action; states `screenRDS`, `screenTunnelInstance`; msgs
   `rdsMsg`; cmd `loadRDSCmd`; store rds lister + tunnel state; wire NewClients /
   NewEphemeral to also return an `aws.RDSLister`. Tests (fakes): menu -> RDS ->
   instance -> exec.
7. `cli/root.go`: return rds lister from NewClients/NewEphemeral.
8. Gate (test/vet/lint/cross-build) + `go build` local install to ~/.local/bin.
   No release.

## Notes
- Requires `ssm:StartSession` on the tunnel instance + the port-forward document;
  `rds:DescribeDBInstances` to list. README/IAM updated when publishing.
- Port-forward session occupies the terminal like a shell session (tea.ExecProcess).
