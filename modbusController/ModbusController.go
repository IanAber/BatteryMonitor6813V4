package ModbusController

import (
	"encoding/binary"
	"fmt"
	"github.com/goburrow/modbus"
	"log"
	"sync"
)

type ModbusController struct {
	rtuClient    *modbus.RTUClientHandler
	modbusClient modbus.Client
	mu           sync.Mutex
}

/**
*  Set up a new ModBus using the parameters given. No attempt is made to connect at this time.
 */
//func New(rtuAddress string, baudRate int, dataBits int, stopBits int, parity string, timeout time.Duration) *ModbusController {
//	this := new(ModbusController)
//	this.rtuClient = modbus.NewRTUClientHandler(rtuAddress)
//	this.rtuClient.BaudRate = baudRate
//	this.rtuClient.DataBits = dataBits
//	this.rtuClient.Timeout = timeout
//	this.rtuClient.Parity = parity
//	this.rtuClient.StopBits = stopBits
//	this.rtuClient.SlaveId = 1
//
//	return this
//}

func (modbusController *ModbusController) Close() {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	if modbusController.rtuClient != nil {
		closeErr := modbusController.rtuClient.Close()
		if closeErr != nil {
			log.Println(closeErr)
		}
	}
}

func (modbusController *ModbusController) Connect() error {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	err := modbusController.rtuClient.Connect()
	if err != nil {
		return err
	}
	modbusController.modbusClient = modbus.NewClient(modbusController.rtuClient)
	return nil
}

func (modbusController *ModbusController) readCoil(coil uint16) (bool, error) {
	data, err := modbusController.modbusClient.ReadCoils(coil, 1)
	if err != nil {
		return false, err
	} else {
		if len(data) != 1 {
			return false, fmt.Errorf("read coil %d returned %d bytes when 1 was expected", coil, len(data))
		} else {
			return data[0] != 0, nil
		}
	}
}

func (modbusController *ModbusController) ReadCoil(coil uint16, slaveId uint8) (bool, error) {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	return modbusController.readCoil(coil)
}

func (modbusController *ModbusController) WriteCoil(coil uint16, value bool, slaveId uint8) error {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	var err error
	if value {
		_, err = modbusController.modbusClient.WriteSingleCoil(coil, 0xFF00)
	} else {
		_, err = modbusController.modbusClient.WriteSingleCoil(coil, 0x0000)
	}
	return err
}

func (modbusController *ModbusController) readHoldingRegister(register uint16) (uint16, error) {
	data, err := modbusController.modbusClient.ReadHoldingRegisters(register, 1)
	if err != nil {
		return 0, err
	} else {
		if len(data) != 2 {
			return 0, fmt.Errorf("read holding register %d returned %d bytes when 2 were expected", register, len(data))
		} else {
			return binary.BigEndian.Uint16(data), nil
		}
	}
}

func (modbusController *ModbusController) ReadHoldingRegister(holdingRegister uint16, slaveId uint8) (uint16, error) {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	return modbusController.readHoldingRegister(holdingRegister)
}

func (modbusController *ModbusController) readHoldingRegisterDiv10(register uint16) (float32, error) {
	data, err := modbusController.modbusClient.ReadHoldingRegisters(register, 1)
	if err != nil {
		return 0, err
	} else {
		if len(data) != 2 {
			return 0, fmt.Errorf("read holding register %d returned %d bytes when 2 were expected", register, len(data))
		} else {
			return float32(binary.BigEndian.Uint16(data)) / 10, nil
		}
	}
}

func (modbusController *ModbusController) WriteHoldingRegister(register uint16, value uint16, slaveId uint8) error {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	_, err := modbusController.modbusClient.WriteSingleRegister(register, value)
	return err
}

func (modbusController *ModbusController) writeHoldingRegisterFloat(register uint16, value float32) error {
	_, err := modbusController.modbusClient.WriteSingleRegister(register, uint16(value*10))
	return err
}

func (modbusController *ModbusController) readInputRegister(register uint16) (uint16, error) {
	data, err := modbusController.modbusClient.ReadInputRegisters(register, 1)
	if err != nil {
		fmt.Println("Register = ", register, " Error = ", err)
		return 0, err
	} else {
		if len(data) != 2 {
			return 0, fmt.Errorf("read input register %d returned %d bytes when 2 were expected", register, len(data))
		} else {
			return binary.BigEndian.Uint16(data), nil
		}
	}
}

func (modbusController *ModbusController) ReadInputRegister(holdingRegister uint16, slaveId uint8) (uint16, error) {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	return modbusController.readInputRegister(holdingRegister)
}

func (modbusController *ModbusController) readDiscreteInput(input uint16) (bool, error) {
	data, err := modbusController.modbusClient.ReadDiscreteInputs(input, 1)
	if err != nil {
		return false, err
	} else {
		if len(data) != 1 {
			return false, fmt.Errorf("read discrete input %d returned %d bytes when 1 was expected", input, len(data))
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
				boolData[(i*8)+bit] = (b & 1) != 0
			}
			b >>= 1
		}
	}
	return boolData
}

func (modbusController *ModbusController) ReadMultipleDiscreteRegisters(start uint16, count uint16, slaveId uint8) ([]bool, error) {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	fmt.Println("Reading ", count, " input registers starting at ", start)
	mbData, err := modbusController.modbusClient.ReadDiscreteInputs(start, count)
	if err != nil {
		return make([]bool, count), err
	} else {
		return convertBitsToBools(mbData, count), err
	}
}

func (modbusController *ModbusController) ReadMultipleCoils(start uint16, count uint16, slaveId uint8) ([]bool, error) {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	mbData, err := modbusController.modbusClient.ReadCoils(start, count)
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

func (modbusController *ModbusController) ReadMultipleInputRegisters(start uint16, count uint16, slaveId uint8) ([]uint16, error) {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	mbData, err := modbusController.modbusClient.ReadInputRegisters(start, count)
	return convertBytesToWords(mbData), err
}

func (modbusController *ModbusController) ReadMultipleHoldingRegisters(start uint16, count uint16, slaveId uint8) ([]uint16, error) {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	mbData, err := modbusController.modbusClient.ReadHoldingRegisters(start, count)
	return convertBytesToWords(mbData), err
}

func (modbusController *ModbusController) ReadDiscreteInput(input uint16, slaveId uint8) (bool, error) {
	modbusController.mu.Lock()
	defer modbusController.mu.Unlock()
	modbusController.rtuClient.SlaveId = slaveId
	return modbusController.readCoil(input)
}
