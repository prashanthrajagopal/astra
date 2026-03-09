
package scheduler

import (
    "database/sql"
    "log"
)

type Scheduler struct {
    DB *sql.DB
}

func NewScheduler(db *sql.DB) *Scheduler {
    return &Scheduler{DB: db}
}

func (s *Scheduler) Start() {
    log.Println("Scheduler loop started")
    for {
        tasks := s.findReadyTasks()
        for _, t := range tasks {
            s.enqueueTask(t)
        }
    }
}

func (s *Scheduler) findReadyTasks() []string {
    return []string{}
}

func (s *Scheduler) enqueueTask(id string) {
    log.Println("enqueue task", id)
}
