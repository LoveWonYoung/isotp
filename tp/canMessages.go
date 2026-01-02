package tp

type CanMessage struct {
	ArbitrationId int
	Dlc           int
	Data          []byte
	ExtendedId    bool
	IsFd          bool
	BitrateSwitch bool
}
