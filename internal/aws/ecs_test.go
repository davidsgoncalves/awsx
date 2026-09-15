package aws

import (
	"slices"
	"testing"
)

func TestTaskID(t *testing.T) {
	arn := "arn:aws:ecs:us-east-1:1:task/vakinha-stg/00d76868f0ab47e7904d34deacf1ceef"
	if got := TaskID(arn); got != "00d76868f0ab47e7904d34deacf1ceef" {
		t.Fatalf("got %q", got)
	}
	if got := TaskID("bare"); got != "bare" {
		t.Fatalf("want the input back when there is no slash, got %q", got)
	}
}

func TestServiceName(t *testing.T) {
	if got := serviceName("service:stg-api-web"); got != "stg-api-web" {
		t.Fatalf("got %q", got)
	}
	if got := serviceName("family:standalone"); got != "family:standalone" {
		t.Fatalf("non-service groups should be left alone, got %q", got)
	}
}

func TestAssembleTasks_ResolvesNodeAndSkipsStopped(t *testing.T) {
	tasks := []ecsTaskView{
		{arn: "arn/t1", group: "service:stg-web", status: "RUNNING", execEnabled: true,
			containerInstance: "ci-1", containers: []string{"web"}},
		{arn: "arn/t2", group: "service:stg-api-web", status: "RUNNING", execEnabled: true,
			containerInstance: "ci-2", containers: []string{"api-web"}},
		{arn: "arn/t3", group: "service:stg-api-web", status: "STOPPED", execEnabled: true,
			containerInstance: "ci-2", containers: []string{"api-web"}},
	}
	nodes := map[string]string{"ci-1": "i-aaa", "ci-2": "i-bbb"}

	got := assembleTasks("vakinha-stg", tasks, nodes)

	if len(got) != 2 {
		t.Fatalf("want 2 running entries, got %d: %+v", len(got), got)
	}
	if got[0].Service != "stg-api-web" || got[1].Service != "stg-web" {
		t.Fatalf("entries are not sorted by service: %+v", got)
	}
	if got[0].InstanceID != "i-bbb" {
		t.Fatalf("stg-api-web should resolve to i-bbb, got %q", got[0].InstanceID)
	}
	if got[0].Cluster != "vakinha-stg" {
		t.Fatalf("cluster not carried, got %q", got[0].Cluster)
	}
}

func TestAssembleTasks_OneEntryPerContainer(t *testing.T) {
	tasks := []ecsTaskView{
		{arn: "arn/t1", group: "service:stg-web", status: "RUNNING",
			containerInstance: "ci-1", containers: []string{"web", "nginx"}},
	}
	got := assembleTasks("c", tasks, map[string]string{"ci-1": "i-aaa"})
	if len(got) != 2 {
		t.Fatalf("want one entry per container, got %d", len(got))
	}
}

func TestAssembleTasks_FargateHasNoNode(t *testing.T) {
	tasks := []ecsTaskView{
		{arn: "arn/t1", group: "service:svc", status: "RUNNING", containers: []string{"app"}},
	}
	got := assembleTasks("c", tasks, map[string]string{})
	if got[0].InstanceID != "" {
		t.Fatalf("want empty instance for a task with no container instance, got %q", got[0].InstanceID)
	}
}

func TestBatches(t *testing.T) {
	var chunks [][]string
	for c := range batches([]string{"a", "b", "c", "d", "e"}, 2) {
		chunks = append(chunks, c)
	}
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(chunks))
	}
	if !slices.Equal(chunks[2], []string{"e"}) {
		t.Fatalf("last chunk = %v", chunks[2])
	}
	for range batches(nil, 100) {
		t.Fatal("empty input should yield nothing")
	}
}

func TestDisplayTask_FallsBackToContainer(t *testing.T) {
	if got := DisplayTask(ECSTask{Service: "svc", Container: "app"}); got != "svc" {
		t.Fatalf("got %q", got)
	}
	if got := DisplayTask(ECSTask{Container: "app"}); got != "app" {
		t.Fatalf("got %q", got)
	}
}

func TestECSExecArgs(t *testing.T) {
	got := ecsExecArgs("prod", "us-east-1", "vakinha-stg", "arn/t1", "api-web", "rails c")
	want := []string{
		"ecs", "execute-command", "--profile", "prod", "--region", "us-east-1",
		"--cluster", "vakinha-stg", "--task", "arn/t1", "--container", "api-web",
		"--interactive", "--command", "rails c",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestECSExecArgs_NoRegion(t *testing.T) {
	got := ecsExecArgs("prod", "", "c", "t", "x", "sh")
	if slices.Contains(got, "--region") {
		t.Fatalf("region flag should be omitted when empty: %v", got)
	}
}
