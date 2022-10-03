package FuelGauge

import (
	"BatteryMonitor6813V4/ModbusBatteryFuelGauge/Data"
	ModbusController "BatteryMonitor6813V4/ModbusBatteryFuelGauge/modbusController"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type fuelGaugeChannel struct {
	Coulombs       float32
	Efficiency     float64
	ModbusData     *Data.Data
	SlaveAddress   uint8
	LastUpdate     time.Time
	Capacity       int16
	LastFullCharge sql.NullTime
}

//type batterySettings struct {
//	CapacityLeft        float32
//	CapacityRight       float32
//	LastFullChargeLeft  mysql.NullTime
//	LastFullChargeRight mysql.NullTime
//}

type FuelGauge struct {
	mbus                 *ModbusController.ModbusController
	FgLeft               fuelGaugeChannel
	FgRight              fuelGaugeChannel
	currentStatement     *sql.Stmt
	systemParamStatement *sql.Stmt
	fullChargeStatement  *sql.Stmt
	pDB                  *sql.DB
	baudRate             int
	commsPort            string
	reportTicker         *time.Ticker
}

/*
var lastCoulombCount struct{
	count_0 uint16
	count_1 uint16
	efficiency_0 float64
	efficiency_1 float64
	when time.Time
}
*/
type ModbusEndPoint struct {
	id         string
	name       string
	address    uint16
	dataType   byte
	multiplier int
	units      string
	writeable  bool
	signed     bool
}

type CoilRequest struct {
	Slave uint8  `json:"slave"`
	Id    uint16 `json:"id"`
	Value bool   `json:"value"`
}

type RegisterRequest struct {
	Slave uint8  `json:"slave"`
	Id    uint16 `json:"id"`
	Value int16  `json:"value"`
}

const WATERCHARGETHRESHOLD = 98.0 // The state of charge point reached at which the watering system is turned on.
const LeftBank = 0
const RightBank = 1

//const LeftBankCellCount = 37
//const RightBankCellCount = 38

// Digital Inputs
const LeftBankSense = 0
const RightBankSense = 1

// Coils
const LeftBankOnRelay = 1
const LeftBankOffRelay = 2
const RightBankOnRelay = 3
const RightBankOffRelay = 4
const GeneratorRelay = 5
const BatteryFanRelay = 8
const LeftWaterRelay = 7
const RightWaterRelay = 8

// Input Registers
const CurrentRegister = 1
const Analogue0 = 2
const Analogue1 = 3
const Analogue2 = 4
const Analogue3 = 5
const Analogue6 = 6
const Analogue7 = 7
const AvgCurrent = 8
const RawCurrent = 9
const UptimeLow = 10
const UptimeHigh = 11

// Holding Registers
const SlaveIdRegister = 1
const BaudRateRegister = 2
const OffsetRegister = 3
const PgaGainRegister = 4
const SamplesPerSecReg = 5
const ChargeRegister = 6
const CurrentGainReg = 7
const EfficiencyReg = 8

// Coils
const Relay1Coil = 1
const Relay2Coil = 2
const Relay3Coil = 3
const Relay4Coil = 4
const Relay5Coil = 5
const Relay6Coil = 6
const Relay7Coil = 7
const Relay8Coil = 8
const Momentary1Coil = 9
const Momentary2Coil = 10
const Momentary3Coil = 11
const Momentary4Coil = 12
const Momentary5Coil = 13
const Momentary6Coil = 14
const Momentary7Coil = 15
const Momentary8Coil = 16

// Discrete Inputs
const Discrete1 = 1
const Discrete2 = 2
const Discrete3 = 3
const Discrete4 = 4
const Discrete5 = 5
const Discrete6 = 6
const Discrete7 = 7
const Discrete8 = 8
const I2cFailure = 9

var EndPointsLeft = []ModbusEndPoint{
	{"1", "Current", CurrentRegister, InputRegister, 100, "A", false, true},
	{"2", "Analog In 0", Analogue0, InputRegister, 1, "", false, false},
	{"3", "Analog In 1", Analogue1, InputRegister, 1, "", false, false},
	{"4", "Analog In 2", Analogue2, InputRegister, 1, "", false, false},
	{"5", "Analog In 3", Analogue3, InputRegister, 1, "", false, false},
	{"6", "Analog In 6", Analogue6, InputRegister, 1, "", false, true},
	{"7", "Analog In 7", Analogue7, InputRegister, 1, "", false, true},
	{"8", "Avg Current", AvgCurrent, InputRegister, 100, "A", false, true},
	{"9", "Raw Current", RawCurrent, InputRegister, 1, "", false, true},
	{"x1", "", 0, Blank, 0, "", false, false},
	{"10", "Up Time Low", UptimeLow, InputRegister, 1, "", false, false},
	{"11", "Up Time High", UptimeHigh, InputRegister, 1, "", false, false},

	{"1", "Slave ID", SlaveIdRegister, HoldingRegister, 1, "", true, false},
	{"2", "Baud Rate", BaudRateRegister, HoldingRegister, 1, "", true, false},
	{"3", "Offset", OffsetRegister, HoldingRegister, 1, "", true, true},
	{"4", "PGA Gain", PgaGainRegister, HoldingRegister, 1, "", true, false},
	{"5", "Samples Per Sec", SamplesPerSecReg, HoldingRegister, 1, "", true, false},
	{"6", "Charge", ChargeRegister, HoldingRegister, 10, "Ahr", true, true},
	{"7", "Current Gain", CurrentGainReg, HoldingRegister, 1, "", true, true},
	{"8", "Charge Efficiency", EfficiencyReg, HoldingRegister, 2, "%", true, true},

	{"1", "Left On", Relay1Coil, Coil, 1, "", true, false},
	{"2", "Left Off", Relay2Coil, Coil, 1, "", true, false},
	{"3", "Right On", Relay3Coil, Coil, 1, "", true, false},
	{"4", "Right Off", Relay4Coil, Coil, 1, "", true, false},
	{"5", "Generator", Relay5Coil, Coil, 1, "", true, false},
	{"6", "Relay 6", Relay6Coil, Coil, 1, "", true, false},
	{"7", "Relay 7", Relay7Coil, Coil, 1, "", true, false},
	{"8", "Fan", Relay8Coil, Coil, 1, "", true, false},
	{"9", "Momentary 1", Momentary1Coil, Coil, 1, "", true, false},
	{"10", "Momentary 2", Momentary2Coil, Coil, 1, "", true, false},
	{"11", "Momentary 3", Momentary3Coil, Coil, 1, "", true, false},
	{"12", "Momentary 4", Momentary4Coil, Coil, 1, "", true, false},
	{"13", "Momentary 5", Momentary5Coil, Coil, 1, "", true, false},
	{"14", "Momentary 6", Momentary6Coil, Coil, 1, "", true, false},
	{"15", "Momentary 7", Momentary7Coil, Coil, 1, "", true, false},
	{"16", "Momentary 8", Momentary8Coil, Coil, 1, "", true, false},

	{"1", "Digital In 1", Discrete1, Discrete, 1, "", false, false},
	{"2", "Digital In 2", Discrete2, Discrete, 1, "", false, false},
	{"3", "Digital In 3", Discrete3, Discrete, 1, "", false, false},
	{"4", "Digital In 4", Discrete4, Discrete, 1, "", false, false},
	{"5", "Digital In 5", Discrete5, Discrete, 1, "", false, false},
	{"6", "Digital In 6", Discrete6, Discrete, 1, "", false, false},
	{"7", "Digital In 7", Discrete7, Discrete, 1, "", false, false},
	{"8", "Digital In 8", Discrete8, Discrete, 1, "", false, false},
	{"9", "I2C Failure", I2cFailure, Discrete, 1, "", false, false},
}

var EndPointsRight = []ModbusEndPoint{
	{"1", "Current", CurrentRegister, InputRegister, 100, "A", false, true},
	{"2", "Analog In 0", Analogue0, InputRegister, 1, "", false, false},
	{"3", "Analog In 1", Analogue1, InputRegister, 1, "", false, false},
	{"4", "Analog In 2", Analogue2, InputRegister, 1, "", false, false},
	{"5", "Analog In 3", Analogue3, InputRegister, 1, "", false, false},
	{"6", "Analog In 6", Analogue6, InputRegister, 1, "", false, true},
	{"7", "Analog In 7", Analogue7, InputRegister, 1, "", false, true},
	{"8", "Avg Current", AvgCurrent, InputRegister, 100, "A", false, true},
	{"9", "Raw Current", RawCurrent, InputRegister, 1, "", false, true},
	{"x1", "", 0, Blank, 0, "", false, false},
	{"10", "Up Time Low", UptimeLow, InputRegister, 1, "", false, false},
	{"11", "Up Time High", UptimeHigh, InputRegister, 1, "", false, false},

	{"1", "Slave ID", SlaveIdRegister, HoldingRegister, 1, "", true, false},
	{"2", "Baud Rate", BaudRateRegister, HoldingRegister, 1, "", true, false},
	{"3", "Offset", OffsetRegister, HoldingRegister, 1, "", true, true},
	{"4", "PGA Gain", PgaGainRegister, HoldingRegister, 1, "", true, false},
	{"5", "Samples Per Sec", SamplesPerSecReg, HoldingRegister, 1, "", true, false},
	{"6", "Charge", ChargeRegister, HoldingRegister, 10, "Ahr", true, true},
	{"7", "Current Gain", CurrentGainReg, HoldingRegister, 1, "", true, true},
	{"8", "Charge Efficiency", EfficiencyReg, HoldingRegister, 2, "%", true, true},

	{"1", "Relay 1", Relay1Coil, Coil, 1, "", true, false},
	{"2", "Relay 2", Relay2Coil, Coil, 1, "", true, false},
	{"3", "Relay 3", Relay3Coil, Coil, 1, "", true, false},
	{"4", "Relay 4", Relay4Coil, Coil, 1, "", true, false},
	{"5", "Relay 5", Relay5Coil, Coil, 1, "", true, false},
	{"6", "Relay 6", Relay6Coil, Coil, 1, "", true, false},
	{"7", "Left Water", Relay7Coil, Coil, 1, "", true, false},
	{"8", "Right Water", Relay8Coil, Coil, 1, "", true, false},
	{"9", "Momentary 1", Momentary1Coil, Coil, 1, "", true, false},
	{"10", "Momentary 2", Momentary2Coil, Coil, 1, "", true, false},
	{"11", "Momentary 3", Momentary3Coil, Coil, 1, "", true, false},
	{"12", "Momentary 4", Momentary4Coil, Coil, 1, "", true, false},
	{"13", "Momentary 5", Momentary5Coil, Coil, 1, "", true, false},
	{"14", "Momentary 6", Momentary6Coil, Coil, 1, "", true, false},
	{"15", "Momentary 7", Momentary7Coil, Coil, 1, "", true, false},
	{"16", "Momentary 8", Momentary8Coil, Coil, 1, "", true, false},

	{"1", "Digital In 1", Discrete1, Discrete, 1, "", false, false},
	{"2", "Digital In 2", Discrete2, Discrete, 1, "", false, false},
	{"3", "Digital In 3", Discrete3, Discrete, 1, "", false, false},
	{"4", "Digital In 4", Discrete4, Discrete, 1, "", false, false},
	{"5", "Digital In 5", Discrete5, Discrete, 1, "", false, false},
	{"6", "Digital In 6", Discrete6, Discrete, 1, "", false, false},
	{"7", "Digital In 7", Discrete7, Discrete, 1, "", false, false},
	{"8", "Digital In 8", Discrete8, Discrete, 1, "", false, false},
	{"9", "I2C Failure", I2cFailure, Discrete, 1, "", false, false},
}

const (
	Coil = iota
	Discrete
	InputRegister
	HoldingRegister
	Blank // Allows blank entries to be placed
)

/**
Change the state of a coil (relay)
*/
func (fuelgauge *FuelGauge) WebToggleCoil(w http.ResponseWriter, r *http.Request) {
	address := r.FormValue("coil")
	var dataPointer *Data.Data

	slaveVal, _ := strconv.ParseUint(r.FormValue("slave"), 10, 16)
	if uint8(slaveVal) == (fuelgauge.FgLeft.SlaveAddress) {
		dataPointer = fuelgauge.FgLeft.ModbusData
	} else {
		dataPointer = fuelgauge.FgRight.ModbusData
	}
	// coil
	n, _ := strconv.ParseUint(address, 10, 16)

	nIndex := uint16(n)
	nIndex = nIndex - dataPointer.CoilStart() // nIndex is now 0 based
	err := fuelgauge.mbus.WriteCoil(nIndex+dataPointer.CoilStart(), !dataPointer.Coil[nIndex], dataPointer.SlaveAddress)
	if err != nil {
		dataPointer.LastError = err.Error()
	}
	w.Header().Set("Cache-Control", "no-store")
	_, err = fmt.Fprint(w, "Coil ", nIndex+dataPointer.CoilStart(), " on slave ", dataPointer.SlaveAddress, " toggled.")
	if err != nil {
		log.Println(err)
	}
	//	getValues(true, dataPointer, fuelgauge.mbus)
}

/**
Process form POST commands from the fuel gauge controller forms
*/
func (fuelgauge *FuelGauge) WebProcessHoldingRegistersForm(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	var dataPointer *Data.Data
	//	var holdingValues []uint16
	//	var i int

	slaveVal, _ := strconv.ParseUint(r.FormValue("slave"), 10, 16)
	if uint8(slaveVal) == (fuelgauge.FgLeft.SlaveAddress) {
		dataPointer = fuelgauge.FgLeft.ModbusData
	} else {
		dataPointer = fuelgauge.FgRight.ModbusData
	}
	if err != nil {
		_, err = fmt.Fprint(w, `<html><head><title>Error</title></head><body><h1>`, err, `</h1></body></html>`)
		if err != nil {
			log.Println(err)
		}
	}
	for sKey, sValue := range r.Form {
		nValue, _ := strconv.ParseFloat(sValue[0], 32)

		for _, ep := range EndPointsLeft {
			if (ep.id == sKey) && (ep.dataType == HoldingRegister) {
				//				log.Println("Holding ", ep.id, " set to ", nValue)
				err = fuelgauge.mbus.WriteHoldingRegister(ep.address, uint16(nValue), dataPointer.SlaveAddress)
				if err != nil {
					log.Println(err)
					dataPointer.LastError = err.Error()
				}
			}
		}
		if err != nil {
			log.Println("Error writing to modbus holding register - ", err)
		}
	}
}

/**
Get the values from the relevant fuel gauge controller
*/
func (fuelgauge *FuelGauge) getValues(lastValues *Data.Data, p *ModbusController.ModbusController) {
	slaveID := lastValues.SlaveAddress
	newValues := Data.New(lastValues.GetSpecs())
	if len(newValues.Discrete) > 0 {
		mbData, err := p.ReadMultipleDiscreteRegisters(newValues.DiscreteStart(), uint16(len(newValues.Discrete)), newValues.SlaveAddress)
		if err != nil {
			log.Println("Error getting discrete inputs from slave ID ", slaveID, " - ", err)
			lastValues.LastError = err.Error()
			copy(newValues.Coil[:], lastValues.Coil)
		} else {
			copy(newValues.Discrete[:], mbData)
		}
	}
	if len(newValues.Coil) > 0 {
		mbData, err := p.ReadMultipleCoils(newValues.CoilStart(), uint16(len(newValues.Coil)), newValues.SlaveAddress)
		if err != nil {
			log.Println("Error getting coils from slave ID ", slaveID, " - ", err)
			lastValues.LastError = err.Error()
			copy(newValues.Discrete[:], lastValues.Discrete)
		} else {
			copy(newValues.Coil[:], mbData)
		}
	}
	if len(newValues.Holding) > 0 {
		mbUintData, err := p.ReadMultipleHoldingRegisters(newValues.HoldingStart(), uint16(len(newValues.Holding)), newValues.SlaveAddress)
		if err != nil {
			log.Println("Error getting holding registers from slave ID ", slaveID, " - ", err)
			lastValues.LastError = err.Error()
			copy(newValues.Holding[:], lastValues.Holding)
		} else {
			//			mbUintData := make([]uint16, len(mbUintData))
			//			for i, v := range mbUintData {
			//				mbUintData[i] = uint16(v)
			//			}
			copy(newValues.Holding[:], mbUintData)
			if (newValues.Holding[5] < 100) && (lastValues.Holding[5] > 300) && (newValues.SlaveAddress == 5) {
				newValues.Holding[5] = lastValues.Holding[5]
				err := fuelgauge.mbus.WriteHoldingRegister(5, lastValues.Holding[5], lastValues.SlaveAddress)
				if err != nil {
					log.Println("Error correcting charge state", err)
				} else {
					log.Printf("Charge value corrected from %d to %d", mbUintData[5], newValues.Holding[5])
				}
			}
		}
	}
	if len(newValues.Input) > 0 {
		mbUintData, err := p.ReadMultipleInputRegisters(newValues.InputStart(), uint16(len(newValues.Input)), newValues.SlaveAddress)
		if err != nil {
			log.Println("Error getting input registers from slave ID ", slaveID, " - ", err)
			lastValues.LastError = err.Error()
			copy(newValues.Input[:], lastValues.Input)
		} else {
			copy(newValues.Input[:], mbUintData)
		}
	}
	lastValues.Update(newValues)
}

/**
Set the charge level for the selected channel
*/
func (fuelgauge *FuelGauge) SetCharge(slave uint8, charge float32) {
	err := fuelgauge.mbus.WriteHoldingRegister(5, uint16(charge), slave)
	if err != nil {
		log.Println(err)
	}
}

/**
Log the data read into the database
*/
func (fuelgauge *FuelGauge) logValues(current0 uint16, current1 uint16, charge0 float32, charge1 float32) {
	i0 := int16(current0)
	i1 := int16(current1)
	c0 := charge0
	c1 := charge1

	// Trim to capacity
	if c0 > float32(fuelgauge.FgLeft.Capacity) {
		c0 = float32(fuelgauge.FgLeft.Capacity)
	} else if c0 < 0.1 {
		c0 = 0.1
	}

	if c1 > float32(fuelgauge.FgRight.Capacity) {
		c1 = float32(fuelgauge.FgRight.Capacity)
	} else if c1 < 0.1 {
		c1 = 0.1
	}

	_, err := fuelgauge.currentStatement.Exec(float32(i0)/100.0, float32(i1)/100.0, c0, c1)
	if err != nil {
		log.Println(err)
	}

	delta := int(charge0) - int(fuelgauge.FgLeft.Coulombs)
	// Take care of overflow of the coulomb counter
	if delta > 65000 {
		delta = delta - 65536
	} else if delta < -65000 {
		delta = delta + 65536
	}
	if delta > 0 {
		_, err = fuelgauge.systemParamStatement.Exec(float64(delta)*fuelgauge.FgLeft.Efficiency, "charge_in_counter_0")
		if err != nil {
			log.Println("Adding charge to bank 0 - ", err)
		}
	} else if delta < 0 {
		_, err = fuelgauge.systemParamStatement.Exec(0-delta, "charge_out_counter_0")
		if err != nil {
			log.Println("Reducing charge to bank 0 - ", err)
		}
		if charge0 > float32(fuelgauge.FgLeft.Capacity) {
			fuelgauge.SetCharge(fuelgauge.FgLeft.SlaveAddress, c0)
		}
	}
	if charge0 != c0 {
		// Update the controller because we needed to correct the charge level
		err := fuelgauge.mbus.WriteHoldingRegister(ChargeRegister, uint16(c0*10.0), fuelgauge.FgLeft.SlaveAddress)
		if err != nil {
			log.Println(err)
		}
		charge0 = c0
	}
	fuelgauge.FgLeft.Coulombs = charge0
	delta = int(charge1) - int(fuelgauge.FgRight.Coulombs)
	if delta > 0 {
		_, err = fuelgauge.systemParamStatement.Exec(float64(delta)*fuelgauge.FgRight.Efficiency, "charge_in_counter_1")
		if err != nil {
			log.Println("Adding charge to bank 1 - ", err)
		}
	} else if delta < 0 {
		_, err = fuelgauge.systemParamStatement.Exec(0-delta, "charge_out_counter_1")
		if err != nil {
			log.Println("Reducing charge to bank 1 - ", err)
		}
		if charge1 > float32(fuelgauge.FgRight.Capacity) {
			fuelgauge.SetCharge(fuelgauge.FgRight.SlaveAddress, c1)
		}
	}
	if charge1 != c1 {
		// Update the controller because we needed to correct the charge level
		err = fuelgauge.mbus.WriteHoldingRegister(ChargeRegister, uint16(c1*10.0), fuelgauge.FgRight.SlaveAddress)
		if err != nil {
			log.Println(err)
		}
		charge1 = c1
	}
	fuelgauge.FgRight.Coulombs = charge1
}

/**
Check that at least one battery is connected
*/
func (fuelgauge *FuelGauge) CheckBatteryConnectionState() {
	if fuelgauge.FgLeft.ModbusData.Discrete[LeftBankSense] && fuelgauge.FgLeft.ModbusData.Discrete[RightBankSense] {
		// Both batteries appear to be switched off so we need to switch the left bank on.
		fuelgauge.PulseRelay(LeftBankOnRelay, fuelgauge.FgLeft.SlaveAddress, 2)
		log.Println("Turning on the left bank because both banks reported OFF")
		// This should neve have happened so we should send an email warning that we had to do it.
		err := smtp.SendMail("mail.cedartechnology.com:587",
			smtp.PlainAuth("", "pi@cedartechnology.com", "7444561", "mail.cedartechnology.com"),
			"pi@cedartechnology.com", []string{"ian.abercrombie@cedartechnology.com"}, []byte(`From: Battery
To: Ian.Abercrombie@CedarTechnology.com
Subject: Battery Correction
Turned on the left bank because both banks were reporting OFF`))
		if err != nil {
			log.Println("Failed to send email about having to turn on the left bank because both banks were off. - ", err)
		}
	}
}

/**
Read the fuel gauge values every second and update the database
*/
func (fuelgauge *FuelGauge) Run() {
	reportTicker := time.NewTicker(time.Second)
	defer reportTicker.Stop()
	for {
		select {
		case <-reportTicker.C:
			{
				fuelgauge.FgLeft.ModbusData.LastError = ""
				fuelgauge.getValues(fuelgauge.FgLeft.ModbusData, fuelgauge.mbus)

				fuelgauge.FgRight.ModbusData.LastError = ""
				fuelgauge.getValues(fuelgauge.FgRight.ModbusData, fuelgauge.mbus)

				// Fudge the current and charge because the sensor isn't working
				fuelgauge.FgRight.ModbusData.Input[0] = 0
				fuelgauge.FgRight.ModbusData.Input[7] = 0
				fuelgauge.FgRight.ModbusData.Holding[5] = 1000

				fuelgauge.CheckBatteryConnectionState()
				//				fuelgauge.logValues(fuelgauge.FgLeft.ModbusData.Input[7], fuelgauge.FgRight.ModbusData.Input[7], float32(fuelgauge.FgLeft.ModbusData.Holding[5])/10, float32(fuelgauge.FgRight.ModbusData.Holding[5])/10)
				fuelgauge.logValues(fuelgauge.FgLeft.ModbusData.Input[7], 100, float32(fuelgauge.FgLeft.ModbusData.Holding[5])/10, 0)
			}
		}
	}
}

/**
Read the capacity and last full charge datetime vaules from the database
*/
func (fuelgauge *FuelGauge) ReadSystemParameters() {
	row := fuelgauge.pDB.QueryRow(`select date_value from system_parameters where name = 'bank0_full'`)
	err := row.Scan(&fuelgauge.FgLeft.LastFullCharge)
	if err != nil {
		log.Println("Error getting last full charge left from system parameters - ", err)
	}
	row = fuelgauge.pDB.QueryRow(`select date_value from system_parameters where name = 'bank1_full'`)
	err = row.Scan(&fuelgauge.FgRight.LastFullCharge)
	if err != nil {
		log.Println("Error getting last full charge right from system parameters - ", err)
	}
	row = fuelgauge.pDB.QueryRow(`select integer_value from system_parameters where name = 'capacity_0'`)
	err = row.Scan(&fuelgauge.FgLeft.Capacity)
	if err != nil {
		log.Println("Error getting left bank capacity from system parameters - ", err)
	}
	row = fuelgauge.pDB.QueryRow(`select integer_value from system_parameters where name = 'capacity_1'`)
	err = row.Scan(&fuelgauge.FgRight.Capacity)
	if err != nil {
		log.Println("Error getting right bank capacity from system parameters - ", err)
	}
}

func (fuelgauge *FuelGauge) setFullCharge(bank int) {
	var sSQL string
	if bank == 0 {
		sSQL = `update system_parameters set integer_value = 1, date_value = now() where name = 'bank0_full' and integer_value = 0`
	} else {
		sSQL = `update system_parameters set integer_value = 1, date_value = now() where name = 'bank1_full' and integer_value = 0`
	}
	_, err := fuelgauge.pDB.Exec(sSQL)
	if err != nil {
		log.Println("Error trying to set the full charge flag for bank ", bank, " - ", err)
	}
}

/**
Test the full charge state of the given battery.
*/
func (fuelgauge *FuelGauge) TestFullCharge(bank uint8) (full bool) {
	switch bank {
	case 0:
		if fuelgauge.FgLeft.Coulombs >= float32(fuelgauge.FgLeft.Capacity) {
			fuelgauge.setFullCharge(0)
			return true
		}
	case 1:
		if fuelgauge.FgRight.Coulombs >= float32(fuelgauge.FgRight.Capacity) {
			fuelgauge.setFullCharge(1)
			return true
		}
	}
	return false
}

func (fuelgauge *FuelGauge) GetLastFullChargeTimes() string {
	var rowData struct {
		Bank0 string `json:"bank_0"`
		Bank1 string `json:"bank_1"`
	}

	rowData.Bank0 = fuelgauge.FgLeft.LastFullCharge.Time.Format(``)
	rowData.Bank1 = fuelgauge.FgRight.LastFullCharge.Time.Format(``)

	jsonString, _ := json.Marshal(rowData)
	return string(jsonString)
}

func (fuelgauge *FuelGauge) ReadyToWater(bank int16) bool {
	var percentCharged float32 = 0.0
	switch bank {
	case 0:
		percentCharged = (fuelgauge.FgLeft.Coulombs / float32(fuelgauge.FgLeft.Capacity)) * 100.0
	case 1:
		percentCharged = (fuelgauge.FgRight.Coulombs / float32(fuelgauge.FgRight.Capacity)) * 100.0
	}
	return percentCharged > WATERCHARGETHRESHOLD
}

func (fuelgauge *FuelGauge) GetCapacity() string {
	var rowData struct {
		Capacity0 int16 `json:"bank_0"`
		Capacity1 int16 `json:"bank_1"`
	}

	rowData.Capacity0 = fuelgauge.FgLeft.Capacity
	rowData.Capacity1 = fuelgauge.FgRight.Capacity

	jsonString, _ := json.Marshal(rowData)
	return string(jsonString)
}

func (fuelgauge *FuelGauge) Capacity() (total int16, left int16, right int16) {
	return fuelgauge.FgLeft.Capacity + fuelgauge.FgRight.Capacity, fuelgauge.FgLeft.Capacity, fuelgauge.FgRight.Capacity
}

/**
Pulse the selected relay on the given slave for the given time duration
*/
func (fuelgauge *FuelGauge) PulseRelay(relay uint16, slave uint8, seconds uint8) {
	// Turn the relay on
	log.Println("Pulse - turning relay ", relay, " on.")
	err := fuelgauge.mbus.WriteCoil(relay, true, slave)
	if err != nil {
		log.Println("Failed to turn on relay ", relay, " - ", err)
	}
	// Turn it off again after the delay period
	time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		log.Println("Pulse - turning relay ", relay, " off again.")
		err = fuelgauge.mbus.WriteCoil(relay, false, slave)
		if err != nil {
			log.Println("Failed to turn off relay ", relay, " - ", err)
		}
	})
}

/**
Switch off one bank. If the other bank is already off we turn it on first so there is always at least one bank on.
This is called when one bank reaches full charge to switch to the other bank.
It should also be called at 7PM to switch to the left bank in case we are still on the right.
This is done while the right bank cells are causing issues.
*/
func (fuelgauge *FuelGauge) SwitchOffBank(bank int) {
	var (
		onRelay           uint16
		offRelay          uint16
		thisBatterySense  uint16
		otherBatterySense uint16
	)

	//	log.Println("Switching off bank ", bank)

	if bank == LeftBank {
		onRelay = RightBankOnRelay         // Right bank on
		offRelay = LeftBankOffRelay        // Left bank off
		thisBatterySense = LeftBankSense   // Sense input for the selected bank
		otherBatterySense = RightBankSense // Right bank sense
	} else {
		onRelay = LeftBankOnRelay         // Left bank on
		offRelay = RightBankOffRelay      // Right bank off
		thisBatterySense = RightBankSense // Sense input for the selected bank
		otherBatterySense = LeftBankSense // Left bank sense
	}

	// If the bank is already switched off then do nothing and return
	if fuelgauge.FgLeft.ModbusData.Discrete[thisBatterySense] {
		return
	}

	if fuelgauge.FgLeft.ModbusData.Discrete[otherBatterySense] {
		// Activate the relay to switch on the other battery if it is switched off so there is always one active bank
		fuelgauge.PulseRelay(onRelay, fuelgauge.FgLeft.SlaveAddress, 2)
		log.Println("OtherBatterySense (", otherBatterySense, ") shows TRUE so attempting to turn on bank by pulsing relay ", onRelay)
		time.Sleep(time.Second * 15)
		// Now switch the selected battery off giving 15 seconds for the switching capacitor to discharge properly

		if !fuelgauge.FgLeft.ModbusData.Discrete[otherBatterySense] {
			fuelgauge.PulseRelay(offRelay, fuelgauge.FgLeft.SlaveAddress, 2)
		} else {
			log.Println("Attempt to turn on the other bank Failed. Cannot turn off bank ", bank)
			err := smtp.SendMail("mail.cedartechnology.com:587",
				smtp.PlainAuth("", "pi@cedartechnology.com", "7444561", "mail.cedartechnology.com"),
				"pi@cedartechnology.com", []string{"ian.abercrombie@cedartechnology.com"}, []byte(`From: Battery
To: Ian.Abercrombie@CedarTechnology.com
Subject: Battery Correction
Attempted to turn on a bank so the other bank can be turned off but the turn on command failed.`))
			if err != nil {
				log.Println("Failed to send email about having to turn on the left bank because both banks were off. - ", err)
			}
		}
	} else {
		// The other battery is already on so just turn ours off Activate the relay to switch off the selected battery
		fuelgauge.PulseRelay(offRelay, fuelgauge.FgLeft.SlaveAddress, 2)
	}
}

/**
Return the JSON representation of the data read from the controllers
*/
func (fuelgauge *FuelGauge) GetData() (string, error) {
	bytesLeftJSON, err := json.Marshal(fuelgauge.FgLeft)
	if err != nil {
		return "", err
	}
	bytesRightJSON, err := json.Marshal(fuelgauge.FgRight)
	if err != nil {
		return "", err
	}
	return `{"left":` + string(bytesLeftJSON) + `,"right":` + string(bytesRightJSON) + `}`, nil
}

/**
Draw the modbus coils, and registers table for display of data from one fuel gauge controller
*/
func (fuelgauge *FuelGauge) drawTable(w http.ResponseWriter, slave uint8, SlaveEndPoints []ModbusEndPoint) {
	var bClosed bool
	var onClick string
	var labelClass string
	var readOnly string
	var name string
	nIndex := 0
	var err error

	for _, ep := range SlaveEndPoints {
		if (nIndex % 4) == 0 {
			_, err = fmt.Fprint(w, `<tr>`)
			if err != nil {
				log.Println(err)
			}
			bClosed = false
		}
		if ep.writeable {
			onClick = `onclick="clickCoil('` + strconv.Itoa(int(slave)) + `','` + ep.id + `')"`
			labelClass = `class="readWrite"`
			readOnly = ``
			name = ` name="` + strconv.Itoa(int(ep.address)) + `"`
		} else {
			onClick = ""
			labelClass = ""
			readOnly = `readonly`
			name = ``

		}
		err = nil
		switch ep.dataType {
		case Coil:
			_, err = fmt.Fprint(w, `<td class="coil" `, onClick, `><span class="coilOff" id="c`, slave, `:`, ep.id, `">`, ep.name, `</span></td>`)
		case Discrete:
			_, err = fmt.Fprint(w, `<td class="discrete"><span class="discreteOff" id="d`, slave, `:`, ep.id, `">`, ep.name, `</span></td>`)
		case HoldingRegister:
			_, err = fmt.Fprint(w, `<td class="holdingRegister"><label for="h`, slave, ":", ep.id, `" `, labelClass, `>`, ep.name, `</label><input class="holdingRegister" type="text"`, name, ` id="h`, slave, ":", ep.id, `" multiplier="`, ep.multiplier, `" signed="`, ep.signed, `" value="" `, readOnly, `></td>`)
		case InputRegister:
			_, err = fmt.Fprint(w, `<td class="inputRegister"><label for="i`, slave, ":", ep.id, `">`, ep.name, `</label `, labelClass, `><input class="inputRegister" type="text" id="i`, slave, ":", ep.id, `" multiplier="`, ep.multiplier, `" signed="`, ep.signed, `" value="" readonly></td>`)
		case Blank:
			_, err = fmt.Fprint(w, `<td>&nbsp;</td>`)
		}
		if err != nil {
			log.Println(err)
		}
		nIndex++
		if (nIndex % 4) == 0 {
			_, err = fmt.Fprint(w, `</tr>`)
			if err != nil {
				log.Println(err)
			}
			bClosed = true
		}
	}
	if !bClosed {
		_, err = fmt.Fprint(w, "</tr>")
		if err != nil {
			log.Println(err)
		}
	}
}

/**
Render the WEB page for the fuel gauge controllers
*/
func (fuelgauge *FuelGauge) WebGetValues(w http.ResponseWriter, _ *http.Request) {

	_, err := fmt.Fprint(w, `<html>
  <head>
    <link rel="shortcut icon" href="">
    <title>Battery Fuel Gauge</title>
    <style>
      table{border-width:2px;border-style:solid}
      td.coilOff{border-width:1px;border-style:solid;text-align:center;background-color:darkgreen;color:white}
      td.coilOn{border-width:1px;border-style:solid;text-align:center;background-color:red;color:white}
      td.discreteOff{border-width:1px;border-style:solid;text-align:center;border:1px solid darkgreen}
      td.discreteOn{border-width:1px;border-style:solid;text-align:center;border:1px solid red}
      span.coilOff{color:white}
	  span.coilOn{color:white}
      span.discreteOn{color:red;font-weight:bold}
	  span.discreteOff{color:darkgreen;font-weight:bold}
      td.holdingRegister{border-width:1px;border-style:solid;text-align:right}
      td.inputRegister{border-width:1px;border-style:solid;text-align:right}
      input.holdingRegister{width: 50px; padding: 6px 1px;margin: 3px 0;box-sizing: border-box;border: 1px solid blue;background-color: #5CADEC;color: white;}
      input.inputRegister{width: 50px; padding: 6px 1px;margin: 3px 0;box-sizing: border-box;border: none;background-color: gainsboro;color: black;}
      label{padding: 6px; margin: 3px}
      label.readWrite{background-color:#68A068; color:white}
      button {
        font-family: Arial, Helvetica, sans-serif;
        font-size: 23px;
        color: #edfcfa;
        padding: 10px 20px;
        background: -moz-linear-gradient( top, #ffffff 0%, #6e5fd9 50%, #3673cf 50%, #000794);
        background: -webkit-gradient( linear, left top, left bottom, from(#ffffff), color-stop(0.50, #6e5fd9), color-stop(0.50, #3673cf), to(#000794));
        -moz-border-radius: 14px;
        -webkit-border-radius: 14px;
        border-radius: 14px;
        border: 1px solid #004d80;
        -moz-box-shadow:
        0px 1px 3px rgba(000, 000, 000, 0.5), inset 0px 0px 2px rgba(255, 255, 255, 1);
        -webkit-box-shadow:
        0px 1px 3px rgba(000, 000, 000, 0.5), inset 0px 0px 2px rgba(255, 255, 255, 1);
        box-shadow:
        0px 1px 3px rgba(000, 000, 000, 0.5), inset 0px 0px 2px rgba(255, 255, 255, 1);
        text-shadow:
        0px -1px 0px rgba(000, 000, 000, 0.2), 0px 1px 0px rgba(255, 255, 255, 0.4);
      }


    </style>
	<script type="text/javascript">
		(function() {
			var url = "ws://" + window.location.host + "/ws";
			var conn = new WebSocket(url);
			conn.onclose = function(evt) {
				alert('Connection closed');
			}
			conn.onmessage = function(evt) {
				data = JSON.parse(evt.data);
				data.fuelgauge.left.ModbusData.discrete.forEach(function (e, i) {setDiscrete(e, i, 5);});
				data.fuelgauge.right.ModbusData.discrete.forEach(function (e, i) {setDiscrete(e, i, 1);});
				data.fuelgauge.left.ModbusData.coil.forEach(function (e, i) {setCoil(e, i, 5);});
				data.fuelgauge.right.ModbusData.coil.forEach(function (e, i) {setCoil(e, i, 1);});
				data.fuelgauge.left.ModbusData.holding.forEach(function (e, i) {setHoldingReg(e, i, 5);});
				data.fuelgauge.right.ModbusData.holding.forEach(function (e, i) {setHoldingReg(e, i, 1);});
				data.fuelgauge.left.ModbusData.input.forEach(function (e, i) {setInputReg(e, i, 5);});
				data.fuelgauge.right.ModbusData.input.forEach(function (e, i) {setInputReg(e, i, 1);});
				if(data.lasterror !== undefined) {
					document.getElementById("error" + slave).innerText = data.lasterror;
				}
			}
		})();
		function setCoil(item, index, slave) {
			var control = document.getElementById("c" + slave + ":" + (index + 1));
			if (item) {
				control.className = "coilOn";
				control.parentElement.className = "coilOn";
			} else {
				control.className = "coilOff";
				control.parentElement.className = "coilOff";
			}
		}

		function setDiscrete(item, index, slave) {
			var control = document.getElementById("d" + slave + ":" + (index + 1));
			if (item) {
				control.className = "discreteOn";
				control.parentElement.className = "discreteOn";
			} else {
				control.className = "discreteOff";
				control.parentElement.className = "discreteOff";
			}
		}

		function setHoldingReg(item, index, slave) {
			control = document.getElementById("h" + slave + ":" + (index + 1));
			if((document.activeElement.id == null) || (document.activeElement.id != control.id)) {
				if ((control.attributes["signed"].value == "true") && (item > 32767)) {
					item = item - 65536;
				}
				control.value = item / control.attributes["multiplier"].value;
			}
		}

		function setInputReg(item, index, slave) {
			control = document.getElementById("i" + slave + ":" + (index + 1));
			if ((control.attributes["signed"].value == "true") && (item > 32767)) {
				item = item - 65536;
			}
			control.value = item / control.attributes["multiplier"].value;
		}


		function clickCoil(slave, id) {
			var xhr = new XMLHttpRequest();
			xhr.open('PATCH','toggleCoil?coil=' + id + '&slave=' + slave);
			xhr.send();
		}

		function getElementVal(control) {
			var v = control.value;
			var m = 1;
			if (control.hasAttribute("multiplier")) {
				m = control.attributes["multiplier"].value;
			}
			if(isNaN(m)) {
				return v;
			} else {
				return v * m;
			}
		}

		function sendFormData(form, url) {
			var urlEncode = function(data, rfc3986) {
				if (typeof rfc3986 === 'undefined') {
					rfc3986 = true;
				}
				// Encode value
				data = encodeURIComponent(data);
				data = data.replace(/%20/g, '+');
				// RFC 3986 compatibility
				if (rfc3986) {
					data = data.replace(/[!'()*]/g, function(c) {
						return '%' + c.charCodeAt(0).toString(16);
					});
				}
				return data;
			}
			form = document.getElementById(form);

			var frmDta = "";
			for (var i=0; i < form.elements.length; ++i) {
				if (form.elements[i].name != ''){
					if (frmDta.length != 0) {
						frmDta = frmDta + "&";
					}
					frmDta = frmDta + urlEncode(form.elements[i].name) + '=' + urlEncode(getElementVal(form.elements[i]));
				}
			}
			var xhr = new XMLHttpRequest();
			xhr.open('POST', url, true);
			xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
			xhr.send(frmDta);
		}
		function clearErrors() {
			document.getElementById("error`, fuelgauge.FgLeft.SlaveAddress, `").innerText = "";`)
	if err != nil {
		log.Println(err)
		return
	}
	if fuelgauge.FgLeft.SlaveAddress != 0 {
		_, err = fmt.Fprint(w, `			document.getElementById("error`, fuelgauge.FgLeft.SlaveAddress, `").innerText = "";`)
		if err != nil {
			log.Println(err)
			return
		}
	}
	_, err = fmt.Fprint(w, `	}
	</script>
  </head>
  <body>
	<h1>Battery Management</h1>
    <h2>Connected on `, fuelgauge.commsPort, ` at `, fuelgauge.baudRate, ` baud</h2>
    <div id="leftBattery">
      <form onsubmit="return false;" id="modbus1Form">
		<input type="hidden" name="slave" value="`, fuelgauge.FgLeft.SlaveAddress, `">
        <table class="pumps"><tr><td colspan=2 style="text-align:center">---Key---</td><td class="coilOn">===ON===</td><td class="coilOff">===OFF===</td></tr>`)
	fuelgauge.drawTable(w, fuelgauge.FgLeft.SlaveAddress, EndPointsLeft)
	if err != nil {
		log.Println(err)
		return
	}

	_, err = fmt.Fprint(w, `
        </table>
        <br /><button class="frmSubmit" type="text" onclick="sendFormData('modbus1Form', 'setHoldingRegisters')">Submit</button>&nbsp;<span id="error`, fuelgauge.FgLeft.SlaveAddress, `"></span>
      </form>
    </div>`)
	if err != nil {
		log.Println(err)
		return
	}
	if fuelgauge.FgRight.SlaveAddress != 0 {
		_, err = fmt.Fprint(w, ` <div id="rightBattery">
      <form onsubmit="return false;" id="modbus2Form">
		<input type="hidden" name="slave" value="`, fuelgauge.FgRight.SlaveAddress, `">
        <table class="pumps"><tr><td colspan=2 style="text-align:center">---Key---</td><td class="coilOn">===ON===</td><td class="coilOff">===OFF===</td></tr>`)
		fuelgauge.drawTable(w, fuelgauge.FgRight.SlaveAddress, EndPointsRight)
		if err != nil {
			log.Println(err)
			return
		}
		_, err = fmt.Fprint(w, `
        </table>
        <br /><button class="frmSubmit" type="text" onclick="sendFormData('modbus2Form', 'setHoldingRegisters')">Submit</button>&nbsp;<span id="error`, fuelgauge.FgRight.SlaveAddress, `"></span>
      </form>
    </div>`)
		if err != nil {
			log.Println(err)
			return
		}
	}
	_, err = fmt.Fprint(w, `
    <div>
      <button class="frmSubmit" type="text" onclick="clearErrors();">Clear Errors</button>
    </div>
  </body>
</html>`)
	if err != nil {
		log.Println(err)
	}
}

/**
Perform the bank watering function. Turn on the relevant valve for the requested time in minutes.
*/
func (fuelgauge *FuelGauge) WaterBank(bank uint8, timer uint8) error {
	var relay uint16
	// Solenoids are on coils 6 & 8 of the right bank controller.
	if bank == LeftBank {
		relay = LeftWaterRelay
	} else {
		relay = RightWaterRelay
	}
	err := fuelgauge.mbus.WriteCoil(relay, true, fuelgauge.FgRight.SlaveAddress)
	if err == nil {
		time.AfterFunc(time.Duration(timer)*time.Minute, func() {
			err := fuelgauge.mbus.WriteCoil(relay, false, fuelgauge.FgRight.SlaveAddress)
			if err != nil {
				log.Println(err)
				return
			}
		})
	} else {
		return err
	}
	return nil
}

/**
Turn on the watering system for the bank for the minutes.
Send PATCH to URL: /waterBank/{bank}/{minutes}
*/
func (fuelgauge *FuelGauge) WebWaterBank(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	bank, err := strconv.ParseUint(vars["bank"], 10, 8)
	if (err != nil) || (bank > 1) {
		http.Error(w, "Invalid battery bank", http.StatusBadRequest)
		return
	}
	timer, err := strconv.ParseUint(vars["minutes"], 10, 8)
	if (err != nil) || (timer > 15) {
		http.Error(w, "Invalid minutes for watering time", http.StatusBadRequest)
		return
	}

	err = fuelgauge.WaterBank(uint8(bank), uint8(timer))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/**
Turn on the fan if it is off
*/
func (fuelgauge *FuelGauge) TurnOnFan() {
	if !fuelgauge.FgLeft.ModbusData.Coil[BatteryFanRelay-1] {
		err := fuelgauge.mbus.WriteCoil(BatteryFanRelay, true, fuelgauge.FgLeft.SlaveAddress)
		if err != nil {
			log.Println("Failed to turn the battery fan on", err)
		}
	}
}

/**
Turn off the fan if it is on
*/
func (fuelgauge *FuelGauge) TurnOffFan() {
	if fuelgauge.FgLeft.ModbusData.Coil[BatteryFanRelay-1] {
		err := fuelgauge.mbus.WriteCoil(BatteryFanRelay, false, fuelgauge.FgLeft.SlaveAddress)
		if err != nil {
			log.Println("Failed to turn the battery fan off", err)
		}
	}
}

/**
Turn on or off the generator.
Send PATCH to URL: /generator/{action}
*/
func (fuelgauge *FuelGauge) WebGeneratorStartStop(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var OnOff bool

	if strings.EqualFold(vars["action"], "start") {
		OnOff = true
	} else if strings.EqualFold(vars["action"], "stop") {
		OnOff = false
	} else {
		http.Error(w, "Start or Stop expected", http.StatusBadRequest)
		return
	}
	// Fan relay is on coil 8 of the left bank controller
	err := fuelgauge.mbus.WriteCoil(GeneratorRelay, OnOff, fuelgauge.FgLeft.SlaveAddress)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/**
Turn on or off the battery house ventilation fan for the minutes.
Send PATCH to URL: /batteryFan/{onOff}
*/
func (fuelgauge *FuelGauge) WebBatteryFan(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var OnOff bool

	if strings.EqualFold(vars["onOff"], "on") {
		OnOff = true
	} else if strings.EqualFold(vars["onOff"], "off") {
		OnOff = false
	} else {
		http.Error(w, "On or Off expected", http.StatusBadRequest)
		return
	}
	// Fan relay is on coil 8 of the left bank controller
	err := fuelgauge.mbus.WriteCoil(BatteryFanRelay, OnOff, fuelgauge.FgLeft.SlaveAddress)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/**
Switch one of the battery banks on or off.
/batterySwitch/{bank}/{onOff}
Left bank = 0, Right bank = 1
onOff = 'on' or 'off'
Left controller Relay 1 = On for the left bank
Left controller Relay 2 = Off for the left bank
Left controller Relay 3 = On for the right bank
Left controller Relay 4 = Off for the right bank
*/
func (fuelgauge *FuelGauge) WebSwitchBattery(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var OnOff bool

	if strings.EqualFold(vars["onOff"], "on") {
		OnOff = true
	} else if strings.EqualFold(vars["onOff"], "off") {
		OnOff = false
	} else {
		http.Error(w, "On or Off expected", http.StatusBadRequest)
		return
	}

	var relay uint16
	switch vars["bank"] {
	case "0":
		if OnOff {
			relay = LeftBankOnRelay
		} else {
			// If the right battery is off (discrete input 8 = 1) DO NOT turn the left bank off
			if fuelgauge.FgLeft.ModbusData.Discrete[RightBankSense] {
				http.Error(w, "Cannot turn both batteries off.", http.StatusBadRequest)
				return
			}
			relay = LeftBankOffRelay
		}
	case "1":
		if OnOff {
			relay = RightBankOnRelay
		} else {
			// If the left battery is off (discrete input 8 = 1) DO NOT turn the left bank off
			if fuelgauge.FgLeft.ModbusData.Discrete[LeftBankSense] {
				http.Error(w, "Cannot turn both batteries off.", http.StatusBadRequest)
				return
			}
			relay = RightBankOffRelay
		}
	default:
		http.Error(w, "bank 0(left) or 1(right) expected", http.StatusBadRequest)
		return
	}

	// Activate the relay to switch the battery
	err := fuelgauge.mbus.WriteCoil(relay, true, fuelgauge.FgLeft.SlaveAddress)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	time.Sleep(time.Second * 2)
	err = fuelgauge.mbus.WriteCoil(relay, false, fuelgauge.FgLeft.SlaveAddress)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
}

/**
Get the charging efficiences from the system parameters
*/
func (fuelgauge *FuelGauge) getChargingEfficiencies(db *sql.DB) error {
	rows, err := db.Query("select name, double_value from system_parameters where name like 'charge__efficiency' order by name")
	if err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			log.Println(closeErr)
		}
		return err
	} else {
		for rows.Next() {
			var name string
			var efficiency float64
			err = rows.Scan(&name, &efficiency)
			if err != nil {
				closeErr := db.Close()
				if closeErr != nil {
					log.Println(closeErr)
				}
				return err
			}
			if name == "charge0_efficiency" {
				fuelgauge.FgLeft.Efficiency = efficiency
			} else if name == "charge1_efficiency" {
				fuelgauge.FgRight.Efficiency = efficiency
			} else {
				closeErr := db.Close()
				if closeErr != nil {
					log.Println(closeErr)
				}
				return errors.New("unknown entry in system parameters found matching query 'charge__efficiency'")
			}
		}
	}
	return nil
}

/**
Initialise a new FuelGauge object and connect to the two fuel gauge controllers using the given parameters
*/
func New(commsPort string, baudRate int, dataBits int, stopBits int, parity string, timeout time.Duration, pDataBase *sql.DB, slave1Address uint8, slave2Address uint8) *FuelGauge {
	var err error
	this := new(FuelGauge)
	this.commsPort = commsPort
	this.baudRate = baudRate
	this.FgLeft.SlaveAddress = slave1Address
	this.FgRight.SlaveAddress = slave2Address
	this.FgLeft.ModbusData = Data.New(16, 1, 9, 1, 11, 1, 8, 1, this.FgLeft.SlaveAddress)
	this.FgRight.ModbusData = Data.New(16, 1, 9, 1, 11, 1, 8, 1, this.FgRight.SlaveAddress)

	// Set up the database connection
	this.pDB = pDataBase
	this.mbus = ModbusController.New(commsPort, baudRate, dataBits, stopBits, parity, timeout)
	if this.mbus != nil {
		defer this.mbus.Close()

		err = this.mbus.Connect()
		if err != nil {
			log.Println("Port = ", commsPort, " baudrate = ", baudRate)
			panic(err)
		}
	}
	this.currentStatement, err = pDataBase.Prepare(`insert into current (logged, channel_0, channel_1, level_of_charge_0, level_of_charge_1) values (now(),?,?,?,?)`)
	if err != nil {
		log.Println("Error preparing insert current statement - ", err)
		panic(err)
	}

	this.systemParamStatement, err = pDataBase.Prepare(`update system_parameters set double_value = double_value + ? where name = ?`)
	if err != nil {
		log.Println("Error preparing system parameter update statement - ", err)
		panic(err)
	}

	this.fullChargeStatement, err = pDataBase.Prepare(`select count(*) from serial_numbers where full_charge = 1 and cell_number between ? and ?`)
	if err != nil {
		log.Println("Error preparing full cell count statement - ", err)
		panic(err)
	}

	rows, err := pDataBase.Query("select logged, level_of_charge_0, level_of_charge_1 from current order by logged desc limit 1")
	if err != nil {
		log.Println("Error getting last current values - ", err)
		panic(err)
	} else {
		this.FgLeft.Coulombs = 0
		this.FgRight.Coulombs = 0
		for rows.Next() {
			var count0 float32
			var count1 float32
			var when sql.NullTime
			err = rows.Scan(&when, &count0, &count1)
			if err != nil {
				log.Println("Failed to get the last coulomb counts - ", err)
				panic(err)
			}
			this.FgLeft.Coulombs = count0
			this.FgLeft.LastUpdate = when.Time
			this.FgRight.Coulombs = count1
			this.FgRight.LastUpdate = when.Time
		}
	}
	err = this.getChargingEfficiencies(pDataBase)
	if err != nil {
		log.Println("Failed to get the charging efficiencies - ", err)
		panic(err)
	}
	return this
}

// Temporarily exclude the right battery as it is not working
func (fuelgauge *FuelGauge) StateOfCharge() float32 {
	//	return ((fuelgauge.FgLeft.Coulombs + fuelgauge.FgRight.Coulombs) * 100) / float32(fuelgauge.FgLeft.Capacity+fuelgauge.FgRight.Capacity)
	return fuelgauge.StateOfChargeLeft()
}

func (fuelgauge *FuelGauge) StateOfChargeLeft() float32 {
	return ((fuelgauge.FgLeft.Coulombs) * 100) / float32(fuelgauge.FgLeft.Capacity)
}

func (fuelgauge *FuelGauge) StateOfChargeRight() float32 {
	return ((fuelgauge.FgRight.Coulombs) * 100) / float32(fuelgauge.FgRight.Capacity)
}

func (fuelgauge *FuelGauge) Current() float32 {
	//	return float32(int16(fuelgauge.FgLeft.ModbusData.Input[AvgCurrent-1])+int16(fuelgauge.FgRight.ModbusData.Input[AvgCurrent-1])) / 100.0
	return float32(int16(fuelgauge.FgLeft.ModbusData.Input[AvgCurrent-1])) / 100.0
}
