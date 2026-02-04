package middleware

type CallbackFunc func(topico string, payload interface{})

type Cliente interface {
	Desconectar()
	Publicar(topico string, mensaje interface{})
	Suscribir(topico string, manejador CallbackFunc)
	Desuscribir(topico string)
}

// almacena si el mensaje es original o replica
type Mensaje struct {
    Original bool `json:"original"`
    Topico   string `json:"topico"`
    Payload  []byte `json:"payload"`
    Interno bool `json:"interno"`
}
