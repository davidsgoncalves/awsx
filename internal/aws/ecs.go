package aws

import (
	"context"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

// describeBatch is the maximum number of items DescribeTasks and
// DescribeContainerInstances accept per call.
const describeBatch = 100

// ECSTask is one container of a running ECS task, together with the node it
// landed on. A task with several containers yields one entry per container.
type ECSTask struct {
	Cluster     string
	TaskARN     string
	Service     string
	Container   string
	InstanceID  string
	PrivateIP   string
	Status      string
	ExecEnabled bool
}

// ECSLister reads the ECS clusters and their running tasks.
type ECSLister interface {
	Clusters(ctx context.Context) ([]string, error)
	Tasks(ctx context.Context, cluster string) ([]ECSTask, error)
}

// TaskID is the trailing identifier of a task ARN.
func TaskID(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}
	return arn
}

// Clusters implements ECSLister.
func (c *Clients) Clusters(ctx context.Context) ([]string, error) {
	var out []string
	p := ecs.NewListClustersPaginator(c.ecs, &ecs.ListClustersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, arn := range page.ClusterArns {
			out = append(out, TaskID(arn))
		}
	}
	sort.Strings(out)
	return out, nil
}

// Tasks implements ECSLister. It resolves each task to the EC2 instance
// running it, so the caller never has to pick a node by hand.
func (c *Clients) Tasks(ctx context.Context, cluster string) ([]ECSTask, error) {
	arns, err := c.taskARNs(ctx, cluster)
	if err != nil {
		return nil, err
	}
	if len(arns) == 0 {
		return nil, nil
	}
	var described []ecsTaskView
	for chunk := range batches(arns, describeBatch) {
		out, err := c.ecs.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: &cluster,
			Tasks:   chunk,
		})
		if err != nil {
			return nil, err
		}
		described = append(described, viewTasks(out)...)
	}
	nodes, err := c.containerInstanceNodes(ctx, cluster, described)
	if err != nil {
		return nil, err
	}
	return assembleTasks(cluster, described, nodes), nil
}

func (c *Clients) taskARNs(ctx context.Context, cluster string) ([]string, error) {
	var arns []string
	p := ecs.NewListTasksPaginator(c.ecs, &ecs.ListTasksInput{Cluster: &cluster})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		arns = append(arns, page.TaskArns...)
	}
	return arns, nil
}

// containerInstanceNodes maps container-instance ARNs to their EC2 instance.
// Tasks on Fargate carry no container instance and are left unmapped.
func (c *Clients) containerInstanceNodes(ctx context.Context, cluster string, tasks []ecsTaskView) (map[string]string, error) {
	var arns []string
	seen := map[string]bool{}
	for _, t := range tasks {
		if t.containerInstance == "" || seen[t.containerInstance] {
			continue
		}
		seen[t.containerInstance] = true
		arns = append(arns, t.containerInstance)
	}
	nodes := map[string]string{}
	for chunk := range batches(arns, describeBatch) {
		out, err := c.ecs.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{
			Cluster:            &cluster,
			ContainerInstances: chunk,
		})
		if err != nil {
			return nil, err
		}
		for _, ci := range out.ContainerInstances {
			nodes[deref(ci.ContainerInstanceArn)] = deref(ci.Ec2InstanceId)
		}
	}
	return nodes, nil
}

// ecsTaskView is the part of a described task the assembly needs.
type ecsTaskView struct {
	arn               string
	group             string
	status            string
	execEnabled       bool
	containerInstance string
	containers        []string
}

func viewTasks(out *ecs.DescribeTasksOutput) []ecsTaskView {
	var vs []ecsTaskView
	for _, t := range out.Tasks {
		v := ecsTaskView{
			arn:               deref(t.TaskArn),
			group:             deref(t.Group),
			status:            deref(t.LastStatus),
			execEnabled:       t.EnableExecuteCommand,
			containerInstance: deref(t.ContainerInstanceArn),
		}
		for _, c := range t.Containers {
			v.containers = append(v.containers, deref(c.Name))
		}
		vs = append(vs, v)
	}
	return vs
}

// assembleTasks flattens described tasks into one entry per running container,
// sorted by service then container.
func assembleTasks(cluster string, tasks []ecsTaskView, nodes map[string]string) []ECSTask {
	var out []ECSTask
	for _, t := range tasks {
		if t.status != "RUNNING" {
			continue
		}
		for _, name := range t.containers {
			out = append(out, ECSTask{
				Cluster:     cluster,
				TaskARN:     t.arn,
				Service:     serviceName(t.group),
				Container:   name,
				InstanceID:  nodes[t.containerInstance],
				Status:      t.status,
				ExecEnabled: t.execEnabled,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Service != out[j].Service {
			return out[i].Service < out[j].Service
		}
		return out[i].Container < out[j].Container
	})
	return out
}

// serviceName turns the task group "service:NAME" into NAME, leaving other
// group forms (a standalone task family) untouched.
func serviceName(group string) string {
	return strings.TrimPrefix(group, "service:")
}

// batches yields successive slices of at most size elements.
func batches(items []string, size int) func(func([]string) bool) {
	return func(yield func([]string) bool) {
		for i := 0; i < len(items); i += size {
			end := min(i+size, len(items))
			if !yield(items[i:end]) {
				return
			}
		}
	}
}

// ECSShellLine wraps command so it runs under a shell. ECS Exec execs the
// --command argument directly, with no shell and a minimal PATH, so a bare
// `rails c` fails even when the binstub is right there in the working
// directory; pipes, redirections and variables would be lost the same way.
func ECSShellLine(command string) string {
	return "/bin/sh -c " + shellQuote(command)
}

// shellQuote wraps s in single quotes, ending and reopening the quoted run
// around each single quote it contains. The ECS agent splits the command with
// shell quoting rules, so this survives the round trip.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// DisplayTask names a task entry: the service, or the container when the task
// does not belong to a service.
func DisplayTask(t ECSTask) string {
	if t.Service != "" {
		return t.Service
	}
	return t.Container
}
