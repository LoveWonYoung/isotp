package tp

const (
	Normal11bits uint32 = iota
	Normal29bits
	NormalFixed29bits
	Extended11bits
	Extended29bits
	Mixed11bits
	Mixed29bits
)
const (
	Physical = iota
	Functional
)

type AbstractAddress interface {
	GetTxArbitrationId(addressType uint32) int
	GetRxArbitrationId(addressType uint32) int
	RequiresTxExtensionByte() bool
	RequiresRxExtensionByte() bool
	GetTxExtensionByte() (int, bool)
	GetRxExtensionByte() (int, bool)
	IsTx29Bit() bool
	IsRx29Bit() bool
	IsForMe(msg CanMessage) bool
}
type Address struct {
	AddressingMode            uint32
	TargetAddress             int
	SourceAddress             int
	AddressExtension          int
	TxId                      int
	RxId                      int
	Is29Bits                  bool
	TxArbitrationIdPhysical   int
	TxArbitrationIdFunctional int
	RxArbitrationIdPhysical   int
	RxArbitrationIdFunctional int
	TxpayloadPrefix           []byte
	RxprefixSize              int
	RxOnly                    bool
	TxOnly                    bool
	PhysicalId                int
	FunctionalId              int
}

// NewAddress /*
func NewAddress(
	AddressingMode uint32,
	TxId int,
	RxId int,
	TargetAddress int,
	SourceAddress int,
	PhysicalId int,
	FunctionalId int,
	AddressExtension int,
	RxOnly bool,
	TxOnly bool) *Address {

	var self Address
	self.RxOnly = RxOnly
	self.TxOnly = TxOnly
	self.AddressingMode = AddressingMode
	self.TargetAddress = TargetAddress
	self.SourceAddress = SourceAddress
	self.TxId = TxId
	self.RxId = RxId
	self.AddressExtension = AddressExtension
	if AddressingMode == Normal29bits || AddressingMode == NormalFixed29bits || AddressingMode == Extended29bits || AddressingMode == Mixed29bits {
		self.Is29Bits = true
	} else {
		self.Is29Bits = false
	}
	if AddressingMode == NormalFixed29bits {
		if PhysicalId == 0 {
			self.PhysicalId = 0x18DA0000
		} else {
			self.PhysicalId = PhysicalId & 0x1FFF0000
		}
		if FunctionalId == 0 {
			self.FunctionalId = 0x18DB0000
		} else {
			self.FunctionalId = FunctionalId & 0x1FFF0000
		}
	}
	if self.AddressingMode == Mixed29bits {
		if PhysicalId == 0 {
			self.PhysicalId = 0x18CE0000
		} else {
			self.PhysicalId = PhysicalId & 0x1FFF0000
		}
		if FunctionalId == 0 {
			self.FunctionalId = 0x18CD0000
		} else {
			self.FunctionalId = FunctionalId & 0x1FFF0000
		}
	}
	self.validate()
	self.TxpayloadPrefix = []byte{}
	self.RxprefixSize = 0
	if !self.TxOnly {
		self.RxArbitrationIdPhysical = self.GetRxArbitrationId(Physical)
		self.RxArbitrationIdFunctional = self.GetRxArbitrationId(Functional)
		if self.AddressingMode == Extended11bits ||
			self.AddressingMode == Extended29bits ||
			self.AddressingMode == Mixed11bits ||
			self.AddressingMode == Mixed29bits {
			self.RxprefixSize = 1
		}
	}
	if !self.RxOnly {
		self.TxArbitrationIdPhysical = self.GetTxArbitrationId(Physical)
		self.TxArbitrationIdFunctional = self.GetTxArbitrationId(Functional)
		// if self.AddressingMode == Extended11bits ||
		// 	self.AddressingMode == Extended29bits {
		// 	self.TxpayloadPrefix = []byte{byte(self.TargetAddress)}
		// } else if self.AddressingMode == Mixed11bits ||self.AddressingMode == Mixed29bits {
		// 	self.TxpayloadPrefix = []byte{byte(self.AddressExtension)}
		// }
		switch self.AddressingMode {
		case Extended11bits, Extended29bits:
			self.TxpayloadPrefix = []byte{byte(self.TargetAddress)}
		case Mixed11bits, Mixed29bits:
			self.TxpayloadPrefix = []byte{byte(self.AddressExtension)}
		}
	}
	return &self
}
func (a *Address) validate() {
	if a.RxOnly && a.TxOnly {
		panic("Address cannot be tx only and rx only")
	}

	if !(a.AddressingMode == Normal11bits || a.AddressingMode == Normal29bits || a.AddressingMode == NormalFixed29bits || a.AddressingMode == Extended11bits || a.AddressingMode == Extended29bits || a.AddressingMode == Mixed11bits || a.AddressingMode == Mixed29bits) {
		panic("Addressing mode is not valid")
	}

	switch a.AddressingMode {
	case Normal11bits, Normal29bits:
		if a.RxId == 0 && !a.TxOnly {
			panic("rxid must be specified for Normal addressing mode (11 or 29 bits ID)")
		}
		if a.TxId == 0 && !a.RxOnly {
			panic("txid must be specified for Normal addressing mode (11 or 29 bits ID)")
		}
		if a.RxId == a.TxId {
			panic("txid and rxid must be different for Normal addressing mode")
		}
	case NormalFixed29bits:
		if a.TargetAddress == 0 && a.SourceAddress == 0 {
			// Both zero is a strong signal of missing configuration.
			panic("target_address and source_address must be specified for Normal Fixed addressing (29 bits ID)")
		}
	case Extended11bits, Extended29bits:
		if !a.RxOnly {
			if a.TargetAddress == 0 || a.TxId == 0 {
				panic("target_address and txid must be specified for Extended addressing mode (11 or 29 bits ID)")
			}
		}
		if !a.TxOnly {
			if a.SourceAddress == 0 || a.RxId == 0 {
				panic("source_address and rxid must be specified for Extended addressing mode (11 or 29 bits ID)")
			}
		}
		if a.RxId == a.TxId {
			panic("txid and rxid must be different")
		}
	case Mixed11bits:
		if a.AddressExtension == 0 {
			panic("address_extension must be specified for Mixed addressing mode (11 bits ID)")
		}
		if a.RxId == 0 && !a.TxOnly {
			panic("rxid must be specified for Mixed addressing mode (11 bits ID)")
		}
		if a.TxId == 0 && !a.RxOnly {
			panic("txid must be specified for Mixed addressing mode (11 bits ID)")
		}
		if a.RxId == a.TxId {
			panic("txid and rxid must be different for Mixed addressing mode (11 bits ID)")
		}
	case Mixed29bits:
		if a.TargetAddress == 0 || a.SourceAddress == 0 || a.AddressExtension == 0 {
			panic("target_address, source_address and address_extension must be specified for Mixed addressing mode (29 bits ID)")
		}
	}

	if a.TargetAddress < 0 || a.TargetAddress > 0xFF {
		panic("target_address must be an integer between 0x00 and 0xFF")
	}
	if a.SourceAddress < 0 || a.SourceAddress > 0xFF {
		panic("source_address must be an integer between 0x00 and 0xFF")
	}
	if a.AddressExtension < 0 || a.AddressExtension > 0xFF {
		panic("address_extension must be an integer between 0x00 and 0xFF")
	}
	if a.TxId < 0 {
		panic("txid must be greater than 0")
	}
	if a.RxId < 0 {
		panic("rxid must be greater than 0")
	}
	if !a.Is29Bits {
		if a.TxId > 0x7FF {
			panic("txid must be smaller than 0x7FF for 11 bits identifier")
		}
		if a.RxId > 0x7FF {
			panic("rxid must be smaller than 0x7FF for 11 bits identifier")
		}
	}
}
func (a *Address) GetTxArbitrationId(addressType uint32) int {
	if a.RxOnly {
		panic("tx part disabled")
	}
	return a.getTxArbitrationId(addressType)
}

func (a *Address) GetRxArbitrationId(addressType uint32) int {
	if a.TxOnly {
		panic("rx part disabled")
	}
	return a.getRxArbitrationId(addressType)
}

func (a *Address) RequiresTxExtensionByte() bool {
	if a.RxOnly {
		panic("tx part disabled")
	}
	return a.requiresExtensionByte()
}

func (a *Address) RequiresRxExtensionByte() bool {
	if a.TxOnly {
		panic("rx part disabled")
	}
	return a.requiresExtensionByte()
}

func (a *Address) GetTxExtensionByte() (int, bool) {
	if a.RxOnly {
		panic("tx part disabled")
	}
	if a.AddressingMode == Extended11bits || a.AddressingMode == Extended29bits {
		return a.TargetAddress, true
	}
	if a.AddressingMode == Mixed11bits || a.AddressingMode == Mixed29bits {
		return a.AddressExtension, true
	}
	return 0, false
}

func (a *Address) GetRxExtensionByte() (int, bool) {
	if a.TxOnly {
		panic("rx part disabled")
	}
	if a.AddressingMode == Extended11bits || a.AddressingMode == Extended29bits {
		return a.SourceAddress, true
	}
	if a.AddressingMode == Mixed11bits || a.AddressingMode == Mixed29bits {
		return a.AddressExtension, true
	}
	return 0, false
}

func (a *Address) IsTx29Bit() bool {
	if a.RxOnly {
		panic("tx part disabled")
	}
	return a.Is29Bits
}

func (a *Address) IsRx29Bit() bool {
	if a.TxOnly {
		panic("rx part disabled")
	}
	return a.Is29Bits
}

func (a *Address) IsForMe(msg CanMessage) bool {
	if a.TxOnly {
		panic("rx part disabled")
	}
	switch a.AddressingMode {
	case Normal11bits, Normal29bits:
		return a.isForMeNormal(msg)
	case Extended11bits, Extended29bits:
		return a.isForMeExtended(msg)
	case NormalFixed29bits:
		return a.isForMeNormalFixed(msg)
	case Mixed11bits:
		return a.isForMeMixed11(msg)
	case Mixed29bits:
		return a.isForMeMixed29(msg)
	default:
		panic("unsupported addressing mode")
	}
}

func (a *Address) GetRxPrefixSize() int {
	return a.RxprefixSize
}

func (a *Address) GetTxPayloadPrefix() []byte {
	return a.TxpayloadPrefix
}

func (a *Address) requiresExtensionByte() bool {
	return a.AddressingMode == Extended11bits ||
		a.AddressingMode == Extended29bits ||
		a.AddressingMode == Mixed11bits ||
		a.AddressingMode == Mixed29bits
}

func (a *Address) getTxArbitrationId(addressType uint32) int {
	switch a.AddressingMode {
	case Normal11bits, Normal29bits, Extended11bits, Extended29bits, Mixed11bits:
		return a.TxId
	case Mixed29bits, NormalFixed29bits:
		bits28_16 := a.PhysicalId
		if addressType == Functional {
			bits28_16 = a.FunctionalId
		}
		return bits28_16 | (a.TargetAddress << 8) | a.SourceAddress
	default:
		panic("Unsupported addressing mode")
	}
}

func (a *Address) getRxArbitrationId(addressType uint32) int {
	switch a.AddressingMode {
	case Normal11bits, Normal29bits, Extended11bits, Extended29bits, Mixed11bits:
		return a.RxId
	case Mixed29bits, NormalFixed29bits:
		bits28_16 := a.PhysicalId
		if addressType == Functional {
			bits28_16 = a.FunctionalId
		}
		return bits28_16 | (a.SourceAddress << 8) | a.TargetAddress
	default:
		panic("Unsupported addressing mode")
	}
}

func (a *Address) isForMeNormal(msg CanMessage) bool {
	if a.Is29Bits == msg.ExtendedId {
		return msg.ArbitrationId == a.RxId
	}
	return false
}

func (a *Address) isForMeExtended(msg CanMessage) bool {
	if a.Is29Bits == msg.ExtendedId {
		if len(msg.Data) > 0 {
			return msg.ArbitrationId == a.RxId && int(msg.Data[0]) == a.SourceAddress
		}
	}
	return false
}

func (a *Address) isForMeNormalFixed(msg CanMessage) bool {
	if a.Is29Bits == msg.ExtendedId {
		if (msg.ArbitrationId&0x1FFF0000 == a.PhysicalId) || (msg.ArbitrationId&0x1FFF0000 == a.FunctionalId) {
			if ((msg.ArbitrationId&0xFF00)>>8) == a.SourceAddress && (msg.ArbitrationId&0xFF) == a.TargetAddress {
				return true
			}
		}
	}
	return false
}

func (a *Address) isForMeMixed11(msg CanMessage) bool {
	if a.Is29Bits == msg.ExtendedId {
		if len(msg.Data) > 0 {
			return msg.ArbitrationId == a.RxId && int(msg.Data[0]) == a.AddressExtension
		}
	}
	return false
}

func (a *Address) isForMeMixed29(msg CanMessage) bool {
	if a.Is29Bits == msg.ExtendedId {
		if len(msg.Data) > 0 {
			if (msg.ArbitrationId&0x1FFF0000 == a.PhysicalId) || (msg.ArbitrationId&0x1FFF0000 == a.FunctionalId) {
				if ((msg.ArbitrationId&0xFF00)>>8) == a.SourceAddress && (msg.ArbitrationId&0xFF) == a.TargetAddress && int(msg.Data[0]) == a.AddressExtension {
					return true
				}
			}
		}
	}
	return false
}
