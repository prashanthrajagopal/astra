
package main

import (
    "log"
    "net/http"
)

func main() {
    http.HandleFunc("/health", func(w http.ResponseWriter,r *http.Request){
        w.Write([]byte("ok"))
    })
    log.Println("api gateway started")
    http.ListenAndServe(":8080",nil)
}
