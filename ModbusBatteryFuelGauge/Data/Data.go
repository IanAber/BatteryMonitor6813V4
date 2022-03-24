package Data

import (
	"log"
)

type Data struct {
	SlaveAddress  uint8    `json:"slave"`
	Coil          []bool   `json:"coil"`
	Discrete      []bool   `json:"discrete"`
	Input         []uint16 `json:"input"`
	Holding       []uint16 `json:"holding"`
	LastError     string   `json:"lasterror"`
	coilStart     uint16
	discreteStart uint16
	inputStart    uint16
	holdingStart  uint16
}

func New(Coils uint16, CoilStart uint16, Discretes uint16, DiscreteStart uint16, Inputs uint16, InputsStart uint16, HoldingRegisters uint16, HoldingStart uint16, slave uint8) *Data {
	this := new(Data)
	this.Coil = make([]bool, Coils)
	this.Discrete = make([]bool, Discretes)
	this.Input = make([]uint16, Inputs)
	this.Holding = make([]uint16, HoldingRegisters)
	this.coilStart = CoilStart
	this.discreteStart = DiscreteStart
	this.inputStart = InputsStart
	this.holdingStart = HoldingStart
	this.SlaveAddress = slave
	return this
}

func (this *Data) CoilStart() uint16 {
	return this.coilStart
}

func (this *Data) DiscreteStart() uint16 {
	return this.discreteStart
}

func (this *Data) InputStart() uint16 {
	return this.inputStart
}

func (this *Data) HoldingStart() uint16 {
	return this.holdingStart
}

func (this *Data) GetSpecs() (uint16, uint16, uint16, uint16, uint16, uint16, uint16, uint16, uint8) {
	return uint16(len(this.Coil)), this.coilStart, uint16(len(this.Discrete)), this.discreteStart, uint16(len(this.Input)), this.inputStart, uint16(len(this.Holding)), this.holdingStart, this.SlaveAddress
}

func (this *Data) Compare(p1 *Data) bool {
	if this.SlaveAddress != p1.SlaveAddress {
		return false
	}
	for n := range this.Coil {
		if this.Coil[n] != p1.Coil[n] {
			//			fmt.Println("Coil ", n, " changed from ", p1.Coil[n], " to ", this.Coil[n])
			return false
		}
	}
	for n := range this.Discrete {
		if this.Discrete[n] != p1.Discrete[n] {
			//			fmt.Println("Discrete Input ", n, " changed from ", p1.Discrete[n], " to ", this.Discrete[n])
			return false
		}
	}
	for n := range this.Input {
		if this.Input[n] != p1.Input[n] {
			//			fmt.Println("Input ", n, " changed from ", p1.Input[n], " to ", this.Input[n])
			return false
		}
	}
	for n := range this.Holding {
		if this.Holding[n] != p1.Holding[n] {
			//			fmt.Println("Holding Register ", n, " changed from ", p1.Holding[n], " to ", this.Holding[n])
			return false
		}
	}
	return true
}

func (this *Data) Update(newData *Data) {
	if this.SlaveAddress != newData.SlaveAddress {
		log.Panic("Trying to update data with data from the wrong slave!")
	}
	copy(this.Coil[:], newData.Coil[0:])
	copy(this.Holding[:], newData.Holding[0:])
	copy(this.Input[:], newData.Input[0:])
	copy(this.Discrete[:], newData.Discrete[0:])
}
