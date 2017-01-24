package messages

//messages sent directly over the wire

type AuthControl struct {
	Token string
}

type AuthTunnel struct {
	ClientID int
	Token    string
}

type OpenTunnel struct {
	ClientID int
}

type Shutdown struct {
	Error string
}
