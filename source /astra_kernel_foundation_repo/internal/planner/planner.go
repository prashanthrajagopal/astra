
package planner

import "astra/internal/tasks"

type Planner struct {}

func New() *Planner{
return &Planner{}
}

func (p *Planner) Plan(goal string) tasks.Graph{
return tasks.Graph{
ID:"graph1",
Tasks:[]tasks.Task{
{ID:"t1",Type:"analyze"},
{ID:"t2",Type:"implement"},
},
}
}
