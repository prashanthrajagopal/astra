
package main

import (
"net/http"
"log"
)

func main(){
http.HandleFunc("/health",func(w http.ResponseWriter,r *http.Request){
w.Write([]byte("ok"))
})
log.Println("api gateway running")
http.ListenAndServe(":8080",nil)
}
