
package tasks

type Status string

const (
Created Status="created"
Queued Status="queued"
Running Status="running"
Completed Status="completed"
Failed Status="failed"
)

type Task struct{
ID string
GraphID string
AgentID string
Type string
Status Status
Payload map[string]interface{}
Result map[string]interface{}
}

type Graph struct{
ID string
Tasks []Task
}
