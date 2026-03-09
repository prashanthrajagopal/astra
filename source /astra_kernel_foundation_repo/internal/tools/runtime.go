
package tools

import "log"

type Runtime struct {}

func New()*Runtime{
return &Runtime{}
}

func (r *Runtime) Run(name string,input string)string{
log.Println("tool run",name)
return ""
}
