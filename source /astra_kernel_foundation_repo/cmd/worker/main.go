
package main

import (
"time"
"log"
)

func main(){
log.Println("worker started")
for{
time.Sleep(5*time.Second)
log.Println("worker polling")
}
}
