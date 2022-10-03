package main

import (
	ModbusController "BatteryMonitor6813V4/ModbusBatteryFuelGauge/modbusController"
	websocket "BatteryMonitor6813V4/ModbusBatteryFuelGauge/webSocket"

	//	"CanMessages/CAN_010"
	//	"CanMessages/CAN_305"
	//	"CanMessages/CAN_306"
	//	"CanMessages/CAN_307"
	"BatteryMonitor6813V4/ModbusBatteryFuelGauge/Data"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var CommsPort = flag.String("Port", "/dev/tty.usbserial-1424440", "communication port")
var BaudRate = flag.Int("Baudrate", 19200, "communication port baud rate")
var DataBits = flag.Int("Databits", 8, "communication port data bits")
var StopBits = flag.Int("Stopbits", 2, "communication port stop bits")
var Parity = flag.String("Parity", "N", "communication port parity")
var TimeoutMilliSecs = flag.Int("Timeout", 200, "communication port timeout in milliseconds")
var Slave1Address = flag.Int("Slave1", 1, "Modbus slave1 ID")
var Slave2Address = flag.Int("Slave2", 0, "Modbus slave2 ID (0 = not present)")
var webPort = flag.String("webport", "8085", "port number for the WEB interface")
var verbose = flag.Bool("Verbose", false, "Set to true if you want printed messages on the terminal.")
var pDatabaseLogin = flag.String("dblogin", "pi", "Database login ID")
var pDatabasePassword = flag.String("dbpassword", "7444561", "Database login Password")
var pDatabaseServer = flag.String("dbserver", "localhost", "Database server")
var pDatabasePort = flag.String("dbport", "3306", "Database port")
var pDatabaseName = flag.String("database", "battery", "Database Name")
var pDB *sql.DB
var mbus *ModbusController.ModbusController
var currentStatement *sql.Stmt
var systemParamStatement *sql.Stmt
var lastCoulombCount struct {
	count_0      uint16
	count_1      uint16
	efficiency_0 float64
	efficiency_1 float64
	when         time.Time
}

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

/*
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
*/

var EndPoints = []ModbusEndPoint{
	{"1", "Current", 1, InputRegister, 10, "A", false, true},
	{"2", "Voltage", 2, InputRegister, 100, "V", false, false},
	{"3", "Temperature", 3, InputRegister, 10, "C", false, false},
	{"4", "Status", 4, InputRegister, 1, "", false, false},
	{"5", "LastError", 5, InputRegister, 1, "", false, false},
	{"6", "Instantaneous Current", 6, InputRegister, 1, "A", false, true},
	{"x1", "", 0, Blank, 0, "", false, false},
	{"x2", "", 0, Blank, 0, "", false, false},
	{"7", "Analog In 1", 7, InputRegister, 1, "V", false, true},
	{"8", "Analog In 2", 8, InputRegister, 1, "V", false, true},
	{"9", "Analog In 3", 9, InputRegister, 1, "V", false, true},
	{"10", "Analog In 4", 10, InputRegister, 1, "V", false, true},
	{"11", "Analog In 5", 11, InputRegister, 1, "V", false, true},
	{"12", "Analog In 6", 12, InputRegister, 1, "V", false, true},
	{"13", "Analog In 7", 13, InputRegister, 1, "V", false, true},
	{"14", "Analog In 8", 14, InputRegister, 1, "V", false, true},
	{"1", "Slave ID", 1, HoldingRegister, 1, "", true, false},
	{"2", "Baud Rate", 2, HoldingRegister, 1, "", true, false},
	{"3", "Charge", 3, HoldingRegister, 1, "Ahr", true, false},
	{"x3", "", 0, Blank, 0, "", false, false},
	{"1", "Relay 1", 1, Coil, 1, "", true, false},
	{"2", "Relay 2", 2, Coil, 1, "", true, false},
	{"3", "Relay 3", 3, Coil, 1, "", true, false},
	{"4", "Relay 4", 4, Coil, 1, "", true, false},
	{"5", "Relay 5", 5, Coil, 1, "", true, false},
	{"6", "Relay 6", 6, Coil, 1, "", true, false},
	{"7", "Relay 7", 7, Coil, 1, "", true, false},
	{"8", "Relay 8", 8, Coil, 1, "", true, false},
	{"1", "Under Voltage", 1, Discrete, 1, "", false, false},
	{"2", "Voltage Alert", 2, Discrete, 1, "", false, false},
	{"3", "Charge Low", 3, Discrete, 1, "", false, false},
	{"4", "Charge High", 4, Discrete, 1, "", false, false},
	{"5", "Temperature", 5, Discrete, 1, "", false, false},
	{"6", "Charge Overflow", 6, Discrete, 1, "", false, false},
	{"7", "Current Alert", 7, Discrete, 1, "", false, false},
	{"x4", "", 0, Blank, 0, "", false, false},
	{"8", "Digital In 1", 8, Discrete, 1, "", false, false},
	{"9", "Digital In 2", 9, Discrete, 1, "", false, false},
	{"10", "Digital In 3", 10, Discrete, 1, "", false, false},
	{"11", "Digital In 4", 11, Discrete, 1, "", false, false},
	{"12", "Digital In 5", 12, Discrete, 1, "", false, false},
	{"13", "Digital In 6", 13, Discrete, 1, "", false, false},
	{"14", "Digital In 7", 14, Discrete, 1, "", false, false},
	{"15", "Digital In 8", 15, Discrete, 1, "", false, false},
}

var pool *websocket.Pool

const (
	Coil = iota
	Discrete
	InputRegister
	HoldingRegister
	Blank // Allows blank entries to be placed
)

var lastSlave1Data *Data.Data
var lastSlave2Data *Data.Data

func setHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "PATCH, GET, OPTIONS")
	w.Header().Set("Access-Control-Expose-Headers", "Authorization")
}

func webToggleCoil(w http.ResponseWriter, r *http.Request) {
	address := r.FormValue("coil")
	var dataPointer *Data.Data

	setHeaders(w)
	slaveVal, _ := strconv.ParseUint(r.FormValue("slave"), 10, 16)
	if *verbose {
		fmt.Println("Toggle coil ", address, " on slave ", slaveVal)
	}
	if uint8(slaveVal) == (lastSlave1Data.SlaveAddress) {
		dataPointer = lastSlave1Data
	} else {
		dataPointer = lastSlave2Data
	}
	// coil
	n, _ := strconv.ParseUint(address, 10, 16)

	nIndex := uint16(n)
	nIndex = nIndex - dataPointer.CoilStart() // nIndex is now 0 based
	if *verbose {
		if dataPointer.Coil[nIndex] {
			fmt.Println("Setting coil ", nIndex, " on slave ", slaveVal, " to false")
		} else {
			fmt.Println("Setting coil ", nIndex, " on slave ", slaveVal, " to true")
		}
	}
	err := mbus.WriteCoil(uint16(nIndex)+dataPointer.CoilStart(), !dataPointer.Coil[nIndex], dataPointer.SlaveAddress)
	if err != nil {
		dataPointer.LastError = err.Error()
	}
	w.Header().Set("Cache-Control", "no-store")
	if _, err := fmt.Fprint(w, "Coil ", nIndex+dataPointer.CoilStart(), " on slave ", dataPointer.SlaveAddress, " toggled."); err != nil {
		log.Println(err)
	}
	getValues(true, dataPointer, mbus)
}

func webProcessHoldingRegistersForm(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	var dataPointer *Data.Data

	setHeaders(w)
	slaveVal, _ := strconv.ParseUint(r.FormValue("slave"), 10, 16)
	if uint8(slaveVal) == (lastSlave1Data.SlaveAddress) {
		dataPointer = lastSlave1Data
	} else {
		dataPointer = lastSlave2Data
	}
	if err != nil {
		if _, err := fmt.Fprint(w, `<html><head><title>Error</title></head><body><h1>`, err, `</h1></body></html>`); err != nil {
			log.Println(err)
		}
	}
	for sKey, sValue := range r.Form {
		nValue, _ := strconv.ParseUint(sValue[0], 10, 16)

		for _, ep := range EndPoints {
			if (ep.id == sKey) && (ep.dataType == HoldingRegister) {
				if *verbose {
					if _, err := fmt.Println("Writing ", nValue, " to slave ", dataPointer.SlaveAddress, " modbus register ", ep.address); err != nil {
						log.Println(err)
					}
				}
				err = mbus.WriteHoldingRegister(uint16(ep.address), uint16(nValue), dataPointer.SlaveAddress)
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

func startDataWebSocket(pool *websocket.Pool, w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	if *verbose {
		fmt.Println("WebSocket Endpoint Hit")
	}
	conn, err := websocket.Upgrade(w, r)
	if err != nil {
		if _, err := fmt.Fprintf(w, "%+v\n", err); err != nil {
			log.Println(err)
		}
	}
	client := &websocket.Client{
		Conn: conn,
		Pool: pool,
	}
	pool.Register <- client
	if *verbose {
		if _, err := fmt.Println("New client so force an update."); err != nil {
			log.Println(err)
		}
	}
	getValues(true, lastSlave1Data, mbus)
	getValues(true, lastSlave2Data, mbus)
}

func getValues(bRefresh bool, lastValues *Data.Data, p *ModbusController.ModbusController) {
	if *verbose {
		if _, err := fmt.Println("Get Values called for slave ", lastValues.SlaveAddress); err != nil {
			log.Println(err)
		}
		if bRefresh {
			if _, err := fmt.Println("Forced client refresh."); err != nil {
				log.Println(err)
			}
		}
	}
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
	if bRefresh || !newValues.Compare(lastValues) {
		lastValues.Update(newValues)
		bytes, err := json.Marshal(lastValues)
		if err != nil {
			log.Print("Error marshalling the data - ", err)
		} else {
			select {
			case pool.Broadcast <- bytes:
				if *verbose {
					if _, err := fmt.Println("Broadcasting data"); err != nil {
						log.Println(err)
					}
				}
			default:
				if *verbose {
					if _, err := fmt.Println("Channel would block!"); err != nil {
						log.Println(err)
					}
				}
			}
		}
	}
	if *verbose {
		if _, err := fmt.Println("Done (", lastValues.SlaveAddress, ")."); err != nil {
			log.Println(err)
		}
	}
}

func reportValues() {
	for {
		lastSlave1Data.LastError = ""
		getValues(false, lastSlave1Data, mbus)
		if lastSlave2Data.SlaveAddress != 0 {
			lastSlave2Data.LastError = ""
			getValues(false, lastSlave2Data, mbus)
		}
		go logValues(lastSlave1Data.Input[0], lastSlave2Data.Input[0], lastSlave1Data.Holding[2], lastSlave2Data.Holding[2])
		time.Sleep(time.Second)
	}
}

func logValues(current_0 uint16, current_1 uint16, charge_0 uint16, charge_1 uint16) {
	i0 := float32(current_0)
	i1 := float32(current_1)

	if i0 > 32767 {
		i0 = i0 - 65536
	}
	if i1 > 32767 {
		i1 = i1 - 65536
	}
	_, err := currentStatement.Exec(i0/10, i1/10, charge_0, charge_1)
	if err != nil {
		log.Println(err)
	}

	delta := int(charge_0) - int(lastCoulombCount.count_0)
	// Take care of overflow of the coulomb counter
	if delta > 65000 {
		delta = delta - 65536
	} else if delta < -65000 {
		delta = delta + 65536
	}
	if delta > 0 {
		if *verbose {
			fmt.Println("Adding ", float64(delta)*lastCoulombCount.efficiency_0)
		}
		_, err = systemParamStatement.Exec(float64(delta)*lastCoulombCount.efficiency_0, "charge_in_counter_0")
		if err != nil {
			log.Println("Adding charge to bank 0 - ", err)
		}
	} else if delta < 0 {
		_, err = systemParamStatement.Exec(0-delta, "charge_out_counter_0")
		if err != nil {
			log.Println("Removing charge to bank 0 - ", err)
		}
	}
	lastCoulombCount.count_0 = charge_0
	delta = int(charge_1) - int(lastCoulombCount.count_1)
	if delta > 0 {
		_, err = systemParamStatement.Exec(float64(delta)*lastCoulombCount.efficiency_1, "charge_in_counter_1")
		if err != nil {
			log.Println("Adding charge to bank 1 - ", err)
		}
	} else if delta < 0 {
		_, err = systemParamStatement.Exec(0-delta, "charge_out_counter_1")
		if err != nil {
			log.Println("Removing charge to bank 1 - ", err)
		}
	}
	lastCoulombCount.count_1 = charge_1
}

func drawTable(w http.ResponseWriter, slave uint8, SlaveEndPoints []ModbusEndPoint) {
	var bClosed bool
	var onClick string
	var labelClass string
	var readOnly string
	var name string
	nIndex := 0

	setHeaders(w)
	for _, ep := range SlaveEndPoints {
		if (nIndex % 4) == 0 {
			if _, err := fmt.Fprint(w, `<tr>`); err != nil {
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
		switch ep.dataType {
		case Coil:
			if _, err := fmt.Fprint(w, `<td class="coil" `, onClick, `><span class="coilOff" id="c`, slave, `:`, ep.id, `">`, ep.name, `</span></td>`); err != nil {
				log.Println(err)
			}
		case Discrete:
			if _, err := fmt.Fprint(w, `<td class="discrete"><span class="discreteOff" id="d`, slave, `:`, ep.id, `">`, ep.name, `</span></td>`); err != nil {
				log.Println(err)
			}
		case HoldingRegister:
			if _, err := fmt.Fprint(w, `<td class="holdingRegister"><label for="h`, slave, ":", ep.id, `" `, labelClass, `>`, ep.name, `</label><input class="holdingRegister" type="text"`, name, ` id="h`, slave, ":", ep.id, `" multiplier="`, ep.multiplier, `" signed="`, ep.signed, `" value="" `, readOnly, `></td>`); err != nil {
				log.Println(err)
			}
		case InputRegister:
			if _, err := fmt.Fprint(w, `<td class="inputRegister"><label for="i`, slave, ":", ep.id, `">`, ep.name, `</label `, labelClass, `><input class="inputRegister" type="text" id="i`, slave, ":", ep.id, `" multiplier="`, ep.multiplier, `" signed="`, ep.signed, `" value="" readonly></td>`); err != nil {
				log.Println(err)
			}
		case Blank:
			if _, err := fmt.Fprint(w, `<td>&nbsp;</td>`); err != nil {
				log.Println(err)
			}
		}
		nIndex++
		if (nIndex % 4) == 0 {
			if _, err := fmt.Fprint(w, `</tr>`); err != nil {
				log.Println(err)
			}
			bClosed = true
		}
	}
	if !bClosed {
		if _, err := fmt.Fprint(w, "</tr>"); err != nil {
			log.Println(err)
		}
	}
}

func webGetValues(w http.ResponseWriter, _ *http.Request) {
	setHeaders(w)
	if _, err := fmt.Fprint(w, `<html>
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
      input.holdingRegister{padding: 6px 1px;margin: 3px 0;box-sizing: border-box;border: 1px solid blue;background-color: #5CADEC;color: white;}
      input.inputRegister{padding: 6px 1px;margin: 3px 0;box-sizing: border-box;border: none;background-color: gainsboro;color: black;}
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
				slave = data.slave;
					data.discrete.forEach(function (e, i) {setDiscrete(e, i, slave);});
				data.coil.forEach(function (e, i) {setCoil(e, i, slave);});
				data.holding.forEach(function (e, i) {setHoldingReg(e, i, slave);});
				data.input.forEach(function (e, i) {setInputReg(e, i, slave);});
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
			document.getElementById("error`, *Slave1Address, `").innerText = "";`); err != nil {
		log.Println(err)
	}
	if lastSlave2Data.SlaveAddress != 0 {
		if _, err := fmt.Fprint(w, `			document.getElementById("error`, *Slave2Address, `").innerText = "";`); err != nil {
			log.Println(err)
		}
	}
	if _, err := fmt.Fprint(w, `	}
	</script>
  </head>
  <body>
	<h1>Battery Management</h1>
    <h2>Connected on `, *CommsPort, ` at `, *BaudRate, ` baud</h2>
    <div id="leftBattery">
      <form onsubmit="return false;" id="modbus1Form">
		<input type="hidden" name="slave" value="`, *Slave1Address, `">
        <table class="pumps"><tr><td colspan=2 style="text-align:center">---Key---</td><td class="coilOn">===ON===</td><td class="coilOff">===OFF===</td></tr>`); err != nil {
		log.Println(err)
	}
	drawTable(w, (uint8)(*Slave1Address), EndPoints)
	if _, err := fmt.Fprint(w, `
        </table>
        <br /><button class="frmSubmit" type="text" onclick="sendFormData('modbus1Form', 'setHoldingRegisters')">Submit</button>&nbsp;<span id="error`, *Slave1Address, `"></span>
      </form>
    </div>`); err != nil {
		log.Println(err)
	}
	if lastSlave2Data.SlaveAddress != 0 {
		if _, err := fmt.Fprint(w, ` <div id="rightBattery">
      <form onsubmit="return false;" id="modbus2Form">
		<input type="hidden" name="slave" value="`, *Slave2Address, `">
        <table class="pumps"><tr><td colspan=2 style="text-align:center">---Key---</td><td class="coilOn">===ON===</td><td class="coilOff">===OFF===</td></tr>`); err != nil {
			log.Println(err)
		}
		drawTable(w, (uint8)(*Slave2Address), EndPoints)
		if _, err := fmt.Fprint(w, `
        </table>
        <br /><button class="frmSubmit" type="text" onclick="sendFormData('modbus2Form', 'setHoldingRegisters')">Submit</button>&nbsp;<span id="error`, *Slave2Address, `"></span>
      </form>
    </div>`); err != nil {
			log.Println(err)
		}
	}
	if _, err := fmt.Fprint(w, `
    <div>
      <button class="frmSubmit" type="text" onclick="clearErrors();">Clear Errors</button>
    </div>
  </body>
</html>`); err != nil {
		log.Println(err)
	}
}

func webWaterBank(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	setHeaders(w)
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

	err = mbus.WriteCoil(uint16(bank)+7, true, uint8(*Slave2Address))
	if err == nil {
		time.AfterFunc(time.Duration(timer)*time.Minute, func() {
			if err := mbus.WriteCoil(uint16(bank)+7, false, uint8(*Slave2Address)); err != nil {
				log.Println(err)
			}
		})
	} else {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func webBatteryFan(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var OnOff bool

	setHeaders(w)
	if strings.EqualFold(vars["onOff"], "on") {
		OnOff = true
	} else if strings.EqualFold(vars["onOff"], "off") {
		OnOff = false
	} else {
		http.Error(w, "On or Off expected", http.StatusBadRequest)
		return
	}
	err := mbus.WriteCoil(8, OnOff, uint8(*Slave1Address))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

/**
Switch one of the battery banks on or off.
/batterySwitch/{bank}/{onOff}
Slave1 - Relay 1 = On/Off connects one or the other coil to supply (24V)
Slave1 - Relay 2 = Enable relay change bank 0 connects common of left bank relay to supply (0V)
Slave1 - Relay 3 = Enable relay change bank 1 connects common of right bank relay to supply (0V)
*/
func webSwitchBattery(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var OnOff bool

	setHeaders(w)
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
			relay = 1
		} else {
			relay = 2
		}
	case "1":
		if OnOff {
			relay = 3
		} else {
			relay = 4
		}
	default:
		http.Error(w, "bank 0 or 1 expected", http.StatusBadRequest)
		return
	}

	// Activate the relay to switch the battery
	err := mbus.WriteCoil(relay, true, uint8(*Slave1Address))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	time.Sleep(time.Second * 2)
	err = mbus.WriteCoil(relay, false, uint8(*Slave1Address))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
}

func webOptionsHandler(w http.ResponseWriter, _ *http.Request) {
	setHeaders(w)
	w.WriteHeader(http.StatusOK)
}

func setUpWebSite() {
	pool = websocket.NewPool()
	go pool.Start()

	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/", webGetValues).Methods("GET")
	router.PathPrefix("/").Methods("OPTIONS").HandlerFunc(webOptionsHandler)
	router.HandleFunc("/toggleCoil", webToggleCoil).Methods("PATCH")
	router.HandleFunc("/setHoldingRegisters", webProcessHoldingRegistersForm).Methods("POST")
	router.HandleFunc("/waterBank/{bank}/{minutes}", webWaterBank).Methods("PATCH")
	router.HandleFunc("/batteryFan/{onOff}", webBatteryFan).Methods("PATCH")
	router.HandleFunc("/batterySwitch/{bank}/{onOff}", webSwitchBattery).Methods("PATCH")
	router.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		startDataWebSocket(pool, w, r)
	}).Methods("GET")
	var sWebAddress string = ":" + *webPort
	log.Fatal(http.ListenAndServe(sWebAddress, router))
}

func getChargingEfficiencies(db *sql.DB) error {
	rows, err := db.Query("select name, double_value from system_parameters where name like 'charge__efficiency' order by name")
	if err != nil {
		if err := db.Close(); err != nil {
			log.Println(err)
		}
		return err
	} else {
		for rows.Next() {
			var name string
			var efficiency float64
			err = rows.Scan(&name, &efficiency)
			if err != nil {
				if err := db.Close(); err != nil {
					log.Println(err)
				}
				return err
			}
			if name == "charge0_efficiency" {
				lastCoulombCount.efficiency_0 = efficiency
			} else if name == "charge1_efficiency" {
				lastCoulombCount.efficiency_1 = efficiency
			} else {
				if err := db.Close(); err != nil {
					log.Println(err)
				}
				return errors.New("Unknown entry in system parameters found matching query 'charge__efficiency'")
			}
		}
	}
	return nil
}

func connectToDatabase() (*sql.DB, error) {
	if pDB != nil {
		_ = pDB.Close()
		pDB = nil
	}
	var sConnectionString = *pDatabaseLogin + ":" + *pDatabasePassword + "@tcp(" + *pDatabaseServer + ":" + *pDatabasePort + ")/" + *pDatabaseName

	if *verbose {
		fmt.Println("Connecting to [", sConnectionString, "]")
	}
	db, err := sql.Open("mysql", sConnectionString)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	// Prepare the insert statements for current and charge
	sSQL := `insert into current (logged, channel_0, channel_1, level_of_charge_0, level_of_charge_1)
                          values (now(),?,?,?,?)`
	currentStatement, err = db.Prepare(sSQL)
	if err != nil {
		if err := db.Close(); err != nil {
			log.Println(err)
		}
		return nil, err
	}
	sSQL = `update system_parameters set double_value = double_value + ? where name = ?`
	systemParamStatement, err = db.Prepare(sSQL)
	if err != nil {
		if err := db.Close(); err != nil {
			log.Println(err)
		}
		return nil, err
	}
	rows, err := db.Query("select logged, round(level_of_charge_0), round(level_of_charge_1) from current order by logged desc limit 1")
	if err != nil {
		if err := db.Close(); err != nil {
			log.Println(err)
		}
		return nil, err
	} else {
		lastCoulombCount.count_0 = 0
		lastCoulombCount.count_1 = 0
		lastCoulombCount.when = time.Now()
		for rows.Next() {
			var count0 uint16
			var count1 uint16
			var when sql.NullTime
			err = rows.Scan(&when, &count0, &count1)
			if err != nil {
				if err := db.Close(); err != nil {
					log.Println(err)
				}
				return nil, err
			}
			lastCoulombCount.count_0 = count0
			lastCoulombCount.count_1 = count1
			lastCoulombCount.when = when.Time
		}
	}
	return db, getChargingEfficiencies(db)
}

func init() {
	var err error
	flag.Parse()
	lastSlave1Data = Data.New(8, 1, 15, 1, 14, 1, 3, 1, (uint8)(*Slave1Address))
	lastSlave2Data = Data.New(8, 1, 15, 1, 14, 1, 3, 1, (uint8)(*Slave2Address))
	// Set up the database connection
	pDB, err = connectToDatabase()
	if err != nil {
		log.Fatalf("Failed to connect to to the database - %s - Sorry, I am giving up.", err)
	}
}

func main() {
	mbus = ModbusController.New(*CommsPort, *BaudRate, *DataBits, *StopBits, *Parity, time.Duration(*TimeoutMilliSecs)*time.Millisecond)
	if mbus != nil {
		defer mbus.Close()

		err := mbus.Connect()
		if err != nil {
			if _, err := fmt.Println("Port = ", *CommsPort, " baudrate = ", *BaudRate); err != nil {
				log.Println(err)
			}
			panic(err)
		}
		if *verbose {
			if _, err := fmt.Println("Connected to heat pump on ", *CommsPort, " at ", *BaudRate, "baud ", *DataBits, " data ", *StopBits, " Stop ", *Parity, "Parity "); err != nil {
				log.Println(err)
			}
		}
	}
	// Start the reporting loop
	go reportValues()
	setUpWebSite()
}
