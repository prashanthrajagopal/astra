
package scheduler

import (
"log"
)

type Scheduler struct {}

func New() *Scheduler {
return &Scheduler{}
}

func (s *Scheduler) Start(){
log.Println("scheduler started")
for{
s.Tick()
}
}

func (s *Scheduler) Tick(){
log.Println("scheduler tick")
}
