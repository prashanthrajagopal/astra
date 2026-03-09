
package llm

import "log"

type Router struct {}

func NewRouter() *Router {
    return &Router{}
}

func (r *Router) Route(prompt string) string {
    log.Println("routing prompt")
    return "local-model"
}
