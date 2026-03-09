
package main

import (
"log"
"time"
)

func main(){
log.Println("astra worker started")
for{
time.Sleep(5*time.Second)
log.Println("worker heartbeat")
}
}
