
package tasks

type Status string

const (
    Pending Status = "pending"
    Running Status = "running"
    Completed Status = "completed"
    Failed Status = "failed"
)

type Task struct {
    ID string
    AgentID string
    Type string
    Status Status
    Payload map[string]interface{}
}
