
package memory

import "log"

type Memory struct {}

func New() *Memory {
    return &Memory{}
}

func (m *Memory) Write(agentID string, content string) {
    log.Println("memory write", agentID, content)
}

func (m *Memory) Search(agentID string, query string) []string {
    return []string{}
}
