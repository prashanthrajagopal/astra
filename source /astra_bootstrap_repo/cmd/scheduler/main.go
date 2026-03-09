
package main

import (
    "database/sql"
    _ "github.com/lib/pq"
    "astra/internal/scheduler"
)

func main() {
    db, _ := sql.Open("postgres","postgres://localhost/astra")
    s := scheduler.NewScheduler(db)
    s.Start()
}
