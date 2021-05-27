package FuelGauge

import (
	"ModbusBatteryFuelGauge/Data"
	"ModbusBatteryFuelGauge/modbusController"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
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
	LastFullCharge mysql.NullTime
}

type batterySettings struct {
	CapacityLeft        float32
	CapacityRight       float32
	LastFullChargeLeft  mysql.NullTime
	LastFullChargeRight mysql.NullTime
}

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

const WATER_CHARGE_THRESHOLD = 98.0 // The state of charge point reached at which the watering system is turned on.
const LEFT_BANK = 0
const RIGHT_BANK = 1
const LEFT_BANK_CELL_COUNT = 37
const RIGHT_BANK_CELL_COUNT = 38

// Digital Inputs
const LEFT_BANK_SENSE = 0
const RIGHT_BANK_SENSE = 1

// Coils
const LEFT_BANK_ON_RELAY = 1
const LEFT_BANK_OFF_RELAY = 2
const RIGHT_BANK_ON_RELAY = 3
const RIGHT_BANK_OFF_RELAY = 4
const BATTERY_FAN_RELAY = 8
const LEFT_WATER_RELAY = 6
const RIGHT_WATER_RELAY = 8

// Input Registers
const CURRENT_REGISTER = 1
const ANALOGUE_0 = 2
const ANALOGUE_1 = 3
const ANALOGUE_2 = 4
const ANALOGUE_3 = 5
const ANALOGUE_6 = 6
const ANALOGUE_7 = 7
const AVG_CURRENT = 8
const RAW_CURRENT = 9
const UPTIME_LOW = 10
const UPTIME_HIGH = 11

// Holding Registers
const SLAVE_ID_REGISTER = 1
const BAUD_RATE_REGISTER = 2
const OFFSET_REGISTER = 3
const PGA_GAIN_REGISTER = 4
const SAMPLES_PER_SEC_REG = 5
const CHARGE_REGISTER = 6
const CURRENT_GAIN_REG = 7
const EFFICIENCY_REG = 8

// Coils
const RELAY_1_COIL = 1
const RELAY_2_COIL = 2
const RELAY_3_COIL = 3
const RELAY_4_COIL = 4
const RELAY_5_COIL = 5
const RELAY_6_COIL = 6
const RELAY_7_COIL = 7
const RELAY_8_COIL = 8
const MOMENTARY_1_COIL = 9
const MOMENTARY_2_COIL = 10
const MOMENTARY_3_COIL = 11
const MOMENTARY_4_COIL = 12
const MOMENTARY_5_COIL = 13
const MOMENTARY_6_COIL = 14
const MOMENTARY_7_COIL = 15
const MOMENTARY_8_COIL = 16

// Discrete Inputs
const DISCRETE_1 = 1
const DISCRETE_2 = 2
const DISCRETE_3 = 3
const DISCRETE_4 = 4
const DISCRETE_5 = 5
const DISCRETE_6 = 6
const DISCRETE_7 = 7
const DISCRETE_8 = 8
const I2C_FAILURE = 9

var EndPoints = []ModbusEndPoint{
	{"1", "Current", CURRENT_REGISTER, InputRegister, 100, "A", false, true},
	{"2", "Analog In 0", ANALOGUE_0, InputRegister, 1, "", false, false},
	{"3", "Analog In 1", ANALOGUE_1, InputRegister, 1, "", false, false},
	{"4", "Analog In 2", ANALOGUE_2, InputRegister, 1, "", false, false},
	{"5", "Analog In 3", ANALOGUE_3, InputRegister, 1, "", false, false},
	{"6", "Analog In 6", ANALOGUE_6, InputRegister, 1, "", false, true},
	{"7", "Analog In 7", ANALOGUE_7, InputRegister, 1, "", false, true},
	{"8", "Avg Current", AVG_CURRENT, InputRegister, 100, "A", false, true},
	{"9", "Raw Current", RAW_CURRENT, InputRegister, 1, "", false, true},
	{"x1", "", 0, Blank, 0, "", false, false},
	{"10", "Up Time Low", UPTIME_LOW, InputRegister, 1, "", false, false},
	{"11", "Up Time High", UPTIME_HIGH, InputRegister, 1, "", false, false},

	{"1", "Slave ID", SLAVE_ID_REGISTER, HoldingRegister, 1, "", true, false},
	{"2", "Baud Rate", BAUD_RATE_REGISTER, HoldingRegister, 1, "", true, false},
	{"3", "Offset", OFFSET_REGISTER, HoldingRegister, 1, "", true, true},
	{"4", "PGA Gain", PGA_GAIN_REGISTER, HoldingRegister, 1, "", true, false},
	{"5", "Samples Per Sec", SAMPLES_PER_SEC_REG, HoldingRegister, 1, "", true, false},
	{"6", "Charge", CHARGE_REGISTER, HoldingRegister, 10, "Ahr", true, true},
	{"7", "Current Gain", CURRENT_GAIN_REG, HoldingRegister, 1, "", true, true},
	{"8", "Charge Efficiency", EFFICIENCY_REG, HoldingRegister, 2, "%", true, true},

	{"1", "Relay 1", RELAY_1_COIL, Coil, 1, "", true, false},
	{"2", "Relay 2", RELAY_2_COIL, Coil, 1, "", true, false},
	{"3", "Relay 3", RELAY_3_COIL, Coil, 1, "", true, false},
	{"4", "Relay 4", RELAY_4_COIL, Coil, 1, "", true, false},
	{"5", "Relay 5", RELAY_5_COIL, Coil, 1, "", true, false},
	{"6", "Relay 6", RELAY_6_COIL, Coil, 1, "", true, false},
	{"7", "Relay 7", RELAY_7_COIL, Coil, 1, "", true, false},
	{"8", "Relay 8", RELAY_8_COIL, Coil, 1, "", true, false},
	{"9", "Momentary 1", MOMENTARY_1_COIL, Coil, 1, "", true, false},
	{"10", "Momentary 2", MOMENTARY_2_COIL, Coil, 1, "", true, false},
	{"11", "Momentary 3", MOMENTARY_3_COIL, Coil, 1, "", true, false},
	{"12", "Momentary 4", MOMENTARY_4_COIL, Coil, 1, "", true, false},
	{"13", "Momentary 5", MOMENTARY_5_COIL, Coil, 1, "", true, false},
	{"14", "Momentary 6", MOMENTARY_6_COIL, Coil, 1, "", true, false},
	{"15", "Momentary 7", MOMENTARY_7_COIL, Coil, 1, "", true, false},
	{"16", "Momentary 8", MOMENTARY_8_COIL, Coil, 1, "", true, false},

	{"1", "Digital In 1", DISCRETE_1, Discrete, 1, "", false, false},
	{"2", "Digital In 2", DISCRETE_2, Discrete, 1, "", false, false},
	{"3", "Digital In 3", DISCRETE_3, Discrete, 1, "", false, false},
	{"4", "Digital In 4", DISCRETE_4, Discrete, 1, "", false, false},
	{"5", "Digital In 5", DISCRETE_5, Discrete, 1, "", false, false},
	{"6", "Digital In 6", DISCRETE_6, Discrete, 1, "", false, false},
	{"7", "Digital In 7", DISCRETE_7, Discrete, 1, "", false, false},
	{"8", "Digital In 8", DISCRETE_8, Discrete, 1, "", false, false},
	{"9", "I2C Failure", I2C_FAILURE, Discrete, 1, "", false, false},
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
func (this *FuelGauge) WebToggleCoil(w http.ResponseWriter, r *http.Request) {
	address := r.FormValue("coil")
	var dataPointer *Data.Data

	slaveVal, _ := strconv.ParseUint(r.FormValue("slave"), 10, 16)
	if uint8(slaveVal) == (this.FgLeft.SlaveAddress) {
		dataPointer = this.FgLeft.ModbusData
	} else {
		dataPointer = this.FgRight.ModbusData
	}
	// coil
	n, _ := strconv.ParseUint(address, 10, 16)

	nIndex := uint16(n)
	nIndex = nIndex - dataPointer.CoilStart() // nIndex is now 0 based
	err := this.mbus.WriteCoil(uint16(nIndex)+dataPointer.CoilStart(), !dataPointer.Coil[nIndex], dataPointer.SlaveAddress)
	if err != nil {
		dataPointer.LastError = err.Error()
	}
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, "Coil ", nIndex+dataPointer.CoilStart(), " on slave ", dataPointer.SlaveAddress, " toggled.")
	//	getValues(true, dataPointer, this.mbus)
}

/**
Process form POST commands from the fuel gauge controller forms
*/
func (this *FuelGauge) WebProcessHoldingRegistersForm(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	var dataPointer *Data.Data
	//	var holdingValues []uint16
	//	var i int

	slaveVal, _ := strconv.ParseUint(r.FormValue("slave"), 10, 16)
	if uint8(slaveVal) == (this.FgLeft.SlaveAddress) {
		dataPointer = this.FgLeft.ModbusData
	} else {
		dataPointer = this.FgRight.ModbusData
	}
	if err != nil {
		fmt.Fprint(w, `<html><head><title>Error</title></head><body><h1>`, err, `</h1></body></html>`)
	}
	for sKey, sValue := range r.Form {
		nValue, _ := strconv.ParseFloat(sValue[0], 32)

		for _, ep := range EndPoints {
			if (ep.id == sKey) && (ep.dataType == HoldingRegister) {
				//				log.Println("Holding ", ep.id, " set to ", nValue)
				err = this.mbus.WriteHoldingRegister(uint16(ep.address), uint16(nValue), dataPointer.SlaveAddress)
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
func (this *FuelGauge) getValues(bRefresh bool, lastValues *Data.Data, p *ModbusController.ModbusController) {
	slaveID := lastValues.SlaveAddress
	newValues := Data.New(lastValues.GetSpecs())
	if len(newValues.Discrete) > 0 {
		mbData, err := p.ReadMultipleDiscreteRegisters(newValues.DiscreteStart(), uint16(len(newValues.Discrete)), newValues.SlaveAddress)
		if err != nil {
			log.Println("Error getting discrete inputs from slave ID ", slaveID, " - ", err)
			lastValues.LastError = err.Error()
		} else {
			copy(newValues.Discrete[:], mbData)
		}
	}
	if len(newValues.Coil) > 0 {
		mbData, err := p.ReadMultipleCoils(newValues.CoilStart(), uint16(len(newValues.Coil)), newValues.SlaveAddress)
		if err != nil {
			log.Println("Error getting coils from slave ID ", slaveID, " - ", err)
			lastValues.LastError = err.Error()
		} else {
			copy(newValues.Coil[:], mbData)
		}
	}
	if len(newValues.Holding) > 0 {
		mbUintData, err := p.ReadMultipleHoldingRegisters(newValues.HoldingStart(), uint16(len(newValues.Holding)), newValues.SlaveAddress)
		if err != nil {
			log.Println("Error getting holding registers from slave ID ", slaveID, " - ", err)
			lastValues.LastError = err.Error()
		} else {
			//			mbUintData := make([]uint16, len(mbUintData))
			//			for i, v := range mbUintData {
			//				mbUintData[i] = uint16(v)
			//			}
			copy(newValues.Holding[:], mbUintData)
		}
	}
	if len(newValues.Input) > 0 {
		mbUintData, err := p.ReadMultipleInputRegisters(newValues.InputStart(), uint16(len(newValues.Input)), newValues.SlaveAddress)
		if err != nil {
			log.Println("Error getting input registers from slave ID ", slaveID, " - ", err)
			lastValues.LastError = err.Error()
		} else {
			copy(newValues.Input[:], mbUintData)
		}
	}
	lastValues.Update(newValues)
}

/**
Set the charge level for the selected channel
*/
func (this *FuelGauge) SetCharge(slave uint8, charge float32) {
	this.mbus.WriteHoldingRegister(5, uint16(charge), slave)
}

/**
Log the data read into the database
*/
func (this *FuelGauge) logValues(current_0 uint16, current_1 uint16, charge_0 float32, charge_1 float32) {
	i0 := int16(current_0)
	i1 := int16(current_1)
	c0 := charge_0
	c1 := charge_1

	// Trim to capacity
	if c0 > float32(this.FgLeft.Capacity) {
		c0 = float32(this.FgLeft.Capacity)
	} else if c0 < 0.1 {
		c0 = 0.1
	}

	if c1 > float32(this.FgRight.Capacity) {
		c1 = float32(this.FgRight.Capacity)
	} else if c1 < 0.1 {
		c1 = 0.1
	}

	_, err := this.currentStatement.Exec(float32(i0)/100.0, float32(i1)/100.0, c0, c1)
	if err != nil {
		log.Println(err)
	}

	delta := int(charge_0) - int(this.FgLeft.Coulombs)
	// Take care of overflow of the coulomb counter
	if delta > 65000 {
		delta = delta - 65536
	} else if delta < -65000 {
		delta = delta + 65536
	}
	if delta > 0 {
		_, err = this.systemParamStatement.Exec(float64(delta)*this.FgLeft.Efficiency, "charge_in_counter_0")
		if err != nil {
			log.Println("Adding charge to bank 0 - ", err)
		}
	} else if delta < 0 {
		_, err = this.systemParamStatement.Exec(0-delta, "charge_out_counter_0")
		if err != nil {
			log.Println("Reducing charge to bank 0 - ", err)
		}
		if charge_0 > float32(this.FgLeft.Capacity) {
			this.SetCharge(this.FgLeft.SlaveAddress, c0)
		}
	}
	if charge_0 != c0 {
		// Update the controller because we needed to correct the charge level
		this.mbus.WriteHoldingRegister(CHARGE_REGISTER, uint16(c0*10.0), this.FgLeft.SlaveAddress)
		charge_0 = c0
	}
	this.FgLeft.Coulombs = charge_0
	delta = int(charge_1) - int(this.FgRight.Coulombs)
	if delta > 0 {
		_, err = this.systemParamStatement.Exec(float64(delta)*this.FgRight.Efficiency, "charge_in_counter_1")
		if err != nil {
			log.Println("Adding charge to bank 1 - ", err)
		}
	} else if delta < 0 {
		_, err = this.systemParamStatement.Exec(0-delta, "charge_out_counter_1")
		if err != nil {
			log.Println("Reducing charge to bank 1 - ", err)
		}
		if charge_1 > float32(this.FgRight.Capacity) {
			this.SetCharge(this.FgRight.SlaveAddress, c1)
		}
	}
	if charge_1 != c1 {
		// Update the controller because we needed to correct the charge level
		this.mbus.WriteHoldingRegister(CHARGE_REGISTER, uint16(c1*10.0), this.FgRight.SlaveAddress)
		charge_1 = c1
	}
	this.FgRight.Coulombs = charge_1
}

/**
Check that at least one battery is connected
*/
func (this *FuelGauge) CheckBatteryConnectionState() {
	if this.FgLeft.ModbusData.Discrete[LEFT_BANK_SENSE] && this.FgLeft.ModbusData.Discrete[RIGHT_BANK_SENSE] {
		// Both batteries appear to be switched off so we need to switch the left bank on.
		this.PulseRelay(LEFT_BANK_ON_RELAY, this.FgLeft.SlaveAddress, 2)
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
func (this *FuelGauge) Run() {
	reportTicker := time.NewTicker(time.Second)
	defer reportTicker.Stop()
	for {
		select {
		case <-reportTicker.C:
			{
				this.FgLeft.ModbusData.LastError = ""
				this.getValues(false, this.FgLeft.ModbusData, this.mbus)

				this.FgRight.ModbusData.LastError = ""
				this.getValues(false, this.FgRight.ModbusData, this.mbus)

				this.CheckBatteryConnectionState()
				this.logValues(this.FgLeft.ModbusData.Input[7], this.FgRight.ModbusData.Input[7], float32(this.FgLeft.ModbusData.Holding[5])/10, float32(this.FgRight.ModbusData.Holding[5])/10)
			}
		}
	}
}

/**
Read the capacity and last full charge datetime vaules from the database
*/
func (this *FuelGauge) ReadSystemParameters() {
	row := this.pDB.QueryRow(`select date_value from system_parameters where name = 'bank0_full'`)
	err := row.Scan(&this.FgLeft.LastFullCharge)
	if err != nil {
		log.Println("Error getting last full charge left from system parameters - ", err)
	}
	row = this.pDB.QueryRow(`select date_value from system_parameters where name = 'bank1_full'`)
	err = row.Scan(&this.FgRight.LastFullCharge)
	if err != nil {
		log.Println("Error getting last full charge right from system parameters - ", err)
	}
	row = this.pDB.QueryRow(`select integer_value from system_parameters where name = 'capacity_0'`)
	err = row.Scan(&this.FgLeft.Capacity)
	if err != nil {
		log.Println("Error getting left bank capacity from system parameters - ", err)
	}
	row = this.pDB.QueryRow(`select integer_value from system_parameters where name = 'capacity_1'`)
	err = row.Scan(&this.FgRight.Capacity)
	if err != nil {
		log.Println("Error getting right bank capacity from system parameters - ", err)
	}
}

func (this *FuelGauge) setFullCharge(bank int) {
	var sSQL string
	if bank == 0 {
		sSQL = `update system_parameters set integer_value = 1, date_value = now() where name = 'bank0_full' and integer_value = 0`
	} else {
		sSQL = `update system_parameters set integer_value = 1, date_value = now() where name = 'bank1_full' and integer_value = 0`
	}
	_, err := this.pDB.Exec(sSQL)
	if err != nil {
		log.Println("Error trying to set the full charge flag for bank ", bank, " - ", err)
	}
}

/**
Test the full charge state of the given battery.
*/
func (this *FuelGauge) TestFullCharge(bank uint8) (full bool) {
	switch bank {
	case 0:
		if this.FgLeft.Coulombs >= float32(this.FgLeft.Capacity) {
			this.setFullCharge(0)
			return true
		}
	case 1:
		if this.FgRight.Coulombs >= float32(this.FgRight.Capacity) {
			this.setFullCharge(1)
			return true
		}
	}
	return false
}

func (this *FuelGauge) GetLastFullChargeTimes() string {
	var rowData struct {
		Bank0 string `json:"bank_0"`
		Bank1 string `json:"bank_1"`
	}

	rowData.Bank0 = this.FgLeft.LastFullCharge.Time.Format(``)
	rowData.Bank1 = this.FgRight.LastFullCharge.Time.Format(``)

	jsonString, _ := json.Marshal(rowData)
	return string(jsonString)
}

func (this *FuelGauge) ReadyToWater(bank int16) bool {
	var percentCharged float32 = 0.0
	switch bank {
	case 0:
		percentCharged = (this.FgLeft.Coulombs / float32(this.FgLeft.Capacity)) * 100.0
	case 1:
		percentCharged = (this.FgRight.Coulombs / float32(this.FgRight.Capacity)) * 100.0
	}
	return percentCharged > WATER_CHARGE_THRESHOLD
}

func (this *FuelGauge) GetCapacity() string {
	var rowData struct {
		Capacity0 int16 `json:"bank_0"`
		Capacity1 int16 `json:"bank_1"`
	}

	rowData.Capacity0 = this.FgLeft.Capacity
	rowData.Capacity1 = this.FgRight.Capacity

	jsonString, _ := json.Marshal(rowData)
	return string(jsonString)
}

func (this *FuelGauge) Capacity() (total int16, left int16, right int16) {
	return this.FgLeft.Capacity + this.FgRight.Capacity, this.FgLeft.Capacity, this.FgRight.Capacity
}

/**
Pulse the selected relay on the given slave for the given time duration
*/
func (this *FuelGauge) PulseRelay(relay uint16, slave uint8, seconds uint8) {
	// Turn the relay on
	log.Println("Pulse - turning relay ", relay, " on.")
	err := this.mbus.WriteCoil(relay, true, slave)
	if err != nil {
		log.Println("Failed to turn on relay ", relay, " - ", err)
	}
	// Turn it off again after the delay period
	time.AfterFunc(time.Duration(seconds)*time.Second, func() {
		log.Println("Pulse - turning relay ", relay, " off again.")
		err = this.mbus.WriteCoil(relay, false, slave)
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
func (this *FuelGauge) SwitchOffBank(bank int) {
	var (
		onRelay           uint16
		offRelay          uint16
		thisBatterySense  uint16
		otherBatterySense uint16
	)

	//	log.Println("Switching off bank ", bank)

	if bank == LEFT_BANK {
		onRelay = RIGHT_BANK_ON_RELAY        // Right bank on
		offRelay = LEFT_BANK_OFF_RELAY       // Left bank off
		thisBatterySense = LEFT_BANK_SENSE   // Sense input for the selected bank
		otherBatterySense = RIGHT_BANK_SENSE // Right bank sense
	} else {
		onRelay = LEFT_BANK_ON_RELAY        // Left bank on
		offRelay = RIGHT_BANK_OFF_RELAY     // Right bank off
		thisBatterySense = RIGHT_BANK_SENSE // Sense input for the selected bank
		otherBatterySense = LEFT_BANK_SENSE // Left bank sense
	}

	// If the bank is already switched off then do nothing and return
	if this.FgLeft.ModbusData.Discrete[thisBatterySense] {
		return
	}

	if this.FgLeft.ModbusData.Discrete[otherBatterySense] {
		// Activate the relay to switch on the other battery if it is switched off so there is always one active bank
		this.PulseRelay(onRelay, this.FgLeft.SlaveAddress, 2)
		log.Println("OtherBatterySense (", otherBatterySense, ") shows TRUE so attempting to turn on bank by pulsing relay ", onRelay)
		time.Sleep(time.Second * 15)
		// Now switch the selected battery off giving 15 seconds for the switching capacitor to discharge properly

		if !this.FgLeft.ModbusData.Discrete[otherBatterySense] {
			this.PulseRelay(offRelay, this.FgLeft.SlaveAddress, 2)
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
		this.PulseRelay(offRelay, this.FgLeft.SlaveAddress, 2)
	}
}

/**
Return the JSON representation of the data read from the controllers
*/
func (this *FuelGauge) GetData() (string, error) {
	bytesLeftJSON, err := json.Marshal(this.FgLeft)
	if err != nil {
		return "", err
	}
	bytesRightJSON, err := json.Marshal(this.FgRight)
	if err != nil {
		return "", err
	}
	return `{"left":` + string(bytesLeftJSON) + `,"right":` + string(bytesRightJSON) + `}`, nil
}

/**
Draw the modbus coils, and registers table for display of data from one fuel gauge controller
*/
func (this *FuelGauge) drawTable(w http.ResponseWriter, slave uint8, SlaveEndPoints []ModbusEndPoint) {
	var bClosed bool
	var onClick string
	var labelClass string
	var readOnly string
	var name string
	nIndex := 0

	for _, ep := range SlaveEndPoints {
		if (nIndex % 4) == 0 {
			fmt.Fprint(w, `<tr>`)
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
		switch ep.dataType {
		case Coil:
			fmt.Fprint(w, `<td class="coil" `, onClick, `><span class="coilOff" id="c`, slave, `:`, ep.id, `">`, ep.name, `</span></td>`)
		case Discrete:
			fmt.Fprint(w, `<td class="discrete"><span class="discreteOff" id="d`, slave, `:`, ep.id, `">`, ep.name, `</span></td>`)
		case HoldingRegister:
			fmt.Fprint(w, `<td class="holdingRegister"><label for="h`, slave, ":", ep.id, `" `, labelClass, `>`, ep.name, `</label><input class="holdingRegister" type="text"`, name, ` id="h`, slave, ":", ep.id, `" multiplier="`, ep.multiplier, `" signed="`, ep.signed, `" value="" `, readOnly, `></td>`)
		case InputRegister:
			fmt.Fprint(w, `<td class="inputRegister"><label for="i`, slave, ":", ep.id, `">`, ep.name, `</label `, labelClass, `><input class="inputRegister" type="text" id="i`, slave, ":", ep.id, `" multiplier="`, ep.multiplier, `" signed="`, ep.signed, `" value="" readonly></td>`)
		case Blank:
			fmt.Fprint(w, `<td>&nbsp;</td>`)
		}
		nIndex++
		if (nIndex % 4) == 0 {
			fmt.Fprint(w, `</tr>`)
			bClosed = true
		}
	}
	if !bClosed {
		fmt.Fprint(w, "</tr>")
	}
}

/**
Render the WEB page for the fuel gauge controllers
*/
func (this *FuelGauge) WebGetValues(w http.ResponseWriter, _ *http.Request) {

	fmt.Fprint(w, `<html>
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
				if(data.lasterror != "") {
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
			document.getElementById("error`, this.FgLeft.SlaveAddress, `").innerText = "";`)
	if this.FgLeft.SlaveAddress != 0 {
		fmt.Fprint(w, `			document.getElementById("error`, this.FgLeft.SlaveAddress, `").innerText = "";`)
	}
	fmt.Fprint(w, `	}
	</script>
  </head>
  <body>
	<h1>Battery Management</h1>
    <h2>Connected on `, this.commsPort, ` at `, this.baudRate, ` baud</h2>
    <div id="leftBattery">
      <form onsubmit="return false;" id="modbus1Form">
		<input type="hidden" name="slave" value="`, this.FgLeft.SlaveAddress, `">
        <table class="pumps"><tr><td colspan=2 style="text-align:center">---Key---</td><td class="coilOn">===ON===</td><td class="coilOff">===OFF===</td></tr>`)
	this.drawTable(w, this.FgLeft.SlaveAddress, EndPoints)
	fmt.Fprint(w, `
        </table>
        <br /><button class="frmSubmit" type="text" onclick="sendFormData('modbus1Form', 'setHoldingRegisters')">Submit</button>&nbsp;<span id="error`, this.FgLeft.SlaveAddress, `"></span>
      </form>
    </div>`)
	if this.FgRight.SlaveAddress != 0 {
		fmt.Fprint(w, ` <div id="rightBattery">
      <form onsubmit="return false;" id="modbus2Form">
		<input type="hidden" name="slave" value="`, this.FgRight.SlaveAddress, `">
        <table class="pumps"><tr><td colspan=2 style="text-align:center">---Key---</td><td class="coilOn">===ON===</td><td class="coilOff">===OFF===</td></tr>`)
		this.drawTable(w, this.FgRight.SlaveAddress, EndPoints)
		fmt.Fprint(w, `
        </table>
        <br /><button class="frmSubmit" type="text" onclick="sendFormData('modbus2Form', 'setHoldingRegisters')">Submit</button>&nbsp;<span id="error`, this.FgRight.SlaveAddress, `"></span>
      </form>
    </div>`)
	}
	fmt.Fprint(w, `
    <div>
      <button class="frmSubmit" type="text" onclick="clearErrors();">Clear Errors</button>
    </div>
  </body>
</html>`)
}

/**
Perform the bank watering function. Turn on the relevant valve for the requested time in minutes.
*/
func (this *FuelGauge) WaterBank(bank uint8, timer uint8) error {
	var relay uint16
	// Solenoids are on coils 6 & 8 of the right bank controller.
	if bank == LEFT_BANK {
		relay = LEFT_WATER_RELAY
	} else {
		relay = RIGHT_WATER_RELAY
	}
	err := this.mbus.WriteCoil(relay, true, this.FgRight.SlaveAddress)
	if err == nil {
		time.AfterFunc(time.Duration(timer)*time.Minute, func() { this.mbus.WriteCoil(relay, false, this.FgRight.SlaveAddress) })
	} else {
		return err
	}
	return nil
}

/**
Turn on the watering system for the bank for the minutes.
Send PATCH to URL: /waterBank/{bank}/{minutes}
*/
func (this *FuelGauge) WebWaterBank(w http.ResponseWriter, r *http.Request) {
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

	err = this.WaterBank(uint8(bank), uint8(timer))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/**
Turn on the fan if it is off
*/
func (this *FuelGauge) TurnOnFan() {
	if !this.FgLeft.ModbusData.Coil[BATTERY_FAN_RELAY-1] {
		err := this.mbus.WriteCoil(BATTERY_FAN_RELAY, true, this.FgLeft.SlaveAddress)
		if err != nil {
			log.Println("Failed to turn the battery fan on", err)
		}
	}
}

/**
Turn off the fan if it is on
*/
func (this *FuelGauge) TurnOffFan() {
	if this.FgLeft.ModbusData.Coil[BATTERY_FAN_RELAY-1] {
		err := this.mbus.WriteCoil(BATTERY_FAN_RELAY, false, this.FgLeft.SlaveAddress)
		if err != nil {
			log.Println("Failed to turn the battery fan off", err)
		}
	}
}

/**
Turn on or off the battery house ventilation fan for the minutes.
Send PATCH to URL: /batteryFan/{onOff}
*/
func (this *FuelGauge) WebBatteryFan(w http.ResponseWriter, r *http.Request) {
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
	err := this.mbus.WriteCoil(BATTERY_FAN_RELAY, OnOff, this.FgLeft.SlaveAddress)
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
func (this *FuelGauge) WebSwitchBattery(w http.ResponseWriter, r *http.Request) {
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
			relay = LEFT_BANK_ON_RELAY
		} else {
			// If the right battery is off (discrete input 8 = 1) DO NOT turn the left bank off
			if this.FgLeft.ModbusData.Discrete[RIGHT_BANK_SENSE] {
				http.Error(w, "Cannot turn both batteries off.", http.StatusBadRequest)
				return
			}
			relay = LEFT_BANK_OFF_RELAY
		}
	case "1":
		if OnOff {
			relay = RIGHT_BANK_ON_RELAY
		} else {
			// If the left battery is off (discrete input 8 = 1) DO NOT turn the left bank off
			if this.FgLeft.ModbusData.Discrete[LEFT_BANK_SENSE] {
				http.Error(w, "Cannot turn both batteries off.", http.StatusBadRequest)
				return
			}
			relay = RIGHT_BANK_OFF_RELAY
		}
	default:
		http.Error(w, "bank 0(left) or 1(right) expected", http.StatusBadRequest)
		return
	}

	// Activate the relay to switch the battery
	err := this.mbus.WriteCoil(relay, true, this.FgLeft.SlaveAddress)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	time.Sleep(time.Second * 2)
	err = this.mbus.WriteCoil(relay, false, this.FgLeft.SlaveAddress)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
}

/**
Get the charging efficiences from the system parameters
*/
func (this *FuelGauge) getChargingEfficiencies(db *sql.DB) error {
	rows, err := db.Query("select name, double_value from system_parameters where name like 'charge__efficiency' order by name")
	if err != nil {
		db.Close()
		return err
	} else {
		for rows.Next() {
			var name string
			var efficiency float64
			err = rows.Scan(&name, &efficiency)
			if err != nil {
				db.Close()
				return err
			}
			if name == "charge0_efficiency" {
				this.FgLeft.Efficiency = efficiency
			} else if name == "charge1_efficiency" {
				this.FgRight.Efficiency = efficiency
			} else {
				db.Close()
				return errors.New("Unknown entry in system parameters found matching query 'charge__efficiency'")
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
			var when mysql.NullTime
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

func (this *FuelGauge) StateOfCharge() float32 {
	return ((this.FgLeft.Coulombs + this.FgRight.Coulombs) * 100) / float32((this.FgLeft.Capacity + this.FgRight.Capacity))
}

func (this *FuelGauge) StateOfChargeLeft() float32 {
	return ((this.FgLeft.Coulombs) * 100) / float32((this.FgLeft.Capacity))
}

func (this *FuelGauge) StateOfChargeRight() float32 {
	return ((this.FgRight.Coulombs) * 100) / float32((this.FgRight.Capacity))
}

func (this *FuelGauge) Current() float32 {
	return float32((int16(this.FgLeft.ModbusData.Input[AVG_CURRENT-1]) + int16(this.FgRight.ModbusData.Input[AVG_CURRENT-1]))) / 100.0
}
