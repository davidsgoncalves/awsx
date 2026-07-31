package aws

import "strings"

// dockerPSCommand lists running containers in a tab-separated form that
// parsePS understands. The Compose service label gives a readable name for
// containers started by docker compose, whose container names carry a
// Compose-assigned numeric suffix.
const dockerPSCommand = `docker ps --format '{{.ID}}\t{{.Names}}\t{{.Label "com.docker.compose.service"}}\t{{.Image}}\t{{.Status}}'`

// psFields is the number of tab-separated fields dockerPSCommand emits.
const psFields = 5

// parsePS turns the output of dockerPSCommand into containers, skipping lines
// that do not have the expected field count.
func parsePS(out string) []Container {
	var cs []Container
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != psFields {
			continue
		}
		cs = append(cs, Container{
			ID:      f[0],
			Name:    f[1],
			Service: f[2],
			Image:   f[3],
			Status:  f[4],
		})
	}
	return cs
}

// DisplayContainer returns the Compose service name, falling back to the
// container name.
func DisplayContainer(c Container) string {
	if c.Service != "" {
		return c.Service
	}
	return c.Name
}

// DockerExecLine builds the shell line that runs command inside container.
// sudo is required because the SSM session runs as ssm-user, which is not in
// the docker group and cannot reach /var/run/docker.sock. This is the single
// source of truth for both the on-screen preview and the executed command.
func DockerExecLine(container, command string) string {
	return "sudo docker exec -it " + container + " " + command
}
