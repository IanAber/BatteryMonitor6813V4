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

func (fullChargeEvaluator *FullChargeEval) loadFullFlags() error {
	row := fullChargeEvaluator.systemParamsSQL.QueryRow("full_charge_min_rows")
	err := row.Scan(&fullChargeEvaluator.minRows)
	if err != nil {
		log.Println("Failed to get full_charge_min_rows from system_parameters.", err)
		return err
	}

	row = fullChargeEvaluator.systemParamsSQL.QueryRow("full_charge_scan_mins")
	err = row.Scan(&fullChargeEvaluator.span)
	if err != nil {
		log.Println("Failed to get full_charge_scan_mins from system_parameters.", err)
		return err
	}

	row = fullChargeEvaluator.systemParamsSQL.QueryRow("full_charge_threshold")
	err = row.Scan(&fullChargeEvaluator.threshold)
	if err != nil {
		log.Println("Failed to get full_charge_threshold from system_parameters.", err)
		return err
	}

	rows, err := fullChargeEvaluator.checkFullSQL.Query()
	if err != nil {
		log.Println("Failed to get the current full cell status.", err)
		return err
	}
	var cellNumber uint8
	var fullCharge bool
	for rows.Next() {
		if err = rows.Scan(&cellNumber, &fullCharge); err != nil {
			log.Println("Error getting full charge rows. ", err)
			return err
		}
		if cellNumber < 100 {
			fullChargeEvaluator.fullFlags[0][cellNumber-1] = fullCharge
		} else {
			fullChargeEvaluator.fullFlags[1][cellNumber-101] = fullCharge
		}
	}
	return nil
}

func (fullChargeEvaluator *FullChargeEval) loadTemporaryTable(when time.Time, span int, bank uint8) (rows int64, err error) {
	rows = 0
	row := fullChargeEvaluator.loadDataSQL.QueryRow(when, span, bank)
	err = row.Scan(&rows)
	if err != nil {
		log.Println("Error getting row count for temporary table -", err)
	}
	return
}

func (fullChargeEvaluator *FullChargeEval) getFullChargeState(cell int, threshold float64) (bool, error) {
	var slope sql.NullFloat64

	res := fullChargeEvaluator.getSlopeSQL.QueryRow(cell)
	err := res.Scan(&slope)
	//	log.Println("Cell ", cell, " slope = ", slope, " threshold = ", threshold)
	if slope.Valid {
		return slope.Float64 < threshold, err
	} else {
		return false, err
	}
}

func (fullChargeEvaluator *FullChargeEval) setFullChargeState(state bool, when time.Time, cell int) (err error) {
	_, err = fullChargeEvaluator.setFullChargeSQL.Exec(state, when, cell)
	return
}

func (fullChargeEvaluator *FullChargeEval) ProcessFullCharge(when time.Time) error {
	err := fullChargeEvaluator.loadFullFlags()
	if err != nil {
		log.Println(err)
		return err
	}
	for bank := range fullChargeEvaluator.fullFlags {
		rows, err := fullChargeEvaluator.loadTemporaryTable(when, fullChargeEvaluator.span, uint8(bank))
		if err != nil {
			log.Println("Error getting the data to process -", err)
			return err
		}
		if rows >= fullChargeEvaluator.minRows {
			//			fmt.Println(rows, "rows read for bank", bank)
			for cell, flag := range fullChargeEvaluator.fullFlags[bank] {
				if !flag {
					full, err := fullChargeEvaluator.getFullChargeState(cell+1, fullChargeEvaluator.threshold)
					if err != nil {
						log.Println("Failed to get the cell state for cell ", cell+1, " - ", err)
						return err
					}
					fullChargeEvaluator.fullFlags[bank][cell] = full
					if full {
						err := fullChargeEvaluator.setFullChargeState(true, when, cell+1)
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
