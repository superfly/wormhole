package messages

// AuthMessage ...
type AuthMessage struct {
	Token   string
	Name    string
	Client  string
	Release *Release
}
