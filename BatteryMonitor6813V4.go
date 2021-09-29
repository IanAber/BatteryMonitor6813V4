package main

import (
	"BatteryMonitor6813V4/FuelGauge"
	"BatteryMonitor6813V4/FullChargeEvaluator"
	"CanMessages/CAN_010"
	"CanMessages/CAN_305"
	"CanMessages/CAN_306"
	"CanMessages/CAN_307"
	"CanMessages/CAN_351"
	"CanMessages/CAN_355"
	"CanMessages/CAN_356"
	"CanMessages/CAN_35E"
	"LTC6813/LTC6813"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/brutella/can"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"log"
	"log/syslog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/conn/spi"
	"periph.io/x/periph/conn/spi/spireg"
	"periph.io/x/periph/host"
	"strconv"
	"sync"
	"time"
)

const SPIBAUDRATE = physic.MegaHertz * 1
const SPIBITSPERWORD = 8

type InverterValues struct {
	Volts          float32 `json:"volts"`
	Amps           float32 `json:"amps"`
	Soc            float32 `json:"soc"`
	Vsetpoint      float32 `json:"vsetpoint"`
	Frequency      float64 `json:"frequency"`
	IMax           float32 `json:"iMax"`
	OnRelay1       bool    `json:"relay1"`
	OnRelay2       bool    `json:"relay2"`
	OnRelay1Slave1 bool    `json:"relay1slave1"`
	OnRelay2Slave1 bool    `json:"relay2slave1"`
	OnRelay1Slave2 bool    `json:"relay1slave2"`
	OnRelay2Slave2 bool    `json:"relay2slave2"`
	GnRun          bool    `json:"gnrun"`
	GnRunSlave1    bool    `json:"gnrunslave1"`
	GnRunSlave2    bool    `json:"gnrunslave2"`
	AutoGn         bool    `json:"autogn"`
	AutoLodExt     bool    `json:"autolodext"`
	AutoLodSoc     bool    `json:"autolodsoc"`
	Tm1            bool    `json:"tm1"`
	Tm2            bool    `json:"tm2"`
	ExtPwrDer      bool    `json:"extpwrder"`
	ExtVfOk        bool    `json:"extvfok"`
	GdOn           bool    `json:"gdon"`
	Errror         bool    `json:"error"`
	Run            bool    `json:"run"`
	BatFan         bool    `json:"batfan"`
	AcdCir         bool    `json:"acdcir"`
	MccBatFan      bool    `json:"mccbatfan"`
	MccAutoLod     bool    `json:"mccautoload"`
	Chp            bool    `json:"chp"`
	ChpAdd         bool    `json:"chpadd"`
	SiComRemote    bool    `json:"sicomremote"`
	OverLoad       bool    `json:"overload"`
	ExtSrcConn     bool    `json:"extsrcconn"`
	Silent         bool    `json:"silent"`
	Current        bool    `json:"current"`
	FeedSelfC      bool    `json:"feedselfc"`
	Esave          bool    `json:"esave"`
	mu             sync.Mutex
	Log            bool `json:"-"`
}

type InverterSetpoints struct {
	VSetpoint         float32 `json:"v_setpoint"`          // Current Setpoint for the Inverter battery voltage
	ISetpoint         float32 `json:"i_setpoint"`          // Current Setpoint for the Inverter battery current
	VDischarge        float32 `json:"v_discharge"`         // Setpoint for minimum discharge voltage
	IDischarge        float32 `json:"i_discharge"`         // Setpoint for maximum discharge current
	VTargetSetpoint   float32 `json:"v_target_setpoint"`   // Voltage we should get to
	ITargetSetpoint   float32 `json:"i_target_setpoint"`   // Current we should get to
	VChargingSetpoint float32 `json:"v_charging_setpoint"` // Voltage for normal charging
	IChargingSetpoint float32 `json:"i_charging_setpoint"` // Current for normal charging
	VChargedSetpoint  float32 `json:"v_charged_setpoint"`  // Voltage for fully charged
	IChargedSetpoint  float32 `json:"i_charged_setpoint"`  // Current for fully charged
}

var (
	ltc                  *LTC6813.LTC6813
	fuelgauge            *FuelGauge.FuelGauge
	spiConnection        spi.Conn
	verbose              *bool
	spiDevice            *string
	nErrors              int
	pDB                  *sql.DB
	pDatabaseLogin       *string
	pDatabasePassword    *string
	pDatabaseServer      *string
	pDatabasePort        *string
	pDatabaseName        *string
	ltcLock              sync.Mutex
	nDevices             int
	voltageStatement     *sql.Stmt
	temperatureStatement *sql.Stmt
	evaluator            *FullChargeEvaluator.FullChargeEval
	iValues              InverterValues
	autoFan              bool
	signal               *sync.Cond
	setpoints            InverterSetpoints
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:    1024,
	WriteBufferSize:   1024,
	EnableCompression: true,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// spaHandler implements the http.Handler interface, so we can use it
// to respond to HTTP requests. The path to the static directory and
// path to the index file within that static directory are used to
// serve the SPA in the given static directory.
type spaHandler struct {
	staticPath string
	indexPath  string
}

// ServeHTTP inspects the URL path to locate a file within the static dir
// on the SPA handler. If a file is found, it will be served. If not, the
// file located at the index path on the SPA handler will be served. This
// is suitable behavior for serving an SPA (single page application).
func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the absolute path to prevent directory traversal
	path, err := filepath.Abs(r.URL.Path)
	if err != nil {
		// if we failed to get the absolute path respond with a 400 bad request
		// and stop
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// prepend the path with the path to the static directory
	path = filepath.Join(h.staticPath, path)

	// check whether a file exists at the given path
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		// file does not exist, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if err != nil {
		// if we got an error (that wasn't that the file doesn't exist) stating the
		// file, return a 500 internal server error and stop
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// otherwise, use http.FileServer to serve the static dir
	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

func setHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "PATCH, GET, OPTIONS")
	w.Header().Set("Access-Control-Expose-Headers", "Authorization")
}

func webOptionsHandler(w http.ResponseWriter, _ *http.Request) {
	setHeaders(w)
	w.WriteHeader(http.StatusOK)
}

/**
Set up the LTC6813 chain.
*/
func getLTC6813(devices int) (int, error) {
	ltcLock.Lock()
	defer ltcLock.Unlock()
	ltc = LTC6813.New(spiConnection, devices)
	if err := ltc.Initialise(); err != nil {
		//		fmt.Print(err)
		log.Fatal(err)
	}
	_, err := ltc.MeasureVoltages()
	if err != nil {
		log.Println("MeasureVoltages - ", err)
	}
	_, err = ltc.MeasureTemperatures()
	if err != nil {
		log.Println("MeasureTemperatures - ", err)
	}
	return devices, nil
}

/**
Get the voltage and temperature measurements from the LTC6813 chain
*/
func performMeasurements() {
	var err error
	if nDevices == 0 {
		nDevices, err = getLTC6813(6)
		if err != nil {
			if *verbose {
				fmt.Print(err)
			}
			log.Println(err)
			nErrors++
			return
		}
	}
	if nDevices == 0 {
		if *verbose {
			fmt.Printf("\033cNo devices found on %s - %s\n", *spiDevice, time.Now().Format("15:04:05.99"))
		}
		log.Printf("\033cNo devices found on %s - %s", *spiDevice, time.Now().Format("15:04:05.99"))
		return
	}
	if *verbose {
		fmt.Println("Measuring voltages")
	}
	_, err = ltc.MeasureVoltagesSC()
	if err != nil {
		// Retry if it failed and ignore the failure if the retry was successful
		_, err = ltc.MeasureVoltagesSC()
	}
	if err != nil {
		if *verbose {
			fmt.Print(" Error measuring voltages - ", err)
		}
		log.Print(" Error measuring voltages - ", err)
		nDevices = 0
		nErrors++
	} // else {
	if *verbose {
		fmt.Println("Measuring Temperatures")
	}
	_, err = ltc.MeasureTemperatures()
	if err != nil {
		_, err = ltc.MeasureTemperatures()
	}
	if err != nil {
		if *verbose {
			fmt.Print(" Error measuring temperatures - ", err)
		}
		log.Print(" Error measuring temperatures - ", err)
		nDevices = 0
		nErrors++
	}
	//	}
	signal.Broadcast() // Tell the world we have data now
}

/**
Log the LTC6813 data to the database
*/
func logData() {
	_, err := voltageStatement.Exec(ltc.GetRawVolts(0, 0), ltc.GetRawVolts(0, 1), ltc.GetRawVolts(0, 2), ltc.GetRawVolts(0, 3), ltc.GetRawVolts(0, 4), ltc.GetRawVolts(0, 5),
		ltc.GetRawVolts(0, 6), ltc.GetRawVolts(0, 7), ltc.GetRawVolts(0, 8), ltc.GetRawVolts(0, 9), ltc.GetRawVolts(0, 10), ltc.GetRawVolts(0, 11),
		ltc.GetRawVolts(0, 12), ltc.GetRawVolts(0, 13), ltc.GetRawVolts(0, 14), ltc.GetRawVolts(0, 15), ltc.GetRawVolts(0, 16), ltc.GetRawVolts(0, 17),
		ltc.GetRawVolts(1, 0), ltc.GetRawVolts(1, 1), ltc.GetRawVolts(1, 2), ltc.GetRawVolts(1, 3), ltc.GetRawVolts(1, 4), ltc.GetRawVolts(1, 5),
		ltc.GetRawVolts(1, 6), ltc.GetRawVolts(1, 7), ltc.GetRawVolts(1, 8), ltc.GetRawVolts(1, 9), ltc.GetRawVolts(1, 10), ltc.GetRawVolts(1, 11),
		ltc.GetRawVolts(1, 12), ltc.GetRawVolts(1, 13), ltc.GetRawVolts(1, 14), ltc.GetRawVolts(1, 15), ltc.GetRawVolts(1, 16), ltc.GetRawVolts(1, 17),
		ltc.GetRawVolts(2, 0), ltc.GetRawVolts(2, 1),
		ltc.GetRawVolts(3, 0), ltc.GetRawVolts(3, 1), ltc.GetRawVolts(3, 2), ltc.GetRawVolts(3, 3), ltc.GetRawVolts(3, 4), ltc.GetRawVolts(3, 5),
		ltc.GetRawVolts(3, 6), ltc.GetRawVolts(3, 7), ltc.GetRawVolts(3, 8), ltc.GetRawVolts(3, 9), ltc.GetRawVolts(3, 10), ltc.GetRawVolts(3, 11),
		ltc.GetRawVolts(3, 12), ltc.GetRawVolts(3, 13), ltc.GetRawVolts(3, 14), ltc.GetRawVolts(3, 15), ltc.GetRawVolts(3, 16), ltc.GetRawVolts(3, 17),
		ltc.GetRawVolts(4, 0), ltc.GetRawVolts(4, 1), ltc.GetRawVolts(4, 2), ltc.GetRawVolts(4, 3), ltc.GetRawVolts(4, 4), ltc.GetRawVolts(4, 5),
		ltc.GetRawVolts(4, 6), ltc.GetRawVolts(4, 7), ltc.GetRawVolts(4, 8), ltc.GetRawVolts(4, 9), ltc.GetRawVolts(4, 10), ltc.GetRawVolts(4, 11),
		ltc.GetRawVolts(4, 12), ltc.GetRawVolts(4, 13), ltc.GetRawVolts(4, 14), ltc.GetRawVolts(4, 15), ltc.GetRawVolts(4, 16), ltc.GetRawVolts(4, 17),
		ltc.GetRawVolts(5, 0), ltc.GetRawVolts(5, 1),
		uint16(((ltc.GetSumOfCellsVolts(0))+(ltc.GetSumOfCellsVolts(1))+(ltc.GetVolts(2, 0))+(ltc.GetVolts(2, 1)))*10),
		uint16(((ltc.GetSumOfCellsVolts(3))+(ltc.GetSumOfCellsVolts(4))+(ltc.GetVolts(5, 0))+(ltc.GetVolts(5, 1)))*10))
	if err != nil {
		log.Println(err)
		return
	}

	if time.Now().Second() == 0 {
		_, err = temperatureStatement.Exec(ltc.GetTemp(0, 0), ltc.GetTemp(0, 1), ltc.GetTemp(0, 2), ltc.GetTemp(0, 3), ltc.GetTemp(0, 4), ltc.GetTemp(0, 5),
			ltc.GetTemp(0, 6), ltc.GetTemp(0, 7), ltc.GetTemp(0, 8), ltc.GetTemp(0, 9), ltc.GetTemp(0, 10), ltc.GetTemp(0, 11),
			ltc.GetTemp(0, 12), ltc.GetTemp(0, 13), ltc.GetTemp(0, 14), ltc.GetTemp(0, 15), ltc.GetTemp(0, 16), ltc.GetTemp(0, 17),
			ltc.GetTemp(1, 0), ltc.GetTemp(1, 1), ltc.GetTemp(1, 2), ltc.GetTemp(1, 3), ltc.GetTemp(1, 4), ltc.GetTemp(1, 5),
			ltc.GetTemp(1, 6), ltc.GetTemp(1, 7), ltc.GetTemp(1, 8), ltc.GetTemp(1, 9), ltc.GetTemp(1, 10), ltc.GetTemp(1, 11),
			ltc.GetTemp(1, 12), ltc.GetTemp(1, 13), ltc.GetTemp(1, 14), ltc.GetTemp(1, 15), ltc.GetTemp(1, 16), ltc.GetTemp(1, 17),
			ltc.GetTemp(2, 0), ltc.GetTemp(2, 1),
			ltc.GetTemp(3, 0), ltc.GetTemp(3, 1), ltc.GetTemp(3, 2), ltc.GetTemp(3, 3), ltc.GetTemp(3, 4), ltc.GetTemp(3, 5),
			ltc.GetTemp(3, 6), ltc.GetTemp(3, 7), ltc.GetTemp(3, 8), ltc.GetTemp(3, 9), ltc.GetTemp(3, 10), ltc.GetTemp(3, 11),
			ltc.GetTemp(3, 12), ltc.GetTemp(3, 13), ltc.GetTemp(3, 14), ltc.GetTemp(3, 15), ltc.GetTemp(3, 16), ltc.GetTemp(3, 17),
			ltc.GetTemp(4, 0), ltc.GetTemp(4, 1), ltc.GetTemp(4, 2), ltc.GetTemp(4, 3), ltc.GetTemp(4, 4), ltc.GetTemp(4, 5),
			ltc.GetTemp(4, 6), ltc.GetTemp(4, 7), ltc.GetTemp(4, 8), ltc.GetTemp(4, 9), ltc.GetTemp(4, 10), ltc.GetTemp(4, 11),
			ltc.GetTemp(4, 12), ltc.GetTemp(4, 13), ltc.GetTemp(4, 14), ltc.GetTemp(4, 15), ltc.GetTemp(4, 16), ltc.GetTemp(4, 17),
			ltc.GetTemp(5, 0), ltc.GetTemp(5, 1))
	}
	if err != nil {
		log.Println(err)
	}
}

/**
Start the Web Socket server. This sends out data to all subscribers on a regular schedule so subscribers don't need to poll for updates.
*/
func startDataWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		if *verbose {
			fmt.Println("startDataWebSocket - ", err)
		}
		return
	}
	for {
		signal.L.Lock()   // Get the signal and lock it.
		signal.Wait()     // Wait for it to be signalled again. It is unlocked while we wait then locked again before returning
		signal.L.Unlock() // Unlock it
		w, err := conn.NextWriter(websocket.TextMessage)
		if err != nil {
			log.Println("Failed to get the values websocket writer - ", err)
			return
		}
		var sJSON = `{"battery":` + string(ltc.GetValuesAsJSON()) + `,"inverter":`
		jInverter, err := json.Marshal(&iValues)
		sJSON += string(jInverter)
		sJSON += `,"fuelgauge":`
		sFuelgauge, err := fuelgauge.GetData()
		if err != nil {
			sFuelgauge += `{"error":"` + err.Error() + `"}`
			log.Println("Failed to get the fuelgauge data - ", err)
		}
		sJSON += sFuelgauge + "}"
		_, err = fmt.Fprint(w, sJSON)
		if err != nil {
			log.Println("failed to write the values message to the websocket - ", err)
			return
		}
		if err := w.Close(); err != nil {
			log.Println("Failed to close the values websocket writer - ", err)
		}
	}
}

/**
Home page
*/
func socketHome(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, err := fmt.Fprint(w, homeHTML)
	if err != nil {
		log.Print("socketHome - ", err)
	}
}

/**
Handle a CAN frome from the inverter
*/
func handleCANFrame(frm can.Frame) {
	if *verbose {
		fmt.Println("Can frame received - ", frm.ID)
	}
	switch frm.ID {
	case 0x305: // Battery voltage, current and state of charge
		c305 := CAN_305.New(frm.Data[0:])

		iValues.Volts = c305.VBatt()
		iValues.Amps = c305.IBatt()
		iValues.Soc = c305.SocBatt()

	case 0x306: // Charge procedure, Operating state, Active error, Charge set point
		c306 := CAN_306.New(frm.Data[0:])
		iValues.Vsetpoint = c306.ChargeSetPoint()

	case 0x010: // Frequency
		c010 := CAN_010.New(frm.Data[0:])
		iValues.Frequency = c010.Frequency()

	case 0x307: // Relays and status
		c307 := CAN_307.New(frm.Data[0:])
		iValues.GnRun = c307.GnRun()
		iValues.OnRelay1 = c307.Relay1Master()
		iValues.OnRelay2 = c307.Relay2Master()
		iValues.OnRelay1Slave1 = c307.Relay1Slave1()
		iValues.OnRelay2Slave1 = c307.Relay2Slave1()
		iValues.OnRelay1Slave2 = c307.Relay1Slave2()
		iValues.OnRelay2Slave2 = c307.Relay2Slave2()
		iValues.GnRun = c307.GnRun()
		iValues.GnRunSlave1 = c307.GnRunSlave1()
		iValues.GnRunSlave2 = c307.GnRunSlave2()
		iValues.AutoGn = c307.AutoGn()
		iValues.AutoLodExt = c307.AutoLodExt()
		iValues.AutoLodSoc = c307.AutoLodSoc()
		iValues.Tm1 = c307.Tm1()
		iValues.Tm2 = c307.Tm2()
		iValues.ExtPwrDer = c307.ExtPwrDer()
		iValues.ExtVfOk = c307.ExtVfOk()
		iValues.GdOn = c307.GdOn()
		iValues.Errror = c307.Error()
		iValues.Run = c307.Run()
		iValues.BatFan = c307.BatFan()
		iValues.AcdCir = c307.AcdCir()
		iValues.MccBatFan = c307.MccBatFan()
		iValues.MccAutoLod = c307.MccAutoLod()
		iValues.Chp = c307.Chp()
		iValues.ChpAdd = c307.ChpAdd()
		iValues.SiComRemote = c307.SiComRemote()
		iValues.OverLoad = c307.Overload()
		iValues.ExtSrcConn = c307.ExtSrcConn()
		iValues.Silent = c307.Silent()
		iValues.Current = c307.Current()
		iValues.FeedSelfC = c307.FeedSelfC()
		iValues.Esave = c307.Esave()
	}
}

func webToggleCoil(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	fuelgauge.WebToggleCoil(w, r)
}

func webProcessHoldingRegistersForm(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	fuelgauge.WebProcessHoldingRegistersForm(w, r)
}

func webWaterBank(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	fuelgauge.WebWaterBank(w, r)
}

func webSwitchOffBank(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	vars := mux.Vars(r)
	bank, err := strconv.ParseUint(vars["bank"], 10, 8)
	if (err != nil) || (bank > 1) {
		http.Error(w, "Invalid battery bank", http.StatusBadRequest)
		return
	}
	fuelgauge.SwitchOffBank(int(bank))
}

func webBatteryFan(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	fuelgauge.WebBatteryFan(w, r)
}

func webSwitchBattery(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	fuelgauge.WebSwitchBattery(w, r)
}

func webGetFuelGaugeValues(w http.ResponseWriter, r *http.Request) {
	setHeaders(w)
	fuelgauge.WebGetValues(w, r)
}

func webGetSerialNumbers(w http.ResponseWriter, _ *http.Request) {

	var serialNumbers struct {
		CellNumber         int    `json:"cell_number"`
		SerialNumber       string `json:"serial_number"`
		InstallDate        string `json:"install_date"`
		FullChargeDetected string `json:"full_charge_detected"`
		FullCharge         int    `json:"full_charge"`
	}
	rowNum := 1
	setHeaders(w)

	rows, err := pDB.Query(`select cell_number as cell_number, serial_number as serial_number, date_format(install_date, "%D %M %Y") as install_date, date_format(full_charge_detected, "%D %M %Y %H:%i:%s") as full_charge_detected, full_charge as full_charge from serial_numbers`)
	if err != nil {
		_, err2 := fmt.Fprint(w, `{"error":"`, err, `"}`)
		log.Println("Error getting serial numbers - ", err)
		if err2 != nil {
			log.Print("webGetSerialNumbers - ", err2)
		}
	} else {
		_, eFmt := fmt.Fprint(w, `{`)
		if eFmt != nil {
			log.Println(eFmt)
		}
		for rows.Next() {
			if rowNum > 1 {
				_, eFmt := fmt.Fprint(w, ",")
				if eFmt != nil {
					log.Println(eFmt)
				}
			}
			if rowNum == 39 {
				rowNum = 101
			}
			err = rows.Scan(&serialNumbers.CellNumber, &serialNumbers.SerialNumber, &serialNumbers.InstallDate, &serialNumbers.FullChargeDetected, &serialNumbers.FullCharge)
			if err != nil {
				log.Println("Failed to get the serial numbers - ", err)
			} else {
				jsonString, err := json.Marshal(serialNumbers)
				if err != nil {
					log.Println("Failed to convert serial numbers to JSON - ", err)
				} else {
					_, eFmt := fmt.Fprint(w, `"`, rowNum, `":`, string(jsonString))
					if eFmt != nil {
						log.Println(eFmt)
					}
				}
			}
			rowNum++
		}
		_, eFmt = fmt.Fprint(w, `}`)
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

func webGetLastFullChargeTimes(w http.ResponseWriter, _ *http.Request) {
	setHeaders(w)
	_, eFmt := fmt.Fprint(w, fuelgauge.GetLastFullChargeTimes())
	if eFmt != nil {
		log.Println(eFmt)
	}
}

func webGetBatterySettings(w http.ResponseWriter, _ *http.Request) {
	setHeaders(w)
	_, eFmt := fmt.Fprint(w, fuelgauge.GetCapacity())
	if eFmt != nil {
		log.Println(eFmt)
	}
}

func returnWebError(w http.ResponseWriter, err error) {
	_, eFmt := fmt.Fprint(w, `{"error":"`, err.Error(), `"}`)
	if eFmt != nil {
		log.Println(eFmt)
	}
}

func webGetStatus(w http.ResponseWriter, r *http.Request) {
	type current struct {
		Current  float64 `json:"current"`
		Left     float64 `json:"left"`
		Right    float64 `json:"right"`
		SOC      float64 `json:"soc"`
		SOCLeft  float64 `json:"soc_left"`
		SOCRight float64 `json:"soc_right"`
	}
	var currentVal current
	setHeaders(w)

	vars := mux.Vars(r)
	seconds, err := strconv.ParseUint(vars["avg"], 10, 16)
	if err != nil {
		http.Error(w, "Invalid averaging seconds (avg)", http.StatusBadRequest)
		log.Println(err)
		return
	}

	// Get the average current and state of charge (SOC) for the past 5 minutes
	sSQL := `select avg(channel_0 + channel_1) as current
			, avg(channel_0) as left_
			, avg(channel_1) as right_
			, avg(level_of_charge_0 + level_of_charge_1) as soc
			, avg(level_of_charge_0) as soc_left
			, avg(level_of_charge_1) as soc_right
		from current
		where logged > date_add(now(), interval -` + strconv.FormatUint(seconds, 10) + ` second)`

	// Get the battery specifications for capacity
	total, left, right := fuelgauge.Capacity()
	row := pDB.QueryRow(sSQL)
	err = row.Scan(&currentVal.Current, &currentVal.Left, &currentVal.Right, &currentVal.SOC, &currentVal.SOCLeft, &currentVal.SOCRight)
	if err != nil {
		_, eFmt := fmt.Fprint(w, `{"error":"`, err.Error(), `","sql":"`, sSQL, `"}`)
		if eFmt != nil {
			log.Println(eFmt)
		}
	} else {
		// Round everything to 1 decimal place and calculate SOC as percentages
		currentVal.Current = math.Round(currentVal.Current*10) / 10
		currentVal.Left = math.Round(currentVal.Left*10) / 10
		currentVal.Right = math.Round(currentVal.Right*10) / 10
		currentVal.SOC = math.Round((currentVal.SOC/float64(total))*1000) / 10
		currentVal.SOCLeft = math.Round((currentVal.SOCLeft/float64(left))*1000) / 10
		currentVal.SOCRight = math.Round((currentVal.SOCRight/float64(right))*1000) / 10

		sJSON, err := json.Marshal(currentVal)
		if err != nil {
			returnWebError(w, err)
			return
		}
		_, eFmt := fmt.Fprint(w, string(sJSON))
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

func webGetCurrentData(w http.ResponseWriter, r *http.Request) {
	type current struct {
		Logged   float64 `json:"logged"`
		Left     float64 `json:"left"`
		Right    float64 `json:"right"`
		SOCLeft  float64 `json:"soc_left"`
		SOCRight float64 `json:"soc_right"`
	}
	var currentVal current
	var currentData []current = nil

	setHeaders(w)

	sSQL := `select min(unix_timestamp(logged)) as logged,
		avg(channel_0) as left_,
		avg(channel_1) as right_,
		avg(level_of_charge_0) as soc_left,
		avg(level_of_charge_1) as soc_right
		from current
		where logged between '` + r.FormValue("start") + `' and '` + r.FormValue("end") + `'
		group by unix_timestamp(logged) DIV 15`

	rows, err := pDB.Query(sSQL)
	if err != nil {
		_, eFmt := fmt.Fprint(w, `{"error":"`, err.Error(), `","sql":"`, sSQL, `"}`)
		if eFmt != nil {
			log.Println(eFmt)
		}
	} else {
		for rows.Next() {
			err = rows.Scan(&currentVal.Logged, &currentVal.Left, &currentVal.Right, &currentVal.SOCLeft, &currentVal.SOCRight)
			if err != nil {
				returnWebError(w, err)
				return
			}
			currentData = append(currentData, currentVal)
		}
		sJSON, err := json.Marshal(currentData)
		if err != nil {
			returnWebError(w, err)
			return
		}
		_, eFmt := fmt.Fprint(w, string(sJSON))
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

func webGetVoltageData(w http.ResponseWriter, r *http.Request) {
	type voltage struct {
		Logged float64 `json:"logged"`
		Left   float64 `json:"left"`
		Right  float64 `json:"right"`
	}
	var voltageVal voltage
	var voltageData []voltage = nil

	setHeaders(w)

	sSQL := `select min(unix_timestamp(logged)) as logged, avg(bank_0) / 10 as left_, avg(bank_1) / 10 as right_
  from voltage
 where logged between '` + r.FormValue("start") + `' and '` + r.FormValue("end") + `'
		group by unix_timestamp(logged) DIV 15`
	rows, err := pDB.Query(sSQL)
	if err != nil {
		_, eFmt := fmt.Fprint(w, `{"error":"`, err.Error(), `","sql":"`, sSQL, `"}`)
		if eFmt != nil {
			log.Println(eFmt)
		}
	} else {
		for rows.Next() {
			err = rows.Scan(&voltageVal.Logged, &voltageVal.Left, &voltageVal.Right)
			if err != nil {
				returnWebError(w, err)
				return
			}
			voltageData = append(voltageData, voltageVal)
		}
		sJSON, err := json.Marshal(voltageData)
		if err != nil {
			returnWebError(w, err)
			return
		}
		_, eFmt := fmt.Fprint(w, string(sJSON))
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

/**
Get the charging parameter values
*/
func webGetChargingParameters(w http.ResponseWriter, _ *http.Request) {
	sJSON, err := json.Marshal(setpoints)
	if err != nil {
		returnWebError(w, err)
		return
	}
	_, eFmt := fmt.Fprint(w, string(sJSON))
	if eFmt != nil {
		log.Println(eFmt)
	}
}

/**
Cell data including current and voltage for one cell
*/
func webGetCellData(w http.ResponseWriter, r *http.Request) {
	type values struct {
		Logged  float64 `json:"logged"`
		Current float64 `json:"amps"`
		Voltage float64 `json:"volts"`
	}
	var cellVal values
	var cellData []values = nil
	sCurrentTerm := ""

	vars := mux.Vars(r)
	setHeaders(w)

	cell, _ := strconv.ParseInt(vars["cell"], 10, 16)
	if (r.FormValue("minAmps") != "") && (r.FormValue("maxAmps") != "") {
		sCurrentTerm = fmt.Sprintf(" and i.channel_%d > %s and i.channel_%d < %s", cell/100, r.FormValue("minAmps"), cell/100, r.FormValue("maxAmps"))
	}

	sSQL := fmt.Sprintf(`select min(unix_timestamp(v.logged)) as logged, avg(cell_%03d) / 10000 as volts, avg(i.channel_%d) as amps
    from voltage v join current i on i.logged = from_unixtime(round(unix_timestamp(v.logged)))
   where v.logged between '`+r.FormValue("start")+`' and '`+r.FormValue("end")+`'`+sCurrentTerm+`
   group by unix_timestamp(v.logged) DIV 15`, cell, cell/100)
	rows, err := pDB.Query(sSQL)
	if err != nil {
		_, eFmt := fmt.Fprint(w, `{"error":"`, err.Error(), `","sql":"`, sSQL, `"}`)
		if eFmt != nil {
			log.Println(eFmt)
		}
	} else {
		for rows.Next() {
			err = rows.Scan(&cellVal.Logged, &cellVal.Voltage, &cellVal.Current)
			if err != nil {
				returnWebError(w, err)
				return
			}
			cellData = append(cellData, cellVal)
		}
		sJSON, err := json.Marshal(cellData)
		if err != nil {
			returnWebError(w, err)
			return
		}
		_, eFmt := fmt.Fprint(w, string(sJSON))
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

func SendSMAHeartBeat() {
	heartbeat := time.NewTicker(time.Second)

	bus, err := can.NewBusForInterfaceWithName("can0")
	//	var err error
	loops := 0

	if err != nil {
		log.Fatalf("Error starting CAN interface - %s -\nSorry, I am giving up", err)
	} else {
		log.Println("Connected to CAN bus - monitoring the inverters.")
	}
	for {
		<-heartbeat.C
		msg351 := CAN_351.New(setpoints.VSetpoint, setpoints.ISetpoint, setpoints.IDischarge, setpoints.VDischarge)
		//		log.Println("CAN-351 : ", msg351.Frame())
		err := bus.Publish(msg351.Frame())
		if err != nil {
			log.Println("CAN 351 Message error - ", err)
		}
		msg355 := CAN_355.New(uint16(fuelgauge.StateOfCharge()), 100.0, fuelgauge.StateOfCharge())
		//		log.Println("CAN-355 : ", msg355.Frame())
		err = bus.Publish(msg355.Frame())
		if err != nil {
			log.Println("CAN 355 Message error - ", err)
		}

		msg356 := CAN_356.New(ltc.GetActiveBatteryVoltage(), fuelgauge.Current(), ltc.GetMaxTemperature())
		//		log.Println("CAN-356 : ", msg356.Frame())
		err = bus.Publish(msg356.Frame())
		if err != nil {
			log.Println("CAN 356 Message error - ", err)
		}

		if loops == 0 {
			msg35E := CAN_35E.New("Encell")
			//			log.Println("CAN-35E : ", msg35E.Frame())
			err = bus.Publish(msg35E.Frame())
			if err != nil {
				log.Println("CAN-35E Message error - ", err)
			}
		}
		loops++
		if loops > 15 {
			loops = 0
		}
		// Move the setpoints closer to the target values slowly. Voltage 0.2V/sec, Current 5.0A/sec
		vDiff := setpoints.VTargetSetpoint - setpoints.VSetpoint
		if vDiff != 0 {
			if vDiff > 0.2 {
				vDiff = 0.2
			}
			if vDiff < -0.2 {
				vDiff = -0.2
			}
			setpoints.VSetpoint += vDiff
		}
		iDiff := setpoints.ITargetSetpoint - setpoints.ISetpoint
		if iDiff != 0 {
			if iDiff > 5.0 {
				iDiff = 5.0
			}
			if iDiff < -5.0 {
				iDiff = -5.0
			}
			setpoints.ISetpoint += iDiff
		}
	}
}

func mainImpl() error {
	//	if !*verbose {
	//		log.SetOutput(ioutil.Discard)
	//	}
	log.SetFlags(log.Lmicroseconds)
	if flag.NArg() != 0 {
		return errors.New("unexpected argument, try -help")
	}

	for {
		nDevices, err := getLTC6813(6)
		if err == nil && nDevices > 0 {
			break
		}
		fmt.Println("Looking for a device")
		time.Sleep(3 * time.Second)
	}
	log.Println("Starting up")
	fmt.Println("Startup.")
	ticker := time.NewTicker(time.Second)
	//	dataReady := make(chan bool)

	go func() {
		log.Println("Starting logger.")
		for {
			<-ticker.C
			performMeasurements()
		}

	}()

	// Every minute we need to process the full charge data.
	fullChargeTicker := time.NewTicker(time.Minute)
	go func() {
		//		var bank0Full bool = false
		//		var bank1Full bool = false
		var bank0WateredToday = false
		var bank1WateredToday = false
		for {
			//			log.Println("Starting full charge checker.")
			<-fullChargeTicker.C
			fuelgauge.ReadSystemParameters()
			hour := time.Now().Hour()
			// No point in testing before 10am or after 8pm as there is no chance
			// we are going to hit full charge so early in the morning or after the sun is going down.
			//			if hour > 9 && hour < 19 {
			if fuelgauge.ReadyToWater(0) && !bank0WateredToday {
				// Water bank 0
				err := fuelgauge.WaterBank(0, 10)
				if err != nil {
					log.Println(err)
				}
				bank0WateredToday = true
			}
			if fuelgauge.ReadyToWater(1) && !bank1WateredToday {
				// Water bank 1
				err := fuelgauge.WaterBank(1, 10)
				if err != nil {
					log.Println(err)
				}
				bank1WateredToday = true
			}
			// If the evaluator pointer is nil then create a new evaluator
			//				log.Println("Checking for full charge...")
			if evaluator == nil {
				evaluator, _ = FullChargeEvaluator.New(pDB)
			}
			// If the pointer is still nil we failed to create the evaluator so skip and try again next time.
			if evaluator != nil {
				// Process the full charge state process
				err := evaluator.ProcessFullCharge(time.Now())
				//					log.Println("Evaluating full charge")
				//					t := time.Date(2020, 7, 21, 13,20, 0, 0, time.Local)
				//					log.Println("Checking for", t)
				//					err := evaluator.ProcessFullCharge( t)
				// If we hit an error we should dispose of the evaluator and wait till the next loop to try again.
				if err != nil {
					log.Println(err)
					evaluator = nil
				}
			} else {
				log.Println("No full charge evaluator!")
			}
			// Test each bank for full charge
			// If bank 0 is full and bank 1 is not, turn on bank 1
			// Don't switch back unless bank 0 drops below 95%
			if fuelgauge.TestFullCharge(0) && !fuelgauge.TestFullCharge(1) {
				fuelgauge.SwitchOffBank(0)
			} else if (fuelgauge.StateOfChargeLeft() < 95.0) || fuelgauge.TestFullCharge(1) {
				fuelgauge.SwitchOffBank(1)
			}
			if fuelgauge.TestFullCharge(0) && fuelgauge.TestFullCharge(1) {
				setpoints.VTargetSetpoint = setpoints.VChargedSetpoint
				setpoints.ITargetSetpoint = setpoints.IChargedSetpoint
			} else if fuelgauge.StateOfChargeLeft() < 98 || fuelgauge.StateOfChargeRight() < 98 {
				setpoints.VTargetSetpoint = setpoints.VChargingSetpoint
				setpoints.ITargetSetpoint = setpoints.IChargingSetpoint
			}
			//			} else {
			if (hour == 1) && (bank0WateredToday || bank1WateredToday) {
				// Clear the flags saying we have watered the battery
				bank0WateredToday = false
				bank1WateredToday = false
			}

			// While the bank 1 cells are problematic we need to switch to bank 0 at 8:00PM
			if (hour == 20) && (time.Now().Minute() == 0) {
				log.Println("Switching off right bank at 8PM")
				go fuelgauge.SwitchOffBank(FuelGauge.RightBank)
			}
			// Manage the battery fan based on the maximum temperature. If one or more temperature sensors
			// show more than 42.0 C then turn on the fan if it is off. If the fan is on and the temperature is below 40.0
			// turn it off.
			temp := ltc.GetMaxTemperature()
			if temp > 39.5 {
				log.Println("Checking the temperature - ", temp, " autoFan = ", autoFan)
			}
			if (temp > 42.0) && !autoFan {
				log.Println("Turning on the battery fan because the maximum temperature has risen to ", temp)
				fuelgauge.TurnOnFan()
				autoFan = true
			} else if (temp < 41.5) && autoFan {
				log.Println("Turning off the battery fan because the maximum temperature has dropped to ", temp)
				fuelgauge.TurnOffFan()
				autoFan = false
			}
		}
	}()

	go func() {
		for {
			//			<-dataReady
			signal.L.Lock()   // Get the signal and lock it.
			signal.Wait()     // Wait for it to be signalled again. It is unlocked while we wait then locked again before returning
			signal.L.Unlock() // Unlock it
			logData()
		}
	}()

	// Start handling incoming CAN messages
	go func() {
		bus, err := can.NewBusForInterfaceWithName("can0")
		if err != nil {
			log.Fatalf("Error starting CAN interface - %s -\nSorry, I am giving up", err)
		} else {
			log.Println("Connected to CAN bus - monitoring the inverters.")
		}
		bus.SubscribeFunc(handleCANFrame)
		err = bus.ConnectAndPublish()
		if err != nil {
			log.Printf("ConnectAndPublish failed - %s", err)
		}
	}()

	// Start sending the SMA heartbeat to the Sunny Island inverters
	go SendSMAHeartBeat()

	// Configure and start the WEB server
	fmt.Println("Starting the WEB server")
	router := mux.NewRouter().StrictSlash(true)
	router.PathPrefix("/").Methods("OPTIONS").HandlerFunc(webOptionsHandler)
	router.HandleFunc("/values", getValues).Methods("GET")
	router.HandleFunc("/version", getVersion).Methods("GET")
	router.HandleFunc("/i2cread", getI2Cread).Methods("GET")
	router.HandleFunc("/i2cwrite", getI2Cwrite).Methods("GET")
	router.HandleFunc("/i2creadByte", getI2CreadByte).Methods("GET")
	router.HandleFunc("/i2cVoltage", getI2CVoltage).Methods("GET")
	router.HandleFunc("/i2cCharge", getI2CCharge).Methods("GET")
	router.HandleFunc("/i2cCurrent", getI2CCurrent).Methods("GET")
	router.HandleFunc("/i2cTemp", getI2CTemp).Methods("GET")
	router.HandleFunc("/ws", startDataWebSocket).Methods("GET")
	router.HandleFunc("/sockets", socketHome).Methods("GET")
	router.HandleFunc("/fuelgauge", webGetFuelGaugeValues).Methods("GET")
	router.HandleFunc("/toggleCoil", webToggleCoil).Methods("PATCH")
	router.HandleFunc("/setHoldingRegisters", webProcessHoldingRegistersForm).Methods("POST")
	router.HandleFunc("/waterBank/{bank}/{minutes}", webWaterBank).Methods("PATCH")
	router.HandleFunc("/batteryFan/{onOff}", webBatteryFan).Methods("PATCH")
	router.HandleFunc("/batterySwitch/{bank}/{onOff}", webSwitchBattery).Methods("PATCH")
	router.HandleFunc("/serialNumbers", webGetSerialNumbers).Methods("GET")
	router.HandleFunc("/lastFullChargeTimes", webGetLastFullChargeTimes).Methods("GET")
	router.HandleFunc("/batterySettings", webGetBatterySettings).Methods("GET")
	router.HandleFunc("/batteryCurrent", webGetCurrentData).Methods("GET")
	router.HandleFunc("/batteryVoltages", webGetVoltageData).Methods("GET")
	router.HandleFunc("/cellValues/{cell}", webGetCellData).Methods("GET")
	router.HandleFunc("/status/{avg}", webGetStatus).Methods("GET")
	router.HandleFunc("/bankOff/{bank}", webSwitchOffBank).Methods("GET")
	router.HandleFunc("/chargingParameters", webGetChargingParameters).Methods("GET")
	spa := spaHandler{staticPath: "/var/www/html", indexPath: "index.html"}
	router.PathPrefix("/").Handler(spa)

	srv := &http.Server{
		Handler: router,
		Addr:    ":8000",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	//err := http.ListenAndServe(":8000", router) // Listen on port 8000
	err := srv.ListenAndServe()
	if err != nil {
		log.Println("WEB Server Startup error - ", err)
	}
	return nil
}

func connectToDatabase() (*sql.DB, error) {
	if pDB != nil {
		_ = pDB.Close()
		pDB = nil
	}
	var sConnectionString = *pDatabaseLogin + ":" + *pDatabasePassword + "@tcp(" + *pDatabaseServer + ":" + *pDatabasePort + ")/" + *pDatabaseName + "?loc=Local"

	//	fmt.Println("Connecting to [", sConnectionString, "]")
	db, err := sql.Open("mysql", sConnectionString)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	// Prepare the insert statements for voltage and temperature
	sSQL := `insert into voltage (cell_001,cell_002,cell_003,cell_004,cell_005,cell_006,cell_007,cell_008,cell_009,cell_010
                                 ,cell_011,cell_012,cell_013,cell_014,cell_015,cell_016,cell_017,cell_018,cell_019,cell_020
                                 ,cell_021,cell_022,cell_023,cell_024,cell_025,cell_026,cell_027,cell_028,cell_029,cell_030
                                 ,cell_031,cell_032,cell_033,cell_034,cell_035,cell_036,cell_037,cell_038
                                 ,cell_101,cell_102,cell_103,cell_104,cell_105,cell_106,cell_107,cell_108,cell_109,cell_110
                                 ,cell_111,cell_112,cell_113,cell_114,cell_115,cell_116,cell_117,cell_118,cell_119,cell_120
                                 ,cell_121,cell_122,cell_123,cell_124,cell_125,cell_126,cell_127,cell_128,cell_129,cell_130
                                 ,cell_131,cell_132,cell_133,cell_134,cell_135,cell_136,cell_137,cell_138
                                 ,bank_0,bank_1)
                          values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
                                 ,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
                                 ,?,?)`
	voltageStatement, err = db.Prepare(sSQL)
	if err != nil {
		errClose := db.Close()
		if errClose != nil {
			log.Println(errClose)
		}
		return nil, err
	}

	sSQL = `insert into temperature (temp_001,temp_002,temp_003,temp_004,temp_005,temp_006,temp_007,temp_008,temp_009,temp_010
                                    ,temp_011,temp_012,temp_013,temp_014,temp_015,temp_016,temp_017,temp_018,temp_019,temp_020
                                    ,temp_021,temp_022,temp_023,temp_024,temp_025,temp_026,temp_027,temp_028,temp_029,temp_030
                                    ,temp_031,temp_032,temp_033,temp_034,temp_035,temp_036,temp_037,temp_038
                                    ,temp_101,temp_102,temp_103,temp_104,temp_105,temp_106,temp_107,temp_108,temp_109,temp_110
                                    ,temp_111,temp_112,temp_113,temp_114,temp_115,temp_116,temp_117,temp_118,temp_119,temp_120
                                    ,temp_121,temp_122,temp_123,temp_124,temp_125,temp_126,temp_127,temp_128,temp_129,temp_130
                                    ,temp_131,temp_132,temp_133,temp_134,temp_135,temp_136,temp_137,temp_138)
                            values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
                                   ,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	temperatureStatement, err = db.Prepare(sSQL)
	if err != nil {
		errClose := db.Close()
		if errClose != nil {
			log.Println(errClose)
		}
		return nil, err
	} else {
		return db, err
	}
}

func init() {
	// Set up the parameters to send to the inverter.
	setpoints.VSetpoint = 65.0
	setpoints.ISetpoint = 1200.0
	setpoints.VTargetSetpoint = 65.0
	setpoints.ITargetSetpoint = 1200.0
	setpoints.VDischarge = 36.0
	setpoints.IDischarge = 1200.0
	setpoints.VChargingSetpoint = 65.0
	setpoints.VChargedSetpoint = 61.0
	setpoints.IChargingSetpoint = 1200.0
	setpoints.IChargedSetpoint = 35.0

	logwriter, e := syslog.New(syslog.LOG_NOTICE, "BatteryMonitor")
	if e == nil {
		log.SetOutput(logwriter)
	} else {
		fmt.Println(e)
	}
	verbose = flag.Bool("v", false, "verbose mode")
	spiDevice = flag.String("c", "/dev/spidev0.1", "SPI device from /dev")
	pDatabaseLogin = flag.String("l", "logger", "Database Login ID")
	pDatabasePassword = flag.String("p", "logger", "Database password")
	pDatabaseServer = flag.String("s", "localhost", "Database server")
	pDatabasePort = flag.String("o", "3306", "Database port")
	pDatabaseName = flag.String("d", "battery", "Name of the database")
	pCommsPort := flag.String("Port", "/dev/serial/by-path/platform-3f980000.usb-usb-0:1.3:1.0-port0", "communication port")
	pBaudRate := flag.Int("Baudrate", 19200, "communication port baud rate")
	pDataBits := flag.Int("Databits", 8, "communication port data bits")
	pStopBits := flag.Int("Stopbits", 2, "communication port stop bits")
	pParity := flag.String("Parity", "N", "communication port parity")
	pTimeoutMilliSecs := flag.Int("Timeout", 500, "communication port timeout in milliseconds")
	pSlave1Address := flag.Int("Slave1", 5, "Modbus slave1 ID")
	pSlave2Address := flag.Int("Slave2", 1, "Modbus slave2 ID (0 = not present)")

	flag.Parse()
	// Initialise the SPI subsystem
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}
	p, err := spireg.Open(*spiDevice)
	if err != nil {
		log.Fatal(err)
	}

	spiConnection, err = p.Connect(SPIBAUDRATE, spi.Mode0, SPIBITSPERWORD)
	if err != nil {
		log.Fatal(err)
	}
	nErrors = 0

	// Set up the database connection
	pDB, err = connectToDatabase()
	if err != nil {
		log.Fatalf("Failed to connect to to the database - %s - Sorry, I am giving up.", err)
	}
	// Set up the modbus serial comms to communicate with the current sensors and relays
	fuelgauge = FuelGauge.New(*pCommsPort, *pBaudRate, *pDataBits, *pStopBits, *pParity, time.Duration(*pTimeoutMilliSecs)*time.Millisecond, pDB, uint8(*pSlave1Address), uint8(*pSlave2Address))
	fuelgauge.ReadSystemParameters()
	go fuelgauge.Run()
}

/*
	WEB Service to return the version information
*/
func getVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, err := fmt.Fprint(w, `<html>
  <head>
    <Cedar Technology Battery Manager>
  </head>
  <body>
    <h1>Cedar Technology Battery Manager</h1>
    <h2>Version 1.0 - March 27th 2020</h2>
  </body>
</html>`)
	if err != nil {
		log.Print("getVersion() - ", err)
	}
}

/*
WEB service to return current process values
*/

func getValues(w http.ResponseWriter, _ *http.Request) {
	ltcLock.Lock()
	defer ltcLock.Unlock()
	// This header allows the output to be used in a WEB page from another server as a data source for some controls
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if ltc != nil {
		_, _ = fmt.Fprintf(w, `{%s,%s}`, ltc.GetVoltagesAsJSON(), ltc.GetTemperaturesAsJSON())
	} else {
		_, _ = fmt.Fprint(w, `{"error":"No Devices"}`)
	}
}

/**
WEB service to read the I2C port
*/
func getI2Cread(w http.ResponseWriter, r *http.Request) {
	var reg int64
	sReg := r.URL.Query().Get("reg")
	if sReg != "" {
		reg, _ = strconv.ParseInt(sReg, 0, 8)
	} else {
		reg = 0x1a
	}
	sensor, _ := strconv.ParseInt(r.URL.Query().Get("sensor"), 0, 8)
	s, err := ltc.ReadI2CWord(int(sensor), LTC6813.LTC2944Address, uint8(reg))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, err2 := fmt.Fprint(w, "Request = ", r.URL.Query().Get("reg"), "\n")
	if err != nil {
		_, err2 = fmt.Fprint(w, "Error - ", err)
	} else {
		_, err2 = fmt.Fprintf(w, s)
	}
	if err2 != nil {
		log.Print("getI2Cread() - ", err2)
	}
}

/**
WEB service to read one 8 bit register from the I2C port
*/
func getI2CreadByte(w http.ResponseWriter, r *http.Request) {
	sensor, _ := strconv.ParseInt(r.URL.Query().Get("sensor"), 0, 8)
	var reg int64
	sReg := r.URL.Query().Get("reg")
	if sReg != "" {
		reg, _ = strconv.ParseInt(sReg, 0, 8)
	} else {
		reg = 0x1a
	}
	s, err := ltc.ReadI2CByte(int(sensor), LTC6813.LTC2944Address, uint8(reg))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, err2 := fmt.Fprint(w, "Request = ", r.URL.Query().Get("reg"), "\n")
	if err != nil {
		_, err2 = fmt.Fprint(w, "Error - ", err)
	} else {
		_, err2 = fmt.Fprintf(w, s)
	}
	if err2 != nil {
		log.Print("getI2CreadByte() - ", err2)
	}
}

/**
WEB service to read the current from the I2C port
*/
func getI2CCurrent(w http.ResponseWriter, r *http.Request) {
	sensor, _ := strconv.ParseInt(r.URL.Query().Get("sensor"), 0, 8)

	t, err := ltc.GetI2CCurrent(int(sensor))
	if err != nil {
		_, eFmt := fmt.Fprint(w, err)
		if eFmt != nil {
			log.Println(eFmt)
		}
	} else {
		_, eFmt := fmt.Fprintf(w, "Current on sensor %d = %f", sensor, t)
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

/**
WEB service to read the voltage from the I2C port
*/
func getI2CVoltage(w http.ResponseWriter, r *http.Request) {
	sensor, _ := strconv.ParseInt(r.URL.Query().Get("sensor"), 0, 8)

	t, err := ltc.GetI2CVoltage(int(sensor))
	if err != nil {
		_, eFmt := fmt.Fprint(w, err)
		if eFmt != nil {
			log.Println(eFmt)
		}
	} else {
		_, eFmt := fmt.Fprintf(w, "Voltage on sensor %d = %f", sensor, t)
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

/**
WEB service to read the charge from the I2C port
*/
func getI2CCharge(w http.ResponseWriter, r *http.Request) {
	sensor, _ := strconv.ParseInt(r.URL.Query().Get("sensor"), 0, 8)

	t, err := ltc.GetI2CAccumulatedCharge(int(sensor))
	if err != nil {
		_, eFmt := fmt.Fprint(w, err)
		if eFmt != nil {
			log.Println(eFmt)
		}
	} else {
		_, eFmt := fmt.Fprintf(w, "Accumulated charge on sensor %d = %f", sensor, t)
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

/**
WEB service to read the temperature from the I2C port
*/
func getI2CTemp(w http.ResponseWriter, r *http.Request) {
	sensor, _ := strconv.ParseInt(r.URL.Query().Get("sensor"), 0, 8)

	t, err := ltc.GetI2CTemp(int(sensor))
	if err != nil {
		_, eFmt := fmt.Fprint(w, err)
		if eFmt != nil {
			log.Println(eFmt)
		}
	} else {
		_, eFmt := fmt.Fprintf(w, "Temperature on sensor %d = %f", sensor, t)
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

/**
WEB service to write to one of the I2C registers
*/
func getI2Cwrite(w http.ResponseWriter, r *http.Request) {
	var reg, value int64
	s := r.URL.Query().Get("reg")
	if s != "" {
		reg, _ = strconv.ParseInt(s, 0, 16)
	} else {
		reg = 0x1a
	}
	s = r.URL.Query().Get("value")
	if s != "" {
		value, _ = strconv.ParseInt(s, 0, 16)
	} else {
		value = 0
	}
	_, err := ltc.WriteI2CByte(2, LTC6813.LTC2944Address, uint8(reg), uint8(value))
	w.Header().Set("Access-Control-Allow-Origin", "*")
	//	fmt.Fprint(w, "Request = ", r.URL.Query().Get("reg"), "\n")
	if err != nil {
		_, eFmt := fmt.Fprint(w, "Error - ", err)
		if eFmt != nil {
			log.Println(eFmt)
		}
	} else {
		_, eFmt := fmt.Fprintf(w, "Register %d set 0x%x", reg, value)
		if eFmt != nil {
			log.Println(eFmt)
		}
	}
}

func main() {

	signal = sync.NewCond(&sync.Mutex{})

	if err := mainImpl(); err != nil {
		_, eFmt := fmt.Fprintf(os.Stderr, "BatteryMonitor6813V4 Error: %s.\n", err)
		if eFmt != nil {
			log.Println(eFmt)
		}
		os.Exit(1)
	}
	fmt.Println("Program has ended.")
}

const homeHTML = `<!DOCTYPE html>
<html lang="en">
    <head>
        <title>WebSocket Example</title>
    </head>
    <body>
		<h1>WEB Socket Example - Voltages</h1><br />
        <pre id="Data">Data goes here</pre>
        <script type="text/javascript">
            (function() {
                var Data = document.getElementById("Data");
				var url = "ws://" + window.location.host + "/ws";
                var conn = new WebSocket(url);
                conn.onclose = function(evt) {
                    ata.textContent = 'Connection closed';
                }
                conn.onmessage = function(evt) {
                    Data.textContent = evt.data;
                }
            })();
        </script>
    </body>
</html>`
