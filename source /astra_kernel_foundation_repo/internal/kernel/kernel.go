
package kernel

import (
"log"
"astra/internal/actors"
)

type Kernel struct {
Actors map[string]actors.Actor
}

func NewKernel() *Kernel {
return &Kernel{
Actors:make(map[string]actors.Actor),
}
}

func (k *Kernel) Spawn(a actors.Actor){
k.Actors[a.ID()] = a
log.Println("spawn actor",a.ID())
}

func (k *Kernel) Send(target string,msg actors.Message){
if a,ok := k.Actors[target]; ok{
a.Receive(nil,msg)
}
}
