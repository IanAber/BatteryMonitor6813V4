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

//func New(Coils uint16, CoilStart uint16, Discretes uint16, DiscreteStart uint16, Inputs uint16, InputsStart uint16, HoldingRegisters uint16, HoldingStart uint16, slave uint8) *Data {
//	this := new(Data)
//	this.Coil = make([]bool, Coils)
//	this.Discrete = make([]bool, Discretes)
//	this.Input = make([]uint16, Inputs)
//	this.Holding = make([]uint16, HoldingRegisters)
//	this.coilStart = CoilStart
//	this.discreteStart = DiscreteStart
//	this.inputStart = InputsStart
//	this.holdingStart = HoldingStart
//	this.SlaveAddress = slave
//	return this
//}

func (data *Data) CoilStart() uint16 {
	return data.coilStart
}

func (data *Data) DiscreteStart() uint16 {
	return data.discreteStart
}

func (data *Data) InputStart() uint16 {
	return data.inputStart
}

func (data *Data) HoldingStart() uint16 {
	return data.holdingStart
}

func (data *Data) GetSpecs() (uint16, uint16, uint16, uint16, uint16, uint16, uint16, uint16, uint8) {
	return uint16(len(data.Coil)), data.coilStart, uint16(len(data.Discrete)), data.discreteStart, uint16(len(data.Input)), data.inputStart, uint16(len(data.Holding)), data.holdingStart, data.SlaveAddress
}

func (data *Data) Compare(p1 *Data) bool {
	if data.SlaveAddress != p1.SlaveAddress {
		return false
	}
	for n := range data.Coil {
		if data.Coil[n] != p1.Coil[n] {
			//			fmt.Println("Coil ", n, " changed from ", p1.Coil[n], " to ", data.Coil[n])
			return false
		}
	}
	for n := range data.Discrete {
		if data.Discrete[n] != p1.Discrete[n] {
			//			fmt.Println("Discrete Input ", n, " changed from ", p1.Discrete[n], " to ", data.Discrete[n])
			return false
		}
	}
	for n := range data.Input {
		if data.Input[n] != p1.Input[n] {
			//			fmt.Println("Input ", n, " changed from ", p1.Input[n], " to ", data.Input[n])
			return false
		}
	}
	for n := range data.Holding {
		if data.Holding[n] != p1.Holding[n] {
			//			fmt.Println("Holding Register ", n, " changed from ", p1.Holding[n], " to ", data.Holding[n])
			return false
		}
	}
	return true
}

func (data *Data) Update(newData *Data) {
	if data.SlaveAddress != newData.SlaveAddress {
		log.Panic("Trying to update data with data from the wrong slave!")
	}
	copy(data.Coil[:], newData.Coil[0:])
	copy(data.Holding[:], newData.Holding[0:])
	copy(data.Input[:], newData.Input[0:])
	copy(data.Discrete[:], newData.Discrete[0:])
}
