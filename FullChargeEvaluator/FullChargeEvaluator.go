package FullChargeEvaluator

import (
	"database/sql"
	"log"
	"time"
)

type FullChargeEval struct {
	pDB              *sql.DB
	loadDataSQL      *sql.Stmt
	getSlopeSQL      *sql.Stmt
	setFullChargeSQL *sql.Stmt
	checkFullSQL     *sql.Stmt
	systemParamsSQL  *sql.Stmt
	fullFlags        [2][38]bool
	span             int
	threshold        float64
	minRows          int64
}

func New(pDB *sql.DB) (*FullChargeEval, error) {
	fce := new(FullChargeEval)
	fce.pDB = pDB
	var err error
	fce.loadDataSQL, err = pDB.Prepare("call ChargingDataLoad(?,?,?)")
	if err != nil {
		log.Println("Failed to prepare the ChargingDataLoad statement -", err)
		return nil, err
	}
	fce.getSlopeSQL, err = pDB.Prepare("select Slope(?)")
	if err != nil {
		log.Println("Failed to prepare the select Slope statement-", err)
		return nil, err
	}
	fce.setFullChargeSQL, err = pDB.Prepare("update serial_numbers set full_charge=?, full_charge_detected=? where cell_number=?")
	if err != nil {
		log.Println("Failed to prepare the update serial_numbers statement -", err)
		return nil, err
	}
	fce.checkFullSQL, err = pDB.Prepare("select cell_number, full_charge from serial_numbers")
	if err != nil {
		log.Println("Failed to prepare the select cell_number, full_charge from serial_numbers -", err)
		return nil, err
	}
	fce.systemParamsSQL, err = pDB.Prepare("select ifnull(date_value ,ifnull(double_value, integer_value)) as value from system_parameters where name = ?")
	if err != nil {
		log.Println("Failed to prepare the select from system_parameters statement -", err)
		return nil, err
	}
	return fce, nil
}

func (this *FullChargeEval) loadFullFlags() error {
	row := this.systemParamsSQL.QueryRow("full_charge_min_rows")
	err := row.Scan(&this.minRows)
	if err != nil {
		log.Println("Failed to get full_charge_min_rows from system_parameters.", err)
		return err
	}

	row = this.systemParamsSQL.QueryRow("full_charge_scan_mins")
	err = row.Scan(&this.span)
	if err != nil {
		log.Println("Failed to get full_charge_scan_mins from system_parameters.", err)
		return err
	}

	row = this.systemParamsSQL.QueryRow("full_charge_threshold")
	err = row.Scan(&this.threshold)
	if err != nil {
		log.Println("Failed to get full_charge_threshold from system_parameters.", err)
		return err
	}

	rows, err := this.checkFullSQL.Query()
	if err != nil {
		log.Println("Failed to get the current full cell status.", err)
		return err
	}
	var cell_number uint8
	var full_charge bool
	for rows.Next() {
		if err = rows.Scan(&cell_number, &full_charge); err != nil {
			log.Println("Error getting full charge rows. ", err)
			return err
		}
		if cell_number < 100 {
			this.fullFlags[0][cell_number-1] = full_charge
		} else {
			this.fullFlags[1][cell_number-101] = full_charge
		}
	}
	return nil
}

func (this *FullChargeEval) loadTemporaryTable(when time.Time, span int, bank uint8) (rows int64, err error) {
	rows = 0
	row := this.loadDataSQL.QueryRow(when, span, bank)
	err = row.Scan(&rows)
	if err != nil {
		log.Println("Error getting row count for temporary table -", err)
	}
	return
}

func (this *FullChargeEval) getFullChargeState(cell int, threshold float64) (bool, error) {
	var slope sql.NullFloat64

	res := this.getSlopeSQL.QueryRow(cell)
	err := res.Scan(&slope)
	//	log.Println("Cell ", cell, " slope = ", slope, " threshold = ", threshold)
	if slope.Valid {
		return slope.Float64 < threshold, err
	} else {
		return false, err
	}
}

func (this *FullChargeEval) setFullChargeState(state bool, when time.Time, cell int) (err error) {
	_, err = this.setFullChargeSQL.Exec(state, when, cell)
	return
}

func (this *FullChargeEval) ProcessFullCharge(when time.Time) error {
	err := this.loadFullFlags()
	if err != nil {
		log.Println(err)
		return err
	}
	for bank := range this.fullFlags {
		rows, err := this.loadTemporaryTable(when, this.span, uint8(bank))
		if err != nil {
			log.Println("Error getting the data to process -", err)
			return err
		}
		if rows >= this.minRows {
			//			fmt.Println(rows, "rows read for bank", bank)
			for cell, flag := range this.fullFlags[bank] {
				if !flag {
					full, err := this.getFullChargeState(cell+1, this.threshold)
					if err != nil {
						log.Println("Failed to get the cell state for cell ", cell+1, " - ", err)
						return err
					}
					this.fullFlags[bank][cell] = full
					if full {
						err := this.setFullChargeState(true, when, cell+1)
						if err != nil {
							log.Println("Failed to set the cell", cell, "to full charge", err)
							return err
						}
					}
				}
			}
			//		} else {
			//			log.Println("Only", rows, "rows were found for time", when)
		}
	}
	return nil
}
