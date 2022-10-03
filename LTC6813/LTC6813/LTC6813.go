package LTC6813

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"periph.io/x/periph/conn/spi"
	"sync"
	"time"
)

type LTC6813DataPacket [8]byte // Data block for one device

type LTC6813Packet []byte

/*	struct{
	cmd [4]byte					// Command to be sent
	data []LTC6813DataPacket	// Array of data blocks, one block per device in the chain
}
*/

type LTC6813Reading struct { // Structure to hold the readings from the chain of devices
	CellVolts        [18]uint16
	GPIOVolts        [9]uint16
	RefVolts         uint16
	SumOfCells       uint16
	ConfigRegister   [12]byte
	StatusRegister   [12]byte
	CommRegister     [6]byte
	SControlRegister [9]byte
	PWMRegister      [9]byte
	temperatures     [18]float32
}

type LTC6813 struct {
	spi               spi.Conn // SPI Connection
	chainLength       int      // The number of devices in the chain
	packet            LTC6813Packet
	readings          []LTC6813Reading
	mu                sync.Mutex // Controls access to the device chain
	dmu               sync.Mutex // Controls access to the data read from the device
	temperatureSensor int8
	lastCommand       time.Time
	lastVoltageError  string
	lastTempError     string
}

// Configuration Register A codes
const ADC_OPTION_0 = 0x00

//const ADC_OPTION_1 = 0x01

// Discharge Enable
const DISCHARGE_DISABLED = 0x00

//const DISCHARGE_ENABLED = 0x02

// Reference Voltage Output
//const REF_OFF = 0x00
const REF_ON = 0x04

// GPIO Pull Downs
//const GPIO1_PULL_DOWN_ON = 0x00
const GPIO1_PULL_DOWN_OFF = 0x08

//const GPIO2_PULL_DOWN_ON = 0x00
const GPIO2_PULL_DOWN_OFF = 0x10

//const GPIO3_PULL_DOWN_ON = 0x00
const GPIO3_PULL_DOWN_OFF = 0x20

//const GPIO4_PULL_DOWN_ON = 0x00
const GPIO4_PULL_DOWN_OFF = 0x40

//const GPIO5_PULL_DOWN_ON = 0x00
const GPIO5_PULL_DOWN_OFF = 0x80

//const GPIO6_PULL_DOWN_ON = 0x00
const GPIO6_PULL_DOWN_OFF = 0x01

//const GPIO7_PULL_DOWN_ON = 0x00
const GPIO7_PULL_DOWN_OFF = 0x02

//const GPIO8_PULL_DOWN_ON = 0x00
const GPIO8_PULL_DOWN_OFF = 0x04

//const GPIO9_PULL_DOWN_ON = 0x00
const GPIO9_PULL_DOWN_OFF = 0x08

// ADC Modes (used with ADC Conversion commands)
//                                           ADC_OPTION_0  :  ADC_OPTION_1
//const ADC_MODE_BASE = 0x00      //               422Hz    or    1kHz
const ADC_MODE_FAST = 0x80 // Fast -        27kHz    or   14kHz
//const ADC_MODE_NORMAL = 0x100   // Normal -       7kHz    or    3kHz
const ADC_MODE_FILTERED = 0x180 // Filtered -     26Hz    or    2kHz

// Discharge Control (used with Start Cell Voltage ADC Conversion,
//    Start Open Wire Conversion and Start Combined Cell Conversion)
//const DCP_NotPermitted = 0x0 // Discharge not permitted
const DCP_Permitted = 0x10 // Discharge Permitted

// Commands
const WRCFGA = 0x01 // Write Configuration Register Group A
const WRCFGB = 0x24 // Write Configuration Register Group B
const RDCFGA = 0x02 // Read Configuration Register Group A
//const RDCFGB = 0x26   // Read Configuration Register Group B
const RDCVA = 0x04   // Read Cell Voltage Register Group A
const RDCVB = 0x06   // Read Cell Voltage Register Group B
const RDCVC = 0x08   // Read Cell Voltage Register Group C
const RDCVD = 0x0A   // Read Cell Voltage Register Group D
const RDCVE = 0x09   // Read Cell Voltage Register Group D
const RDCVF = 0x0B   // Read Cell Voltage Register Group D
const RDAUXA = 0x0C  // Read Auxiliary Register Group A
const RDAUXB = 0x0E  // Read Auxiliary Register Group B
const RDAUXC = 0x0D  // Read Auxiliary Register Group C
const RDAUXD = 0x0F  // Read Auxiliary Register Group D
const RDSTATA = 0x10 // Read Status Register Group A
//const RDSTATB = 0x12  // Read Status Register Group B
//const WRSCTRL = 0x14  // Write S Control Register Group
//const WRPWM = 0x20    // Write Pulse Width Modulation Register Group
//const WRPSB = 0x1C    // Write Pulse Width Modulation/S Control Register Group B
//const RDSCTRL = 0x16  // Read S Control Register Group
//const RDPWM = 0x22    // Read Pulse Width Modulation Register Group
//const RDPSB = 0x1E    // Read Pulse Width Modulation/S Control Register Group B
//const STSCTRL = 0x19  // Start S Control Pulsing and Poll Status
//const CLRSCTRL = 0x18 // Clear S Control Register Group
const ADCV = 0x260 // Start Cell Voltage ADC Conversion and Poll Status
//const ADOW = 0x228    // Start Open Wire ADC Con- version and Poll Status
//const CVST = 0x207    // Start Self-Test Cell Voltage Conversion and Poll Status
//const ADOL = 0x201    // Start Overlap Measurements of Cell 7 and Cell 13 Voltages
const ADAX = 0x460 // Start GPIOs ADC Conversion and Poll Status
//const ADAXD = 0x408   // Start GPIOs ADC Conversion with Digital Redundancy and Poll Status
//const AXOW = 0x410    // Start GPIOs Open Wire ADC Conversion and Poll Status
//const AXST = 0x407    // Start Self-Test GPIOs Conversion and Poll Status
//const ADSTAT = 0x468  // Start Status group ADC Conversion and Poll Status
//const ADSTATD = 0x408 // Start Status group ADC Conversion and Poll Status
//const STATST = 0x40F  // Start Self-Test Status group Conversion and Poll Status
const ADCVAX = 0x46F // Start Combined Cell Voltage and GPIO1, GPIO2 Conversion and Poll Status
const ADCVSC = 0x467 // Start Combined Cell Voltage and Sum of Cells Conversion and Poll Status
//const CLRCELL = 0x711 // Clear Cell Voltage Register Group
//const CLRAUX = 0x712  // Clear Auxiliary Register Group
//const CLRSTAT = 0x713 // Clear Status Register Group
//const PLADC = 0x714   // Poll ADC Conversion Status
//const DIAGN = 0x715   // Diagnose MUX and Poll Status
const WRCOMM = 0x721 // Write Communications Register Group
const RDCOMM = 0x722 // Read Communications Register Group
const STCOMM = 0x723 // Start I2C/SPI Communication
//const MUTE = 0x28     // Mute Discharge
//const UNMUTE = 0x29   // Unmute Discharge

const BCOEFFICIENT = 6000.0 // B Coefficient of the thermistor used to measure temperature

/**
Communication register ICOM values specify control actions before transmitting/ receiving each data byte
*/
const I2CStart = 0x60 // Generate a START Signal on I2C Port Followed by Data Transmission
//const I2CStop = 0x10       // Generate a STOP Signal on I2C Port
//const I2CBlank = 0x00      // Proceed Directly to Data Transmission on I2C Port
const I2CNoTransmit = 0x70 // Release SDA and SCL and Ignore the Rest of the Data

/**
Communication register FCOM values specify control actions after transmitting/ receiving each data byte
*/
const I2CACK = 0x00 // Master Generates an ACK Signal on Ninth Clock Cycle
//const I2CNACK = 0x08     // Master Generates a NACK Signal on Ninth Clock Cycle
const I2CNackStop = 0x09 // Master Generates a NACK Signal Followed by STOP Signal

/**
Read Write bit for I2C communications
*/
const I2CREAD = 0x10
const I2CWRITE = 0x00

const LTC2944Address = 0xC8 // I2C address of the LTC2944 Battery Fuel Gauge in the top 7 bits

/**
LTC2944 registers
*/
//const LTC2944Status = 0x00                  // Status register
//const LTC2944Control = 0x01                 // Control register
const LTC2944ChargeMSB = 0x02 // Accumulated charge most significant byte
//const LTC2944ChargeLSB = 0x03               // Accumulated charge least significant byte
//const LTC2944ChargeThresholdHighMSB = 0x04  // Accumulated charge threshold high limit most significant byte
//const LTC2944ChargeThresholdHighLSB = 0x05  // Accumulated charge threshold high limit least significant byte
//const LTC2944ChargeThresholdLowMSB = 0x06   // Accumulated charge threshold low limit most significant byte
//const LTC2944ChargeThresholdLowLSB = 0x07   // Accumulated charge threshold low limit least significant byte
const LTC2944VoltageMSB = 0x08 // Voltage most significant byte
//const LTC2944VoltageLSB = 0x09              // Voltage least significant byte
//const LTC2944VoltageThresholdHighMSB = 0x0A // Voltage High Treshold most significant byte
//const LTC2944VoltageThresholdHighLSB = 0x0B // Voltage High Treshold least significant byte
//const LTC2944VoltageThresholdLowMSB = 0x0C  // Voltage Low Treshold most significant byte
//const LTC2944VoltageThresholdLowLSB = 0x0D  // Voltage Low Treshold least significant byte
const LTC2944CurrentMSB = 0x0E // Current most significant byte
//const LTC2944CurrentLSB = 0x0F              // Current least significant byte
//const LTC2944CurrentThresholdHighMSB = 0x10 // Current High Treshold most significant byte
//const LTC2944CurrentThresholdHighLSB = 0x11 // Current High Treshold least significant byte
//const LTC2944CurrentThresholdLowMSB = 0x12  // Current Low Treshold most significant byte
//const LTC2944CurrentThresholdLowLSB = 0x13  // Current Low Treshold least significant byte
const LTC2944TempMSB = 0x14 // Temperature most significant byte
//const LTC2944TempLSB = 0x15                 // Temperature least significant byte
//const LTC2944TempThresholdHighMSB = 0x16    // Temperature High Treshold most significant byte
//const LTC2944TempThresholdHighLSB = 0x17    // Temperature High Treshold least significant byte

// LTC2944 status register bits
//const LTC2944StatusCurrent = 0x40    // Indicates one of the current limits was exceeded
//const LTC2944StatusCharge = 0x20     // Indicates that the value of the ACR hit either top or bottom
//const LTC2944StatusTemp = 0x10       // Indicates one of the temperature limits was exceeded
//const LTC2944StatusChargeHigh = 0x08 // Indicates that the ACR value exceeded the charge threshold high limit
//const LTC2944StatusChargeLow = 0x04  // Indicates that the ACR value exceeded the charge threshold low limit
//const LTC2944StatusVoltage = 0x02    // Indicates one of the voltage limits was exceeded
//const LTC2944StatusLockout = 0x01    // Indicates recovery from undervoltage. If set to 1, a UVLO has occurred and the contents of the registers are uncertain

// LTC2944 Control register values
// ADC modes
//const LTC2944ADCAutomatic = 0xC0 // Automatic Mode: continuously performing voltage, current and temperature conversions
//const LTC2944ADCScan = 0x80      // Scan Mode: performing voltage, current and temperature conversion every 10s
//const LTC2944ADCManual = 0x04    // Manual Mode: performing single conversions of voltage, current and temperature then sleep
//const LTC2944ADCSleep = 0x00     // Sleep
// Prescaler
//const LTC2944Prescale1 = 0x00    // Coulomb counter prescale = 1
//const LTC2944Prescale4 = 0x08    // Coulomb counter prescale = 4
//const LTC2944Prescale16 = 0x10   // Coulomb counter prescale = 16
//const LTC2944Prescale64 = 0x18   // Coulomb counter prescale = 64
//const LTC2944Prescale256 = 0x20  // Coulomb counter prescale = 256
//const LTC2944Prescale1024 = 0x28 // Coulomb counter prescale = 1024
//const LTC2944Prescale4096 = 0x30 // Coulomb counter prescale = 4096
//const LTC2944PrescaleMax = 0x38  // Coulomb counter prescale = 4096 (Default)
// ALCC pin configuration
//const LTC2944ALCCDisabled = 0x00   // ALCC pin disabled.
//const LTC2944ALCCAlert = 0x04      // Alert Mode. Alert functionality enabled. Pin becomes logic output.
//const LTC2944ALCCCharge = 0x02     // Charge Complete Mode. Pin becomes logic input and accepts charge complete inverted signal (e.g., from a charger) to set accumulated charge register (C,D) to FFFFh.
//const LTC2944ALCCNotAllowed = 0x06 // This value is not allowed!
// Power down
//const LTC2944Shutdown = 0x01 // Shut down analog section to reduce ISUPPLY.

func Round(val float64, roundOn float64, places int) (newVal float64) {
	var round float64
	pow := math.Pow(10, float64(places))
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	newVal = round / pow
	return
}

func New(connection spi.Conn, length int) *LTC6813 {
	ltc := new(LTC6813)
	ltc.packet = make([]byte, 4+(length*8))
	ltc.readings = make([]LTC6813Reading, length)
	ltc.chainLength = length
	ltc.spi = connection
	ltc.temperatureSensor = 0
	//	ltc.lastCommand = int64(0)
	return ltc
}

/**
Get the number of LTC6813s in the chain.
*/
func (this *LTC6813) GetChainLength() int {
	return len(this.readings)
}

/**
Clear the data packet to all zeros
*/
func (this *LTC6813) clearPacket() {
	this.packet = make([]byte, len(this.packet))
	for bank := 0; bank < this.chainLength; bank++ {
		this.setData(bank, 0, 0, 0, 0, 0, 0)
	}
}

/**
Calculate the PEC for the given data block
*/
func (this *LTC6813) calculatePEC(data []byte) uint16 {
	var remainder uint16 = 16
	var crcTable = [256]uint16{
		0x0000, 0xc599, 0xceab, 0x0b32, 0xd8cf, 0x1d56, 0x1664, 0xd3fd, 0xf407, 0x319e, 0x3aac, 0xff35, 0x2cc8, 0xe951, 0xe263, 0x27fa,
		0xad97, 0x680e, 0x633c, 0xa6a5, 0x7558, 0xb0c1, 0xbbf3, 0x7e6a, 0x5990, 0x9c09, 0x973b, 0x52a2, 0x815f, 0x44c6, 0x4ff4, 0x8a6d,
		0x5b2e, 0x9eb7, 0x9585, 0x501c, 0x83e1, 0x4678, 0x4d4a, 0x88d3, 0xaf29, 0x6ab0, 0x6182, 0xa41b, 0x77e6, 0xb27f, 0xb94d, 0x7cd4,
		0xf6b9, 0x3320, 0x3812, 0xfd8b, 0x2e76, 0xebef, 0xe0dd, 0x2544, 0x02be, 0xc727, 0xcc15, 0x098c, 0xda71, 0x1fe8, 0x14da, 0xd143,
		0xf3c5, 0x365c, 0x3d6e, 0xf8f7, 0x2b0a, 0xee93, 0xe5a1, 0x2038, 0x07c2, 0xc25b, 0xc969, 0x0cf0, 0xdf0d, 0x1a94, 0x11a6, 0xd43f,
		0x5e52, 0x9bcb, 0x90f9, 0x5560, 0x869d, 0x4304, 0x4836, 0x8daf, 0xaa55, 0x6fcc, 0x64fe, 0xa167, 0x729a, 0xb703, 0xbc31, 0x79a8,
		0xa8eb, 0x6d72, 0x6640, 0xa3d9, 0x7024, 0xb5bd, 0xbe8f, 0x7b16, 0x5cec, 0x9975, 0x9247, 0x57de, 0x8423, 0x41ba, 0x4a88, 0x8f11,
		0x057c, 0xc0e5, 0xcbd7, 0x0e4e, 0xddb3, 0x182a, 0x1318, 0xd681, 0xf17b, 0x34e2, 0x3fd0, 0xfa49, 0x29b4, 0xec2d, 0xe71f, 0x2286,
		0xa213, 0x678a, 0x6cb8, 0xa921, 0x7adc, 0xbf45, 0xb477, 0x71ee, 0x5614, 0x938d, 0x98bf, 0x5d26, 0x8edb, 0x4b42, 0x4070, 0x85e9,
		0x0f84, 0xca1d, 0xc12f, 0x04b6, 0xd74b, 0x12d2, 0x19e0, 0xdc79, 0xfb83, 0x3e1a, 0x3528, 0xf0b1, 0x234c, 0xe6d5, 0xede7, 0x287e,
		0xf93d, 0x3ca4, 0x3796, 0xf20f, 0x21f2, 0xe46b, 0xef59, 0x2ac0, 0x0d3a, 0xc8a3, 0xc391, 0x0608, 0xd5f5, 0x106c, 0x1b5e, 0xdec7,
		0x54aa, 0x9133, 0x9a01, 0x5f98, 0x8c65, 0x49fc, 0x42ce, 0x8757, 0xa0ad, 0x6534, 0x6e06, 0xab9f, 0x7862, 0xbdfb, 0xb6c9, 0x7350,
		0x51d6, 0x944f, 0x9f7d, 0x5ae4, 0x8919, 0x4c80, 0x47b2, 0x822b, 0xa5d1, 0x6048, 0x6b7a, 0xaee3, 0x7d1e, 0xb887, 0xb3b5, 0x762c,
		0xfc41, 0x39d8, 0x32ea, 0xf773, 0x248e, 0xe117, 0xea25, 0x2fbc, 0x0846, 0xcddf, 0xc6ed, 0x0374, 0xd089, 0x1510, 0x1e22, 0xdbbb,
		0x0af8, 0xcf61, 0xc453, 0x01ca, 0xd237, 0x17ae, 0x1c9c, 0xd905, 0xfeff, 0x3b66, 0x3054, 0xf5cd, 0x2630, 0xe3a9, 0xe89b, 0x2d02,
		0xa76f, 0x62f6, 0x69c4, 0xac5d, 0x7fa0, 0xba39, 0xb10b, 0x7492, 0x5368, 0x96f1, 0x9dc3, 0x585a, 0x8ba7, 0x4e3e, 0x450c, 0x8095,
	}
	for _, b := range data {
		addr := byte(remainder>>7) ^ b
		remainder = (remainder << 8) ^ crcTable[addr]
	}
	return (remainder * 2)
}

/**
Send a command to the LTC6813 chain
*/
func (this *LTC6813) sendCommand() error {
	dummyPacket := make([]byte, 1) // This is needed to wake up the ISOSPI connection due to the
	for device := 0; device < this.chainLength; device++ {
		if err := this.spi.Tx(dummyPacket, dummyPacket); err != nil {
			log.Println(err)
		} // timing requirement between CS going low and the first clock pulse.
	}
	return this.spi.Tx(this.packet, this.packet)
}

/**
Returns a slice containing the data packet for the given bank
*/
func (this *LTC6813) getData(bank int) []byte {
	return this.packet[((bank * 8) + 4):((bank * 8) + 10)]
}

/**
Calculate the PEC for the command
*/
func (this *LTC6813) calculateCmdPEC() uint16 {
	return this.calculatePEC(this.packet[0:2])
}

/**
If the last command was sent 400ms or more ago then send a dummy read of the configuration A register to wake up the chain
*/
func (this *LTC6813) wakeUp() error {
	return nil
	/*
		var err error
		if time.Since(this.lastCommand).Nanoseconds() > 50000000 {
			dummyPacket := make([]byte, 1)                // This is needed to wake up the ISOSPI connection due to the
			for device := 0; device < this.chainLength; device++ {
				this.spi.Tx(dummyPacket, dummyPacket) 		// timing requirement between CS going low and the first clock pulse.
			}
		} else {
			err = nil
		}
		this.lastCommand = time.Now()
		return err
	*/
}

/**
Calculate the PEC for the data block for the given bank.
*/
func (this *LTC6813) calculateDataPEC(bank int) uint16 {
	return this.calculatePEC(this.getData(bank))
}

/**
Return the PEC for the command
*/
func (this *LTC6813) getCmdPEC() uint16 {
	return binary.BigEndian.Uint16(this.packet[2:4])
}

/**
Return the PEC for the data block for the given bank
*/
func (this *LTC6813) getDataPEC(bank int) uint16 {
	return binary.BigEndian.Uint16(this.packet[(bank*8)+10 : (bank*8)+12])
}

/**
Sets the command and its PEC
*/
func (this *LTC6813) setCommand(cmd uint16) {
	binary.BigEndian.PutUint16(this.packet[0:], cmd)
	binary.BigEndian.PutUint16(this.packet[2:], this.calculateCmdPEC())
}

/**
Sets the given banks data block complete with PEC
*/
func (this *LTC6813) setData(bank int, b0 byte, b1 byte, b2 byte, b3 byte, b4 byte, b5 byte) {
	this.packet[(bank*8)+4] = b0
	this.packet[(bank*8)+5] = b1
	this.packet[(bank*8)+6] = b2
	this.packet[(bank*8)+7] = b3
	this.packet[(bank*8)+8] = b4
	this.packet[(bank*8)+9] = b5
	binary.BigEndian.PutUint16(this.packet[((bank*8)+10):], this.calculatePEC(this.getData(bank)))
}

/**
Checks the PEC for the command and each banks data block
*/
func (this *LTC6813) checkPEC(sError string, bCheckCmd bool) error {
	if bCheckCmd {
		if this.getCmdPEC() != this.calculateCmdPEC() {
			return fmt.Errorf("PEC error in Command")
		}
	}
	for i := 0; i < this.chainLength; i++ {
		if this.getDataPEC(i) != this.calculateDataPEC(i) {

			return fmt.Errorf("PEC error in data block %d %x [%x] %s", i, this.getData(0), this.getData(i), sError)
		}
	}
	return nil
}

/* Read one bank from all devices in the chain
 */
func (this *LTC6813) readADCInputBank(nBank int) (int, error) {
	var cmd uint16
	var sErrorHelp string = "ReadADCInputBank "

	switch nBank {
	case 0:
		cmd = RDCVA
		sErrorHelp = sErrorHelp + "0 - Cell Volts A"
	case 1:
		cmd = RDCVB
		sErrorHelp = sErrorHelp + "1 - Cell Volts B"
	case 2:
		cmd = RDCVC
		sErrorHelp = sErrorHelp + "2 - Cell Volts C"
	case 3:
		cmd = RDCVD
		sErrorHelp = sErrorHelp + "3 - Cell Volts D"
	case 4:
		cmd = RDCVE
		sErrorHelp = sErrorHelp + "4 - Cell Volts D"
	case 5:
		cmd = RDCVF
		sErrorHelp = sErrorHelp + "5 - Cell Volts D"
	case 6:
		cmd = RDAUXA
		sErrorHelp = sErrorHelp + "6 - Auxilliary A"
	case 7:
		cmd = RDAUXB
		sErrorHelp = sErrorHelp + "7 - Auxilliary B"
	case 8:
		cmd = RDAUXC
		sErrorHelp = sErrorHelp + "8 - Auxilliary C"
	case 9:
		cmd = RDAUXD
		sErrorHelp = sErrorHelp + "9 - Auxilliary D"
	default:
		cmd = RDSTATA
		sErrorHelp = sErrorHelp + "10 - Status A"
	}

	this.clearPacket()
	this.setCommand(cmd)

	for set := 0; set < this.chainLength; set++ {
		this.packet[(set*8)+4] = 0xff
		this.packet[(set*8)+5] = 0xff
		this.packet[(set*8)+6] = 0xff
		this.packet[(set*8)+7] = 0xff
		this.packet[(set*8)+8] = 0xff
		this.packet[(set*8)+9] = 0xff
		binary.BigEndian.PutUint16(this.packet[((set*8)+10):], this.calculateDataPEC(set))
	}

	if err := this.sendCommand(); err != nil {
		return 0, err
	}

	if nBank < 11 {
		this.dmu.Lock()
		defer this.dmu.Unlock()
		for set := 0; set < this.chainLength; set++ {
			switch nBank {
			case 0:
				this.readings[set].CellVolts[0] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
				this.readings[set].CellVolts[1] = binary.LittleEndian.Uint16(this.getData(set)[2:4])
				this.readings[set].CellVolts[2] = binary.LittleEndian.Uint16(this.getData(set)[4:])
			case 1:
				this.readings[set].CellVolts[3] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
				this.readings[set].CellVolts[4] = binary.LittleEndian.Uint16(this.getData(set)[2:4])
				this.readings[set].CellVolts[5] = binary.LittleEndian.Uint16(this.getData(set)[4:])
			case 2:
				this.readings[set].CellVolts[6] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
				this.readings[set].CellVolts[7] = binary.LittleEndian.Uint16(this.getData(set)[2:4])
				this.readings[set].CellVolts[8] = binary.LittleEndian.Uint16(this.getData(set)[4:])
			case 3:
				this.readings[set].CellVolts[9] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
				this.readings[set].CellVolts[10] = binary.LittleEndian.Uint16(this.getData(set)[2:4])
				this.readings[set].CellVolts[11] = binary.LittleEndian.Uint16(this.getData(set)[4:])
			case 4:
				this.readings[set].CellVolts[12] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
				this.readings[set].CellVolts[13] = binary.LittleEndian.Uint16(this.getData(set)[2:4])
				this.readings[set].CellVolts[14] = binary.LittleEndian.Uint16(this.getData(set)[4:])
			case 5:
				this.readings[set].CellVolts[15] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
				this.readings[set].CellVolts[16] = binary.LittleEndian.Uint16(this.getData(set)[2:4])
				this.readings[set].CellVolts[17] = binary.LittleEndian.Uint16(this.getData(set)[4:])
			case 6:
				this.readings[set].GPIOVolts[0] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
				this.readings[set].GPIOVolts[1] = binary.LittleEndian.Uint16(this.getData(set)[2:4])
				this.readings[set].GPIOVolts[2] = binary.LittleEndian.Uint16(this.getData(set)[4:])
			case 7:
				this.readings[set].GPIOVolts[3] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
				this.readings[set].GPIOVolts[4] = binary.LittleEndian.Uint16(this.getData(set)[2:4])
				this.readings[set].RefVolts = binary.LittleEndian.Uint16(this.getData(set)[4:])
			case 8:
				this.readings[set].GPIOVolts[5] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
				this.readings[set].GPIOVolts[6] = binary.LittleEndian.Uint16(this.getData(set)[2:4])
				this.readings[set].GPIOVolts[7] = binary.LittleEndian.Uint16(this.getData(set)[4:])
			case 9:
				this.readings[set].GPIOVolts[8] = binary.LittleEndian.Uint16(this.getData(set)[0:2])
			default:
				this.readings[set].SumOfCells = binary.LittleEndian.Uint16(this.getData(set)[0:2])
			}
		}
	}
	return this.GetChainLength(), this.checkPEC(sErrorHelp, false)
}

/**
Read all the ADC inputs
*/
func (this *LTC6813) readADCInputs() (int, error) {
	for bank := 0; bank < 6; bank++ {
		_, err := this.readADCInputBank(bank)
		if err != nil {
			return 0, err
		}
	}

	return this.GetChainLength(), nil
}

/**
Read all the ADC inputs and Sum of Cells
*/
func (this *LTC6813) readADCInputsSC() (int, error) {
	for bank := 0; bank < 6; bank++ {
		_, err := this.readADCInputBank(bank)
		if err != nil {
			return 0, err
		}
	}
	_, err := this.readADCInputBank(10)
	if err != nil {
		return 0, err
	}
	return this.GetChainLength(), nil
}

/**
Read all the ADC inputs + GPIO 1 & 2
*/
func (this *LTC6813) readADCAXInputs() (int, error) {
	for bank := 0; bank < 7; bank++ {
		_, err := this.readADCInputBank(bank)
		if err != nil {
			return 0, err
		}
	}
	return this.GetChainLength(), nil
}

/**
Read the GPIO analogue readings for a bank
*/
func (this *LTC6813) readGPIOADCInputs() (int, error) {
	for bank := 6; bank < 10; bank++ {
		_, err := this.readADCInputBank(bank)
		if err != nil {
			return 0, err
		}
	}

	return this.GetChainLength(), nil
}

/**
Set up the LTC6813 chain
*/
func (this *LTC6813) Initialise() error {
	this.mu.Lock()
	defer this.mu.Unlock()
	// Wake up the chain...
	this.clearPacket()
	this.setCommand(WRCFGA)
	for i := 0; i < this.chainLength; i++ {
		this.setData(i,
			ADC_OPTION_0+DISCHARGE_DISABLED+REF_ON+GPIO1_PULL_DOWN_OFF+GPIO2_PULL_DOWN_OFF+GPIO3_PULL_DOWN_OFF+GPIO4_PULL_DOWN_OFF+GPIO5_PULL_DOWN_OFF,
			0, 0, 0, 0, 0)
	}
	if err := this.sendCommand(); err != nil {
		return err
	}

	this.clearPacket()
	this.setCommand(WRCFGB)
	for i := 0; i < this.chainLength; i++ {
		this.setData(i, GPIO6_PULL_DOWN_OFF+GPIO7_PULL_DOWN_OFF+GPIO8_PULL_DOWN_OFF+GPIO9_PULL_DOWN_OFF, 0, 0, 0, 0, 0)
	}
	if err := this.sendCommand(); err != nil {
		return err
	}
	return nil
}

/**
Start the analogue conversion for the GPIO inputs
*/
func (this *LTC6813) startGPIOConversion(mode uint16) error {
	this.clearPacket()
	this.setCommand(ADAX + mode)

	if err := this.sendCommand(); err != nil {
		return err
	}
	return nil
}

/**
Start the analogue conversion for all channels
*/
func (this *LTC6813) startConversion(mode uint16) error {
	this.clearPacket()
	this.setCommand(ADCV + mode + DCP_Permitted)

	if err := this.sendCommand(); err != nil {
		return err
	}
	return nil
}

/**
Start the analogue conversion for all channels and the sum of cells
*/
func (this *LTC6813) startConversionSC(mode uint16) error {
	this.clearPacket()
	this.setCommand(ADCVSC + mode + DCP_Permitted)

	if err := this.sendCommand(); err != nil {
		return err
	}
	return nil
}

/**
Start the cell measurement and the two direct temperature inputs
*/
func (this *LTC6813) startAxConversion(mode uint16) error {
	this.clearPacket()
	this.setCommand(ADCVAX + mode + DCP_Permitted)

	if err := this.sendCommand(); err != nil {
		return err
	}
	return nil
}

/**
Read the configuration register into the data block
*/
func (this *LTC6813) readConfigRegister() error {

	this.clearPacket()
	this.setCommand(RDCFGA)
	for i := 0; i < this.chainLength; i++ {
		this.setData(i, 0, 0, 0, 0, 0, 0)
	}
	if err := this.sendCommand(); err != nil {
		return err
	}
	return nil
}

/*
Temperature sensors 0..7 and 8..16 are addressed by setting the GPIO output ports 7, 8 & 9 to the relative address
*/
func (this *LTC6813) setTemperatureSensor(sensor int8) error {
	this.clearPacket()
	this.setCommand(WRCFGB)
	for i := 0; i < this.chainLength; i++ {
		this.setData(i, byte((sensor*2)+1), 0, 0, 0, 0, 0)
	}
	err := this.sendCommand()
	if err != nil {
		this.clearPacket()
		this.setCommand(WRCFGB)
		for i := 0; i < this.chainLength; i++ {
			this.setData(i, byte((sensor*2)+1), 0, 0, 0, 0, 0)
		}
	}
	return err
}

/**
Update the temperature value from the ADC input data received.
*/
func (this *LTC6813) updateTemperature() {
	this.dmu.Lock()
	defer this.dmu.Unlock()
	for b := range this.readings {
		t, err := calculateTemperature(this.readings[b].GPIOVolts[0])
		if err == nil {
			this.readings[b].temperatures[this.temperatureSensor] = t
		} else {
			this.readings[b].temperatures[this.temperatureSensor] = -273.0
		}
		t, err = calculateTemperature(this.readings[b].GPIOVolts[1])
		if err == nil {
			this.readings[b].temperatures[this.temperatureSensor+8] = t
		} else {
			this.readings[b].temperatures[this.temperatureSensor+8] = -273.0
		}
	}
}

/**
Start a complete voltage measurement cycle
*/
func (this *LTC6813) MeasureVoltages() (int, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	err := this.startConversion(ADC_MODE_FILTERED)
	if err != nil {
		return 0, err
	}
	time.Sleep(time.Millisecond * 300)
	i, err := this.readADCInputs()
	return i, err
}

/**
Start a complete voltage measurement cycle including sum of cells
*/
func (this *LTC6813) MeasureVoltagesSC() (int, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	err := this.startConversionSC(ADC_MODE_FILTERED)
	if err != nil {
		return 0, err
	}
	time.Sleep(time.Millisecond * 350)
	i, err := this.readADCInputsSC()
	if err != nil {
		this.lastVoltageError = "Voltage Error : " + err.Error()
	} else {
		this.lastVoltageError = ""
	}
	return i, err
}

/**
Measure the voltages and temperatures. Each operation measures only four temperatures with the
first two cycling through all 16 possibilities each invocation.
*/
func (this *LTC6813) MeasureVoltagesAndAux() (int, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	err := this.startAxConversion(ADC_MODE_FILTERED)
	if err != nil {
		return 0, err
	}
	time.Sleep(time.Millisecond * 400)
	i, err := this.readADCAXInputs()
	if err == nil {
		this.updateTemperature()
	}
	this.temperatureSensor += 1
	if this.temperatureSensor == 8 {
		this.temperatureSensor = 0
	}
	if err := this.setTemperatureSensor(this.temperatureSensor); err != nil {
		log.Println(err)
	}
	return i, err
}

/**
Measure the temperatures
*/
func (this *LTC6813) MeasureTemperatures() (int, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	err := this.startGPIOConversion(ADC_MODE_FAST)
	if err != nil {
		return 0, err
	}
	time.Sleep(time.Millisecond * 50)
	i, err := this.readGPIOADCInputs()
	this.dmu.Lock()
	defer this.dmu.Unlock()
	for b := range this.readings {
		t, err := calculateTemperature(this.readings[b].GPIOVolts[0])
		if err == nil {
			this.readings[b].temperatures[this.temperatureSensor] = float32(t)
		} else {
			this.readings[b].temperatures[this.temperatureSensor] = -273.0
		}
		t, err = calculateTemperature(this.readings[b].GPIOVolts[1])
		if err == nil {
			this.readings[b].temperatures[this.temperatureSensor+8] = float32(t)
		} else {
			this.readings[b].temperatures[this.temperatureSensor+8] = -273.0
		}
		t, err = calculateTemperature(this.readings[b].GPIOVolts[2])
		if err == nil {
			this.readings[b].temperatures[16] = float32(t)
		} else {
			this.readings[b].temperatures[16] = -273.0
		}
		t, err = calculateTemperature(this.readings[b].GPIOVolts[5])
		if err == nil {
			this.readings[b].temperatures[17] = float32(t)
		} else {
			this.readings[b].temperatures[17] = -273.0
		}
	}
	this.temperatureSensor += 1
	if this.temperatureSensor == 8 {
		this.temperatureSensor = 0
	}
	if err := this.setTemperatureSensor(this.temperatureSensor); err != nil {
		log.Println(err)
	}
	if err != nil {
		this.lastTempError = "Temperature Error : " + err.Error()
	} else {
		this.lastTempError = ""
	}
	return i, err
}

/**
Return the cell voltage for a given cell.
*/
func (this *LTC6813) GetVolts(bank int, cell int) float32 {
	this.dmu.Lock()
	defer this.dmu.Unlock()
	if bank < this.chainLength {
		return float32(this.readings[bank].CellVolts[cell]) / 10000.0
	} else {
		return 0.0
	}
}

/**
Return the cell voltage for a given cell.
*/
func (this *LTC6813) GetRawVolts(bank int, cell int) uint16 {
	this.dmu.Lock()
	defer this.dmu.Unlock()
	if bank < this.chainLength {
		return this.readings[bank].CellVolts[cell]
	} else {
		return 0
	}
}

/**
Convert a voltage measurement to a temperature value
*/
func calculateTemperature(t uint16) (float32, error) {
	if (t > 28000) || (t < 100) {
		return -273.15, fmt.Errorf("ERR")
	} else {
		return float32(Round((1.0/((math.Log(1/((30000.0/float64(t))-1))/BCOEFFICIENT)+(0.003354)))-273.15, 0.5, 1)), nil
	}
}

/**
Return the temperature for the given sensor
*/
//func (this *LTC6813) getTemperature(bank int, sensor int) (float64, error) {
//	return calculateTemperature(this.readings[bank].GPIOVolts[sensor])
//}

/**
public implementation of the get temperature function correcting for over and under range values
*/
func (this *LTC6813) GetTemperature(bank int, sensor int) (float32, error) {
	this.dmu.Lock()
	defer this.dmu.Unlock()

	if bank < this.chainLength {
		if this.readings[bank].temperatures[sensor] < -100.0 {
			return this.readings[bank].temperatures[sensor], fmt.Errorf("ERR")
		}
		if this.readings[bank].temperatures[sensor] > 100.0 {
			return this.readings[bank].temperatures[sensor], fmt.Errorf("ERR")
		}
		return this.readings[bank].temperatures[sensor], nil
	} else {
		return 0.0, nil
	}
}

/**
Return the temperature as an integer value degrees C * 10
*/
func (this *LTC6813) GetTemp(bank int, sensor int) int16 {
	this.dmu.Lock()
	defer this.dmu.Unlock()

	t := int16(0)
	if bank < this.chainLength {
		t = int16(this.readings[bank].temperatures[sensor] * 10)
	}
	switch {
	case (t < -100):
		return -32768
	case (t > 1100):
		return 32767
	default:
		return t
	}
}

/**
Get the reference voltage value
*/
func (this *LTC6813) GetRefVolts(bank int) float32 {
	this.dmu.Lock()
	defer this.dmu.Unlock()
	if bank < this.chainLength {
		return float32(this.readings[bank].RefVolts) / 10000.0
	} else {
		return 0.0
	}
}

/**
Get the sum of cells voltage value
*/
func (this *LTC6813) GetSumOfCellsVolts(bank int) float32 {
	this.dmu.Lock()
	defer this.dmu.Unlock()
	if bank < this.chainLength {
		return (float32(this.readings[bank].SumOfCells) / 10000.0) * 30
	} else {
		return 0.0
	}
}

/**
Get the raw sum of cells voltage value
*/
func (this *LTC6813) GetRawSumOfCellsVolts(bank int) uint16 {
	this.dmu.Lock()
	defer this.dmu.Unlock()
	if bank < this.chainLength {
		return (this.readings[bank].SumOfCells / 10) * 3
	} else {
		return 0
	}
}

/**
Get the GPIO measured voltage
*/
func (this *LTC6813) GetGPIOVolts(bank int, sensor int) uint16 {
	this.dmu.Lock()
	defer this.dmu.Unlock()
	if bank < this.chainLength {
		return this.readings[bank].GPIOVolts[sensor]
	} else {
		return 0
	}
}

/**
Returns a JSON data object containing the 18 cell voltages
*/
func (this *LTC6813) GetVoltagesAsJSON() string {
	var values struct {
		Voltages [2][]uint16 `json:"voltages"`
		Totals   [2]float32  `json:"totals"`
	}

	//	values.Voltages = make([][]uint16, 2)
	values.Voltages[0] = append(append(this.GetCellVolts(0), this.GetCellVolts(1)...), this.GetCellVolts(2)...)
	values.Voltages[1] = append(append(this.GetCellVolts(3), this.GetCellVolts(4)...), this.GetCellVolts(5)...)
	values.Totals[0] = float32(Round((float64(this.GetSumOfCellsVolts(0))*0.003)+(float64(this.GetSumOfCellsVolts(1))*0.003)+(float64(this.GetOneCellVolts(2, 0))/10000.0)+(float64(this.GetOneCellVolts(2, 1))/10000.0), 0.5, 2))
	values.Totals[1] = float32(Round((float64(this.GetSumOfCellsVolts(3))*0.003)+(float64(this.GetSumOfCellsVolts(4))*0.003)+(float64(this.GetOneCellVolts(5, 0))/10000.0)+(float64(this.GetOneCellVolts(5, 1))/10000.0), 0.5, 2))
	s, err := json.Marshal(values)
	if err != nil {
		fmt.Println("Error marshalling the voltages to JSON - ", s)
		return ""
	}
	return string(s)
}

/**
GetOneCellVolts returns a single cell voltage measurement
*/
func (this *LTC6813) GetOneCellVolts(bank int, cell int) uint16 {
	if bank < this.chainLength {
		return this.readings[bank].CellVolts[cell]
	} else {
		return 0
	}
}

/**
GetCellVolts returns the array of cell voltages for the given bank
*/
func (this *LTC6813) GetCellVolts(bank int) []uint16 {
	if bank < this.chainLength {
		return this.readings[bank].CellVolts[0:18]
	} else {
		return make([]uint16, 18)
	}
}

/**
Returns a JSON object containing the 18 temperature values
*/
func (this *LTC6813) GetTemperaturesAsJSON() string {
	var values struct {
		Temperatures [2][]float32 `json:"temperatures"`
	}
	//	retString := `temperatures":[`
	this.dmu.Lock()
	defer this.dmu.Unlock()

	values.Temperatures[0] = append(append(this.GetBankTemperatures(0), this.GetBankTemperatures(1)...), this.GetBankTemperatures(2)[0:2]...)
	values.Temperatures[1] = append(append(this.GetBankTemperatures(3), this.GetBankTemperatures(4)...), this.GetBankTemperatures(5)[0:2]...)

	s, err := json.Marshal(values)
	if err != nil {
		log.Println("Error marshalling the temperatures to JSON - ", err)
		return ""
	}
	return string(s)
}

func (this *LTC6813) GetBankTemperatures(bank int) []float32 {
	if bank < this.chainLength {
		return this.readings[bank].temperatures[0:18]
	} else {
		return make([]float32, 18)
	}
}

/**
Returns a JSON object with all values for Current and Temperature inside
*/
func (this *LTC6813) GetValuesAsJSON() []byte {
	var values struct {
		VoltageError     string       `json:"voltage_error"`
		TemperatureError string       `json:"temperature_error"`
		Voltages         [2][]uint16  `json:"voltages"`
		Totals           [2]float32   `json:"totals"`
		Temperatures     [2][]float32 `json:"temperatures"`
	}
	values.VoltageError = this.lastVoltageError
	values.TemperatureError = this.lastTempError
	values.Voltages[0] = append(append(this.GetCellVolts(0), this.GetCellVolts(1)...), this.GetCellVolts(2)[0:2]...)
	values.Voltages[1] = append(append(this.GetCellVolts(3), this.GetCellVolts(4)...), this.GetCellVolts(5)[0:2]...)
	//	values.Totals[0] = float32(Round((float64(this.GetSumOfCellsVolts(0))*0.003)+(float64(this.GetSumOfCellsVolts(1))*0.003)+(float64(this.GetRawVolts(2, 0))/10000.0)+(float64(this.GetRawVolts(2, 1))/10000.0), 0.5, 2))
	//	values.Totals[1] = float32(Round((float64(this.GetSumOfCellsVolts(3))*0.003)+(float64(this.GetSumOfCellsVolts(4))*0.003)+(float64(this.GetRawVolts(5, 0))/10000.0)+(float64(this.GetRawVolts(5, 1))/10000.0), 0.5, 2))
	values.Totals[0] = this.GetSumOfCellsVolts(0) + this.GetSumOfCellsVolts(1) + float32(this.GetRawVolts(2, 0))/10000.0 + (float32(this.GetRawVolts(2, 1)) / 10000.0)
	values.Totals[1] = this.GetSumOfCellsVolts(3) + this.GetSumOfCellsVolts(4) + float32(this.GetRawVolts(5, 0))/10000.0 + (float32(this.GetRawVolts(5, 1)) / 10000.0)
	values.Temperatures[0] = append(append(this.GetBankTemperatures(0), this.GetBankTemperatures(1)...), this.GetBankTemperatures(2)[0:2]...)
	values.Temperatures[1] = append(append(this.GetBankTemperatures(3), this.GetBankTemperatures(4)...), this.GetBankTemperatures(5)[0:2]...)

	j, err := json.Marshal(values)
	if err != nil {
		log.Println("Error getting values as JSON - ", err)
		return []byte(err.Error())
	}
	return j
}

func (this *LTC6813) GetMaxTemperature() (tMax float32) {
	tMax = 0.0
	for _, reading := range this.readings {
		for _, temp := range reading.temperatures {
			if temp > tMax {
				tMax = temp
			}
		}
	}
	return
}

func (this *LTC6813) Test() (int, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.readADCInputBank(0)
}

/**
Write a single byte to the given I2C device using the given command
*/
func (this *LTC6813) WriteI2CByte(bank int, address uint8, command uint8, data uint8) (int, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.clearPacket()
	this.setData(bank, I2CStart+(address>>4), ((address << 4) & 0xf0), ((command >> 4) & 0x0f), ((command << 4) & 0xf0), ((data >> 4) & 0x0f), ((data<<4)&0xf0)+I2CNackStop)
	this.setCommand(WRCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}
	this.clearPacket()
	this.setCommand(STCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}
	return 0, nil
}

/**
Write a single 16 bit word to the given I2C device using the given command
*/
func (this *LTC6813) WriteI2CWord(address uint8, command uint8, data uint16) (int, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.clearPacket()
	for b := range this.readings {
		this.setData(b, I2CStart+(address>>4), ((address << 4) & 0xf0), (command >> 4), (command << 4), byte(data>>4), byte(data<<4)+I2CNackStop)
	}
	this.setCommand(WRCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}
	//	this.clearPacket()
	//	this.setCommand(STCOMM)
	//	this.sendCommand()
	//	this.clearPacket()
	//	this.setData(0, byte((data >> 12) & 0x0f), byte((data >> 4) &0xf0) + I2CStop, I2CNoTransmit, I2CNoTransmit, I2CNoTransmit, I2CNoTransmit)
	return 0, nil
}

/**
Get a single byte of data from the specified bank at the byte offset given
*/
func (this *LTC6813) getByte(bank int, offset int) uint8 {
	return this.getData(bank)[offset*2]<<4 + this.getData(bank)[(offset*2)+1]>>4
}

/**
Read a single byte from the given I2C device using the given command
*/
func (this *LTC6813) ReadI2CByte(bank int, address uint8, command uint8) (string, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.clearPacket()
	for b := range this.readings {
		this.setData(b, I2CStart+(address>>4), ((address<<4)&0xf0)+I2CWRITE, (command >> 4), (command << 4), I2CNoTransmit, 0)
	}
	this.setCommand(WRCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	this.setCommand(STCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	for b := range this.readings {
		this.setData(b, I2CStart+(address>>4), ((address<<4)&0xf0)+I2CREAD, 0x0F, 0x0F+I2CNackStop, I2CNoTransmit, 0)
	}
	this.setCommand(WRCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	this.setCommand(STCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	this.setCommand(RDCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	err := this.checkPEC("I2C Communication Error(PEC)", false)
	if err != nil {
		return "Failed", err
	} else {
		return fmt.Sprintf("Data from register %d of the LTC2944 - %x | 0x%x", command, this.getData(bank), this.getByte(bank, 1)), nil
	}
}

/**
Read a single 16 bit word from the given I2C device using the given command
*/
func (this *LTC6813) ReadI2CWord(bank int, address uint8, command uint8) (string, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.clearPacket()
	for b := range this.readings {
		this.setData(b, I2CStart+(address>>4), ((address<<4)&0xf0)+I2CWRITE, (command >> 4), (command << 4), I2CNoTransmit, 0)
	}
	this.setCommand(WRCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	this.setCommand(STCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	for b := range this.readings {
		this.setData(b, I2CStart+(address>>4), ((address<<4)&0xf0)+I2CREAD, 0x0F, 0xF0+I2CACK, 0x0F, 0xF0+I2CNackStop)
	}
	this.setCommand(WRCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	this.setCommand(STCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	this.setCommand(RDCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	err := this.checkPEC("I2C Communication Error(PEC)", false)
	if err != nil {
		return "Failed", err
	} else {
		return fmt.Sprintf("Data from register %d of the LTC2944 sensor %d - %x\n 0x%x 0x%x", command, bank, this.getData(bank), this.getByte(bank, 1), this.getByte(bank, 2)), nil
	}
}

/**
Read a single 16 bit word from the given I2C device at the given register and return the data
*/
func (this *LTC6813) ReadI2CWordData(bank int, address uint8, command uint8) (uint16, error) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.clearPacket()
	for b := range this.readings {
		this.setData(b, I2CStart+(address>>4), ((address<<4)&0xf0)+I2CWRITE, (command >> 4), (command << 4), I2CNoTransmit, 0)
	}
	this.setCommand(WRCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	this.setCommand(STCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	for b := range this.readings {
		this.setData(b, I2CStart+(address>>4), ((address<<4)&0xf0)+I2CREAD, 0x0F, 0xF0+I2CACK, 0x0F, 0xF0+I2CNackStop)
	}
	this.setCommand(WRCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	this.setCommand(STCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	this.clearPacket()
	this.setCommand(RDCOMM)
	if err := this.sendCommand(); err != nil {
		log.Println(err)
	}

	err := this.checkPEC("I2C Communication Error(PEC)", false)
	if err != nil {
		return 0, err
	} else {
		fmt.Println("Data = ", this.getData(bank))
		msb := this.getByte(bank, 1)
		lsb := this.getByte(bank, 2)
		fmt.Println("msb=", msb, "\nlsb=", lsb, "\n")
		v := uint16(msb)<<8 + uint16(lsb)
		return v, nil
	}
}

func (this *LTC6813) GetI2CTemp(sensor int) (float64, error) {
	v, err := this.ReadI2CWordData(sensor, LTC2944Address, LTC2944TempMSB)
	if err != nil {
		return 0.0, err
	} else {
		return ((510 * (float64(int16(v)) / 65535)) - 273.15), nil
	}
}

func (this *LTC6813) GetI2CCurrent(sensor int) (float64, error) {
	v, err := this.ReadI2CWordData(sensor, LTC2944Address, LTC2944CurrentMSB)
	if err != nil {
		return 0.0, err
	} else {
		//		return float64(int16(v)), nil
		return 0.064 * (float64(v-32767) / 32767.0), nil
		//		return (256 * (float64(v - 32767) / 32767.0)), nil
	}
}

func (this *LTC6813) GetI2CVoltage(sensor int) (float64, error) {
	v, err := this.ReadI2CWordData(sensor, LTC2944Address, LTC2944VoltageMSB)
	if err != nil {
		return 0.0, err
	} else {
		return 70.8 * (float64(v) / 65535.0), nil
	}
}

func (this *LTC6813) GetI2CAccumulatedCharge(sensor int) (float64, error) {
	v, err := this.ReadI2CWordData(sensor, LTC2944Address, LTC2944ChargeMSB)
	if err != nil {
		return 0.0, err
	} else {
		return float64(v), nil
	}
}

func (this *LTC6813) GetActiveBatteryVoltage() float32 {
	voltageLeft := ((this.GetSumOfCellsVolts(0)) + (this.GetSumOfCellsVolts(1)) + (this.GetVolts(2, 0)) + (this.GetVolts(2, 1)))
	voltageRight := ((this.GetSumOfCellsVolts(3)) + (this.GetSumOfCellsVolts(4)) + (this.GetVolts(5, 0)) + (this.GetVolts(5, 1)))

	return float32(math.Max(float64(voltageLeft), float64(voltageRight)))
}
