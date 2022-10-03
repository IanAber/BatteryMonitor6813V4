package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

/**
GetTimeRange returns the start and end times passed as query parameters.
*/
func GetTimeRange(r *http.Request) (start time.Time, end time.Time, err error) {
	params := r.URL.Query()
	values := params["start"]
	if len(values) != 1 {
		err = fmt.Errorf("Exactly one 'start=' value must be supplied for start time")
		return
	}
	timeVal, err := time.Parse("2006-1-2 15:4", values[0])
	if err != nil {
		return
	} else {
		start = timeVal
	}

	values = params["end"]
	if len(values) != 1 {
		err = fmt.Errorf("Exactly one 'start=' value must be supplied for start time")
		return
	}
	timeVal, err = time.Parse("2006-1-2 15:4", values[0])
	if err != nil {
		return
	} else {
		end = timeVal
	}
	log.Println("Date/time requested from ", start, " to ", end)
	return
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

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, "Current Data", err, http.StatusBadRequest, false)
		return
	}

	var sSQL string

	if start.After(end) {
		ReturnJSONErrorString(w, "Current Data", "Start must be before end", http.StatusBadRequest, false)
		return
	}
	if end.Sub(start) > time.Hour {
		sSQL = `select min(unix_timestamp(logged)) as logged,
		avg(channel_0) as left_,
		avg(channel_1) as right_,
		avg(level_of_charge_0) as soc_left,
		avg(level_of_charge_1) as soc_right
		from current
		where logged between ? and ?
		group by unix_timestamp(logged) DIV 15`
	} else {
		sSQL = `select unix_timestamp(logged) as logged,
		channel_0 as left_,
		channel_1 as right_,
		level_of_charge_0 as soc_left,
		level_of_charge_1 as soc_right
		from current
		where logged between ? and ?`
	}

	rows, err := pDB.Query(sSQL, start, end)
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
