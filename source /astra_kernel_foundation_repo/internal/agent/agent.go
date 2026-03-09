
package agent

import (
"astra/internal/tasks"
"log"
)

type Agent struct{
ID string
}

func New(id string)*Agent{
return &Agent{ID:id}
}

func (a *Agent) Execute(t tasks.Task){
log.Println("agent executing task",t.ID)
}
