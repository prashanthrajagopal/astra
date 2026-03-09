
package messaging

import "log"

type Bus struct {}

func New()*Bus{
return &Bus{}
}

func (b *Bus) Publish(stream string,payload interface{}){
log.Println("publish",stream)
}
