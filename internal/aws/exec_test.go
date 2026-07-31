package aws

import (
	"context"
	"slices"
	"testing"
)

func TestLoginArgs(t *testing.T) {
	got := loginArgs("prod")
	want := []string{"sso", "login", "--profile", "prod"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSessionArgs_NoRegion(t *testing.T) {
	got := sessionArgs("prod", "", "i-abc")
	want := []string{"ssm", "start-session", "--profile", "prod", "--target", "i-abc"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSessionArgs_WithRegion(t *testing.T) {
	got := sessionArgs("prod", "sa-east-1", "i-abc")
	want := []string{"ssm", "start-session", "--profile", "prod", "--region", "sa-east-1", "--target", "i-abc"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPortForwardArgs(t *testing.T) {
	got := portForwardArgs("prod", "sa-east-1", "i-1", "db.rds.amazonaws.com", 5432, 54321)
	want := []string{
		"ssm", "start-session", "--profile", "prod", "--region", "sa-east-1",
		"--target", "i-1",
		"--document-name", "AWS-StartPortForwardingSessionToRemoteHost",
		"--parameters", "host=db.rds.amazonaws.com,portNumber=5432,localPortNumber=54321",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPortForwardArgs_NoRegion(t *testing.T) {
	got := portForwardArgs("prod", "", "i-1", "h", 6379, 6380)
	want := []string{
		"ssm", "start-session", "--profile", "prod",
		"--target", "i-1",
		"--document-name", "AWS-StartPortForwardingSessionToRemoteHost",
		"--parameters", "host=h,portNumber=6379,localPortNumber=6380",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInteractiveCommandArgs(t *testing.T) {
	got := interactiveCommandArgs("prod", "sa-east-1", "i-1", "sudo docker exec -it web rails c")
	want := []string{
		"ssm", "start-session", "--profile", "prod", "--region", "sa-east-1",
		"--target", "i-1",
		"--document-name", "AWS-StartInteractiveCommand",
		"--parameters", `{"command":["sudo docker exec -it web rails c"]}`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInteractiveCommandArgs_NoRegion(t *testing.T) {
	got := interactiveCommandArgs("prod", "", "i-1", "bash")
	want := []string{
		"ssm", "start-session", "--profile", "prod",
		"--target", "i-1",
		"--document-name", "AWS-StartInteractiveCommand",
		"--parameters", `{"command":["bash"]}`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A comma in the command would be read as a list separator by the AWS CLI's
// key=value shorthand, silently splitting it into two parameters. JSON encoding
// is what keeps the command intact.
func TestInteractiveCommandArgs_CommandWithCommaStaysOneParameter(t *testing.T) {
	got := interactiveCommandArgs("prod", "", "i-1", `rails runner "puts [1,2]"`)
	want := `{"command":["rails runner \"puts [1,2]\""]}`
	if got[len(got)-1] != want {
		t.Fatalf("parameters = %s, want %s", got[len(got)-1], want)
	}
}

func TestInteractiveCommand_WithConfigFile(t *testing.T) {
	c := CLI{Profile: "_awsx", ConfigFile: "/tmp/awsx/config"}
	cmd := c.InteractiveCommand("i-1", "bash")

	if !slices.Contains(cmd.Args, "AWS-StartInteractiveCommand") {
		t.Fatalf("args missing document name: %v", cmd.Args)
	}
	if !slices.Contains(cmd.Env, "AWS_CONFIG_FILE=/tmp/awsx/config") {
		t.Fatalf("AWS_CONFIG_FILE not set in env: %v", cmd.Env)
	}
}

func TestSSOSessionLoginArgs(t *testing.T) {
	got := ssoSessionLoginArgs("vakinha")
	want := []string{"sso", "login", "--sso-session", "vakinha"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSessionCommand_WithConfigFile(t *testing.T) {
	c := CLI{Profile: "_awsx", ConfigFile: "/tmp/awsx/config"}
	cmd := c.SessionCommand("i-1")

	wantArgs := []string{"aws", "ssm", "start-session", "--profile", "_awsx", "--target", "i-1"}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Fatalf("args = %v, want %v", cmd.Args, wantArgs)
	}
	if !slices.Contains(cmd.Env, "AWS_CONFIG_FILE=/tmp/awsx/config") {
		t.Fatalf("AWS_CONFIG_FILE not set in env: %v", cmd.Env)
	}
}

func TestSessionCommand_NoConfigFileNoEnvOverride(t *testing.T) {
	c := CLI{Profile: "prod"}
	cmd := c.SessionCommand("i-1")
	if cmd.Env != nil {
		t.Fatalf("expected nil env (inherit), got %v", cmd.Env)
	}
}

var _ Login = SSOSessionLogin{}

func TestSSOSessionLogin_Interface(t *testing.T) {
	// Compile-time check above; ensure the ctx param is accepted.
	var l Login = SSOSessionLogin{Session: "vakinha"}
	_ = l
	_ = context.Background()
}
