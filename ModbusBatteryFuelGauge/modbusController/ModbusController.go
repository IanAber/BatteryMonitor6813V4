package ModbusController

import (
	"encoding/binary"
	"fmt"
	"github.com/goburrow/modbus"
	"log"
	"sync"
	"time"
)

type ModbusController struct {
	rtuClient    *modbus.RTUClientHandler
	modbusClient modbus.Client
	mu           sync.Mutex
}

/**
*  Set up a new ModBus using the parameters given. No attempt is made to connect at this time.
 */
func New(rtuAddress string, baudRate int, dataBits int, stopBits int, parity string, timeout time.Duration) *ModbusController {
	this := new(ModbusController)
	this.rtuClient = modbus.NewRTUClientHandler(rtuAddress)
	this.rtuClient.BaudRate = baudRate
	this.rtuClient.DataBits = dataBits
	this.rtuClient.Timeout = timeout
	this.rtuClient.Parity = parity
	this.rtuClient.StopBits = stopBits
	this.rtuClient.SlaveId = 1

	return this
}

func (this *ModbusController) Close() {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.rtuClient != nil {
		if err := this.rtuClient.Close(); err != nil {
			log.Println(err)
		}
	}
}

func (this *ModbusController) Connect() error {
	this.mu.Lock()
	defer this.mu.Unlock()
	err := this.rtuClient.Connect()
	if err != nil {
		return err
	}
	this.modbusClient = modbus.NewClient(this.rtuClient)
	return nil
}

func (this *ModbusController) readCoil(coil uint16) (bool, error) {
	data, err := this.modbusClient.ReadCoils(coil, 1)
	if err != nil {
		return false, err
	} else {
		if len(data) != 1 {
			return false, fmt.Errorf("Read Coil %d returned %d bytes when 1 was expected.", coil, len(data))
		} else {
			return data[0] != 0, nil
		}
	}
}

func (this *ModbusController) ReadCoil(coil uint16, slaveId uint8) (bool, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	return this.readCoil(coil)
}

func (this *ModbusController) WriteCoil(coil uint16, value bool, slaveId uint8) error {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	var err error
	if value {
		_, err = this.modbusClient.WriteSingleCoil(coil, 0xFF00)
	} else {
		_, err = this.modbusClient.WriteSingleCoil(coil, 0x0000)
	}
	return err
}

func (this *ModbusController) readHoldingRegister(register uint16) (uint16, error) {
	data, err := this.modbusClient.ReadHoldingRegisters(register, 1)
	if err != nil {
		return 0, err
	} else {
		if len(data) != 2 {
			return 0, fmt.Errorf("Read Holding Register %d returned %d bytes when 2 were expected.", register, len(data))
		} else {
			return binary.BigEndian.Uint16(data), nil
		}
	}
}

func (this *ModbusController) ReadHoldingRegister(holdingRegister uint16, slaveId uint8) (uint16, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	return this.readHoldingRegister(holdingRegister)
}

func (this *ModbusController) readHoldingRegisterDiv10(register uint16) (float32, error) {
	data, err := this.modbusClient.ReadHoldingRegisters(register, 1)
	if err != nil {
		return 0, err
	} else {
		if len(data) != 2 {
			return 0, fmt.Errorf("Read Holding Register %d returned %d bytes when 2 were expected.", register, len(data))
		} else {
			return (float32(binary.BigEndian.Uint16(data)) / 10), nil
		}
	}
}

func (this *ModbusController) WriteHoldingRegister(register uint16, value uint16, slaveId uint8) error {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	_, err := this.modbusClient.WriteSingleRegister(register, value)
	return err
}

func (this *ModbusController) writeHoldingRegisterFloat(register uint16, value float32) error {
	_, err := this.modbusClient.WriteSingleRegister(register, uint16(value*10))
	return err
}

func (this *ModbusController) readInputRegister(register uint16) (uint16, error) {
	data, err := this.modbusClient.ReadInputRegisters(register, 1)
	if err != nil {
		fmt.Println("Register = ", register, " Error = ", err)
		return 0, err
	} else {
		if len(data) != 2 {
			return 0, fmt.Errorf("Read Input Register %d returned %d bytes when 2 were expected.", register, len(data))
		} else {
			return binary.BigEndian.Uint16(data), nil
		}
	}
}

func (this *ModbusController) ReadInputRegister(holdingRegister uint16, slaveId uint8) (uint16, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	return this.readInputRegister(holdingRegister)
}

func (this *ModbusController) readDiscreteInput(input uint16) (bool, error) {
	data, err := this.modbusClient.ReadDiscreteInputs(input, 1)
	if err != nil {
		return false, err
	} else {
		if len(data) != 1 {
			return false, fmt.Errorf("Read Discrete Input %d returned %d bytes when 1 was expected.", input, len(data))
		} else {
			return data[0] != 0, nil
		}
	}
}

func convertBitsToBools(byteData []byte, length uint16) []bool {
	boolData := make([]bool, length)
	for i, b := range byteData {
		for bit := 0; bit < 8; bit++ {
			boolIndex := uint16((i * 8) + bit)
			if boolIndex < length {
				boolData[(i*8)+bit] = ((b & 1) != 0)
			}
			b >>= 1
		}
	}
	return boolData
}

func (this *ModbusController) ReadMultipleDiscreteRegisters(start uint16, count uint16, slaveId uint8) ([]bool, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	mbData, err := this.modbusClient.ReadDiscreteInputs(start, count)
	if err != nil {
		return make([]bool, count), err
	} else {
		return convertBitsToBools(mbData, count), err
	}
}

func (this *ModbusController) ReadMultipleCoils(start uint16, count uint16, slaveId uint8) ([]bool, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	mbData, err := this.modbusClient.ReadCoils(start, count)
	if err != nil {
		return make([]bool, count), err
	} else {
		return convertBitsToBools(mbData, count), err
	}
}

func convertBytesToWords(byteData []byte) []uint16 {
	wordData := make([]uint16, len(byteData)/2)
	for i := range wordData {
		wordData[i] = binary.BigEndian.Uint16(byteData[i*2:])
	}
	return wordData
}

func (this *ModbusController) ReadMultipleInputRegisters(start uint16, count uint16, slaveId uint8) ([]uint16, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	mbData, err := this.modbusClient.ReadInputRegisters(start, count)
	return convertBytesToWords(mbData), err
}

func (this *ModbusController) ReadMultipleHoldingRegisters(start uint16, count uint16, slaveId uint8) ([]uint16, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	mbData, err := this.modbusClient.ReadHoldingRegisters(start, count)
	return convertBytesToWords(mbData), err
}

func (this *ModbusController) ReadDiscreteInput(input uint16, slaveId uint8) (bool, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.rtuClient.SlaveId = slaveId
	return this.readCoil(input)
}
